package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	telegoBot "vrcmemes-bot/bot"
	"vrcmemes-bot/config"
	"vrcmemes-bot/database"
	"vrcmemes-bot/handlers"
	"vrcmemes-bot/internal/suggestions"

	"github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
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

	// --- Bot Initialization ---
	// 1. Create the raw telego bot instance first
	var bot *telego.Bot
	if cfg.Debug {
		bot, err = telego.NewBot(cfg.BotToken, telego.WithDefaultDebugLogger())
	} else {
		bot, err = telego.NewBot(cfg.BotToken, telego.WithDefaultLogger(false, false))
	}
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to create telego bot: %v", err)
	}

	// 2. Create the suggestion repository
	suggestionRepo := database.NewMongoSuggestionRepository(db)

	// 3. Create the suggestion manager
	suggestionMgr := suggestions.NewManager(bot, cfg.ChannelID, suggestionRepo)

	// 4. Create message handler with dependencies
	messageHandler := handlers.NewMessageHandler(cfg.ChannelID, dbLogger, dbLogger, dbLogger, suggestionMgr)

	// 5. Create the bot wrapper
	appBot, err := telegoBot.New(bot, cfg.Debug, messageHandler)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal(err)
	}

	// Start the bot wrapper's processing loop in a separate goroutine
	go appBot.Start(ctx)

	// Wait for context cancellation (e.g., SIGINT, SIGTERM)
	<-ctx.Done()

	log.Println("Shutting down bot...")
	// Stop the bot wrapper gracefully
	appBot.Stop()

	log.Println("Bot shutdown complete.")
}
