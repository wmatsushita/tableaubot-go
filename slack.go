package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/nlopes/slack"
)

const (
	// action is used for slack attament action.
	actionSelect = "select"
	actionStart  = "start"
	actionCancel = "cancel"
)

type SlackService struct {
	client *slack.Client
	bot    *Bot
	botID  string
}

func (s *SlackService) PostMessage(channel, text string) error {
	_, _, err := s.client.PostMessage(channel, slack.MsgOptionText(text, false))
	return err
}

func (s *SlackService) PostViewListMessage(channel, text string, views []*View) error {
	attachment := slack.Attachment{
		Text:       text,
		Color:      "#f9a41b",
		CallbackID: "viewRequest",
		Actions: []slack.AttachmentAction{
			{
				Name:    actionSelect,
				Type:    "select",
				Options: viewListToAttachmentActionOption(views),
			},

			{
				Name:  actionCancel,
				Text:  "Cancel",
				Type:  "button",
				Style: "danger",
			},
		},
	}

	params := slack.MsgOptionAttachments(attachment)

	log.Println("Will send response!")
	if _, _, err := s.client.PostMessage(channel, params); err != nil {
		return fmt.Errorf("failed to post message: %s", err)
	}

	return nil
}

func (s *SlackService) PostFileUploadMessage(channel, fileName string, reader io.Reader) error {
	params := slack.FileUploadParameters{
		Filename: fileName, Reader: reader,
		Channels: []string{channel}}
	if _, err := s.client.UploadFile(params); err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	return nil
}

func viewListToAttachmentActionOption(views []*View) []slack.AttachmentActionOption {
	options := []slack.AttachmentActionOption{}
	for _, view := range views {
		options = append(options, slack.AttachmentActionOption{Text: view.Name, Value: strings.Replace(view.ContentUrl, "sheets/", "", 1)})
	}

	return options
}
