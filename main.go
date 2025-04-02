package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vrcmemes-bot/bot"
	"vrcmemes-bot/config"
	"vrcmemes-bot/database"
	"vrcmemes-bot/handlers"

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

	// Connect to MongoDB
	client, db, err := database.ConnectDB(cfg)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal(err)
	}
	defer func() {
		if err = client.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
			sentry.CaptureException(err)
		} else {
			log.Println("Disconnected from MongoDB.")
		}
	}()

	// Create logger/repository instance
	dbLogger := database.NewMongoLogger(db)

	// Creating context for application lifecycle
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create message handler with dependencies
	messageHandler := handlers.NewMessageHandler(cfg.ChannelID, dbLogger, dbLogger, dbLogger)

	// Creating bot instance
	b, err := bot.New(cfg.BotToken, cfg.Debug, messageHandler)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal(err)
	}

	// Start the bot in a separate goroutine
	go b.Start(ctx)

	// Wait for context cancellation (e.g., SIGINT, SIGTERM)
	<-ctx.Done()

	log.Println("Shutting down bot...")
	// Stop the bot gracefully
	b.Stop()

	log.Println("Bot shutdown complete.")
}
