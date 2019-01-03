package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-http-utils/headers"
)

const (
	REQUEST_TIMEOUT     int    = 120
	TABLEAU_HOST        string = "data.ifoodcorp.com.br"
	TABLEAU_API_VERSION string = "3.0"
	VIEWS_PAGE_SIZE     int    = 1000
)

var (
	httpClient = &http.Client{
		Timeout: time.Duration(REQUEST_TIMEOUT) * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
		},
	}

	tableauSession *TableauSession
)

type TableauSession struct {
	AuthToken string
	Site      string
}

type Site struct {
	ID         string `json:"id,omitempty" xml:"id,attr,omitempty"`
	Name       string `json:"name,omitempty" xml:"name,attr,omitempty"`
	ContentUrl string `json:"contentUrl,omitempty" xml:"contentUrl,attr,omitempty"`
}

type Credentials struct {
	Name     string `json:"name,omitempty" xml:"name,attr,omitempty"`
	Password string `json:"password,omitempty" xml:"password,attr,omitempty"`
	Token    string `json:"token,omitempty" xml:"token,attr,omitempty"`
	Site     *Site  `json:"site,omitempty" xml:"site,omitempty"`
}

type AuthResponse struct {
	Credentials *Credentials `json:"credentials,omitempty" xml:"credentials,omitempty"`
}

type Pagination struct {
	PageNumber     int `json:"pageNumber,omitempty" xml:"pageNumber,attr,omitempty"`
	PageSize       int `json:"pageSize,omitempty" xml:"pageSize,attr,omitempty"`
	TotalAvailable int `json:"totalAvailable,omitempty" xml:"totalAvailable,attr,omitempty"`
}

type View struct {
	Id         string `json:"id,omitempty" xml:"id,attr,omitempty"`
	Name       string `json:"name,omitempty" xml:"name,attr,omitempty"`
	ContentUrl string `json:"contentUrl,omitempty" xml:"contentUrl,attr,omitempty"`
}

type ViewsResponse struct {
	Pagination *Pagination `json:"pagination,omitempty" xml:"pagination,omitempty"`
	Views      []*View     `json:"views,omitempty" xml:"views>view,omitmepty"`
}

type TableauService struct {
	client  *TableauClient
	session *TableauSession
	views   []*View
}

func (ts *TableauService) Authenticate(login string, password string) (err error) {
	ts.session = &TableauSession{}
	ts.session.AuthToken, ts.session.Site, err = ts.client.authenticate(login, password)
	return err
}

func (ts *TableauService) LoadAllViews() (err error) {
	ts.views = []*View{}
	pageNumber := 1
	var hasMore bool
	var viewList []*View
	for viewList, hasMore, err = ts.client.viewList(ts.session.AuthToken, ts.session.Site, pageNumber); hasMore && err == nil; pageNumber++ {
		ts.views = append(ts.views, viewList...)
		viewList, hasMore, err = ts.client.viewList(ts.session.AuthToken, ts.session.Site, pageNumber)
	}
	if pageNumber == 1 {
		ts.views = viewList
	}
	log.Println("hasMore: ", hasMore)
	log.Println("Err: ", err)
	log.Println("pageNumber: ", pageNumber)
	return err
}

func (ts *TableauService) SearchViewByName(token string, limit int) (results []*View, limited bool) {
	for _, view := range ts.views {
		if strings.Contains(strings.ToLower(view.Name), strings.ToLower(token)) {
			results = append(results, view)
		}
		if len(results) >= limit {
			break
		}
	}
	return results, len(results) >= limit
}

func (ts *TableauService) GetView(contentUrl string) (io.Reader, error) {
	return ts.client.getView(ts.session.AuthToken, contentUrl)
}

type TableauClient struct{}

func (tc *TableauClient) authenticate(login string, password string) (token string, site string, err error) {
	data := []byte(fmt.Sprintf("<tsRequest><credentials name='%s' password='%s'><site contentUrl='' /></credentials></tsRequest>", login, password))
	url := fmt.Sprintf("https://%s/api/%s/auth/signin", TABLEAU_HOST, TABLEAU_API_VERSION)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", "", err
	}
	req.Header.Set(headers.ContentType, "application/xml")

	log.Println(req)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer readCloseResponse(resp.Body)

	var authResponse = &AuthResponse{}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	log.Println(string(respBytes))
	err = xml.Unmarshal(respBytes, authResponse)
	if err != nil {
		return "", "", err
	}

	return authResponse.Credentials.Token, authResponse.Credentials.Site.ID, nil
}

func (tc *TableauClient) viewList(authToken string, site string, pageNumber int) (views []*View, hasMore bool, err error) {
	url := fmt.Sprintf("https://%s/api/%s/sites/%s/views", TABLEAU_HOST, TABLEAU_API_VERSION, site)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set(headers.ContentType, "application/xml")
	req.Header.Set("X-Tableau-Auth", authToken)
	query := req.URL.Query()
	query.Add("pageSize", strconv.Itoa(VIEWS_PAGE_SIZE))
	query.Add("pageNumber", strconv.Itoa(pageNumber))
	req.URL.RawQuery = query.Encode()

	log.Println(req)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer readCloseResponse(resp.Body)

	var viewsResponse = &ViewsResponse{}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	err = xml.Unmarshal(respBytes, viewsResponse)
	if err != nil {
		return nil, false, err
	}

	return viewsResponse.Views, (viewsResponse.Pagination.PageNumber * viewsResponse.Pagination.PageSize) < viewsResponse.Pagination.TotalAvailable, nil
}

func (tc *TableauClient) getView(authToken, contentUrl string) (io.Reader, error) {
	url := fmt.Sprintf("http://%s/views/%s.png", TABLEAU_HOST, contentUrl)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headers.Cookie, fmt.Sprintf("workgroup_session_id=%s", authToken))
	req.Header.Set("Connection", "keep-alive")
	query := req.URL.Query()
	query.Add(":embed", "y")
	query.Add(":refresh", "yes")
	query.Add(":highdpi", "true")
	query.Add(":size", "1920,1080")
	req.URL.RawQuery = query.Encode()

	log.Println(req)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error making request: %v", err)
	}

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, resp.Body); err != nil {
		return nil, fmt.Errorf("Error creating buffer: %v", err)
	}

	return &buffer, nil
}

func readCloseResponse(resp io.ReadCloser) {
	io.Copy(ioutil.Discard, resp)
	resp.Close()
}
