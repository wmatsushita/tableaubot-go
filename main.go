package main

import (
	"log"
	"os"

	"github.com/kelseyhightower/envconfig"
)

type envConfig struct {
	// Port is server port to be listened.
	Port string `envconfig:"PORT" default:"3000"`

	// BotToken is bot user token to access to slack API.
	BotToken string `envconfig:"BOT_TOKEN" required:"true"`

	// VerificationToken is used to validate interactive messages from slack.
	VerificationToken string `envconfig:"VERIFICATION_TOKEN" required:"true"`

	// BotID is bot user ID.
	BotID string `envconfig:"BOT_ID" required:"true"`

	// ChannelID is slack channel ID where bot is working.
	// Bot responses to the mention in this channel.
	ChannelID string `envconfig:"CHANNEL_ID"`

	TableauLogin    string `envconfig:"TABLEAU_LOGIN" required:"true"`
	TableauPassword string `envconfig:"TABLEAU_PASSWORD" required:"true"`

	BotConfigLimit int `envconfig:"BOT_CONFIG_LIMIT" default:"20"`
}

func main() {
	os.Exit(_main(os.Args[1:]))
}

func _main(args []string) int {
	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Printf("[ERROR] Failed to process env var: %s", err)
		return 1
	}

	bot := &Bot{
		Config: &BotConfig{
			Port:              env.Port,
			Limit:             env.BotConfigLimit,
			BotToken:          env.BotToken,
			VerificationToken: env.VerificationToken,
			BotId:             env.BotID,
			TableauLogin:      env.TableauLogin,
			TableauPassword:   env.TableauPassword,
		},
	}
	err := bot.Initialize()
	if err != nil {
		log.Println("[ERROR] Failed to initialize Bot", err)
		return 1
	}

	err = bot.ListenForInteractions()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return 1
	}

	return 0
}
