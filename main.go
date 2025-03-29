package main

import (
	"context"
	"log"

	"vrcmemes-bot/bot"
	"vrcmemes-bot/config"
)

func main() {
	// Loading configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Creating context
	ctx := context.Background()

	// Creating and starting bot
	b, err := bot.New(cfg.BotToken, cfg.ChannelID)
	if err != nil {
		log.Fatal(err)
	}

	b.Start(ctx)
}
