package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/nlopes/slack"
)

type BotConfig struct {
	Port                               string
	BotToken, BotId, VerificationToken string
	TableauLogin, TableauPassword      string
	Limit                              int
}

type Bot struct {
	Config         *BotConfig
	TableauService *TableauService
	SlackService   *SlackService
}

func (b *Bot) Initialize() error {
	b.TableauService = &TableauService{}
	err := b.TableauService.Authenticate(b.Config.TableauLogin, b.Config.TableauPassword)
	if err != nil {
		log.Println("[ERROR] Error authenticating with Tableau", err)
		return err
	}
	err = b.TableauService.LoadAllViews()
	if err != nil {
		log.Println("[ERROR] Error loading all views", err)
		return err
	}

	b.SlackService = &SlackService{
		client: slack.New(b.Config.BotToken),
		bot:    b,
		botID:  b.Config.BotId,
	}

	slack.OptionDebug(true)(b.SlackService.client)
	go b.SlackService.ListenForEvents()

	return nil
}

func (b *Bot) FindViewsAndRespond(channel, token string) error {
	if b.TableauService == nil {
		return errors.New("TableauService was not initialized")
	}

	views, limited := b.TableauService.SearchViewByName(token, b.Config.Limit)
	if len(views) == 0 {
		b.SlackService.PostMessage(channel, "Sorry, I didn't find any dashboard with those terms")
		return nil
	}

	var msg string
	if limited {
		msg = fmt.Sprintf("I found too many dashboards and had to limit to %d results. \n If the one you want is not in the list, try search again writing more.", b.Config.Limit)
	} else {
		msg = "Here are the dashboards I found. The one you want should be in the list below"
	}

	return b.SlackService.PostViewListMessage(channel, msg, views)
}

func (b *Bot) ListenForInteractions() error {

	http.Handle("/interaction", b)

	log.Printf("[INFO] Server listening on :%s", b.Config.Port)
	if err := http.ListenAndServe(":"+b.Config.Port, nil); err != nil {
		return err
	}

	return nil
}

func (b *Bot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[ERROR] Invalid method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	jsonStr, err := url.QueryUnescape(string(buf)[8:])
	if err != nil {
		log.Printf("[ERROR] Failed to unescape request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var message slack.AttachmentActionCallback
	if err := json.Unmarshal([]byte(jsonStr), &message); err != nil {
		log.Printf("[ERROR] Failed to decode json message from slack: %s", jsonStr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Only accept message from slack with valid token
	if message.Token != b.Config.VerificationToken {
		log.Printf("[ERROR] Invalid token: %s", message.Token)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	action := message.Actions[0]
	switch action.Name {
	case actionSelect:
		tableauViewUrl := action.SelectedOptions[0].Value

		log.Println("")
		log.Printf("Received request to get view by url(%s)\n", tableauViewUrl)

		reader, err := b.TableauService.GetView(tableauViewUrl)
		if err != nil {
			log.Println("[ERROR] Error fetching view from Tableau", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		channel := message.OriginalMessage.Msg.Channel
		err = b.SlackService.PostFileUploadMessage(channel, action.SelectedOptions[0].Text, reader)
		if err != nil {
			log.Println("[ERROR] Error uploading file to slack", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		title := "Ok! I am fetching you dashboard right now"
		responseMessage(w, message.OriginalMessage, title, "")
		return
	case actionCancel:
		title := fmt.Sprintf(":x: @%s canceled the request", message.User.Name)
		responseMessage(w, message.OriginalMessage, title, "")
		return
	default:
		log.Printf("[ERROR] ]Invalid action was submitted: %s", action.Name)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// responseMessage response to the original slackbutton enabled message.
// It removes button and replace it with message which indicate how bot will work
func responseMessage(w http.ResponseWriter, original slack.Message, title, value string) {
	original.Attachments[0].Actions = []slack.AttachmentAction{} // empty buttons
	original.Attachments[0].Fields = []slack.AttachmentField{
		{
			Title: title,
			Value: value,
			Short: false,
		},
	}

	w.Header().Add("Content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&original)
}
