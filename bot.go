package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type Challenge struct {
	Challenge string `json:"challenge"`
}

type EventCallback struct {
	Token       string   `json:"token"`
	TeamID      string   `json:"team_id"`
	APIAppID    string   `json:"api_app_id"`
	Event       Event    `json:"event"`
	Type        string   `json:"type"`
	EventID     string   `json:"event_id"`
	EventTime   int      `json:"event_time"`
	AuthedUsers []string `json:"authed_users"`
}

type Event struct {
	ClientMsgID string `json:"client_msg_id"`
	Type        string `json:"type"`
	Text        string `json:"text"`
	User        string `json:"user"`
	Ts          string `json:"ts"`
	Channel     string `json:"channel"`
	EventTs     string `json:"event_ts"`
}

type Action struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	SelectedOptions []Option `json:"selected_options"`
}

type Option struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Interaction struct {
	Type         string   `json:"type"`
	Actions      []Action `json:"actions"`
	CallbackID   string   `json:"callback_id"`
	Channel      Channel  `json:"channel"`
	User         User     `json:"user"`
	ActionTs     string   `json:"action_ts"`
	MessageTs    string   `json:"message_ts"`
	AttachmentID string   `json:"attachment_id"`
	Token        string   `json:"token"`
	IsAppUnfurl  bool     `json:"is_app_unfurl"`
	ResponseURL  string   `json:"response_url"`
}

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

type Message struct {
	ReplaceOriginal bool   `json:"replace_original"`
	Text            string `json:"text"`
}

func respondMessage(responseURL string, message Message) error {
	msg, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("respondMessage: error marshalling message: %v", err)
	}
	body := bytes.NewBuffer(msg)
	_, err = http.Post(responseURL, "application/json", body)
	if err != nil {
		return fmt.Errorf("respondMessage: error POSTing message: %v", err)
	}
	return nil
}

func (b *Bot) FindViewsAndRespond(channel, token string) error {
	if b.TableauService == nil {
		return errors.New("TableauService was not initialized")
	}

	views, limited := b.TableauService.SearchViewByName(token, b.Config.Limit)
	if len(views) == 0 {
		if err := b.SlackService.PostMessage(channel, "Sorry, I didn't find any dashboard with those terms"); err != nil {
			return fmt.Errorf("error posting message to channel: %v", err)
		}
		return fmt.Errorf("error fetching views: no views")
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

	http.HandleFunc("/events", b.handleEvents)
	http.HandleFunc("/interaction", b.handleInteractions)
	http.HandleFunc("/", b.handleChallenge)

	log.Printf("[INFO] Server listening on :%s", b.Config.Port)
	if err := http.ListenAndServe(":"+b.Config.Port, nil); err != nil {
		return err
	}

	return nil
}

func (b *Bot) handleInteractions(w http.ResponseWriter, r *http.Request) {
	payload := r.FormValue("payload")
	var interaction Interaction
	if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
		log.Printf("error unmarshalling interaction: %v", err)
		return
	}

	if interaction.Token != b.Config.VerificationToken {
		log.Printf("error: invalid token: %s", interaction.Token)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	action := interaction.Actions[0]
	switch action.Name {
	case actionSelect:
		respondMessage(interaction.ResponseURL, Message{Text: "Dá pra fazer... :noiando:", ReplaceOriginal: true})
		go func() { // TODO: this is pretty naive, improve this otherwise we can blow the machine up
			tableauViewURL := action.SelectedOptions[0].Value
			reader, err := b.TableauService.GetView(tableauViewURL)
			if err != nil {
				log.Printf("error fetching view from Tableau: %v", err)
				return
			}
			if err := b.SlackService.PostFileUploadMessage(interaction.Channel.Name, "TableauDashboard.png", reader); err != nil {
				log.Printf("error uploading file: %v", err)
				return
			}
			respondMessage(interaction.ResponseURL, Message{Text: "Complete!", ReplaceOriginal: true})
		}()
		// end the http request
		return
	}
}

func (b *Bot) handleChallenge(w http.ResponseWriter, r *http.Request) {
	var ch Challenge
	if err := json.NewDecoder(r.Body).Decode(&ch); err != nil {
		log.Printf("[ERROR] in challenge unmarshal: %v", err)
		return
	}
	w.Write([]byte(ch.Challenge))
}

func (b *Bot) handleEvents(w http.ResponseWriter, r *http.Request) {
	var ev EventCallback
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		log.Printf("error decoding event: %v", err)
		return
	}

	// Parse message
	m := strings.Split(strings.TrimSpace(ev.Event.Text), " ")[1:]
	if len(m) == 0 {
		log.Printf("[ERROR] invalid message: %s", ev.Event.Text)
		return
	}
	log.Println("message:", m)

	if strings.ToLower(m[0]) == "find" && len(m) >= 2 {
		log.Println("finding views for", ev.Event.User)
		if err := b.FindViewsAndRespond(ev.Event.Channel, strings.Join(m[1:], " ")); err != nil {
			log.Printf("[ERROR] FindViewsAndRespond: %v", err)
			return
		}
	}
}
