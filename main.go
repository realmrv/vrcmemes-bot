package main

import (
	"context"
	"log"
	"time"

	"vrcmemes-bot/bot"
	"vrcmemes-bot/config"

	"github.com/getsentry/sentry-go"
)

func main() {
	// Loading configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Sentry
	err = sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Environment:      cfg.AppEnv,
		Release:          cfg.Version,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
		Debug:            cfg.Debug,
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	// Creating context
	ctx := context.Background()

	// Creating and starting bot
	b, err := bot.New(cfg.BotToken, cfg.ChannelID, cfg.Debug)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal(err)
	}

	b.Start(ctx)
}
