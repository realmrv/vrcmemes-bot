package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	sentry "github.com/getsentry/sentry-go"
	telego "github.com/mymmrac/telego"

	telegoBot "vrcmemes-bot/bot"
	"vrcmemes-bot/config"
	"vrcmemes-bot/database"
	"vrcmemes-bot/handlers"
	"vrcmemes-bot/internal/suggestions"
	"vrcmemes-bot/pkg/locales"
	// _ "go.uber.org/automaxprocs" // Uncomment if needed
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize localization bundle
	locales.Init()

	// Initialize Sentry (if DSN is provided)
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
	client, _, err := database.ConnectDB(cfg)
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

	// Create repository instances
	db := client.Database(cfg.MongoDBDatabase) // Get database instance
	suggestionRepo := database.NewMongoSuggestionRepository(db)
	userActionLogger := database.NewMongoLogger(db)
	postLogger := database.NewMongoLogger(db)
	// Assuming UserRepository is implemented by MongoLogger for now
	// If not, use: userRepo := database.NewMongoUserRepository(db)
	userRepo := database.NewMongoLogger(db)

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

	// 2. Create the suggestion manager
	suggestionManager := suggestions.NewManager(
		bot,
		suggestionRepo, // Pass the specific suggestion repository
		cfg.ChannelID,
	)

	// 3. Create message handler with dependencies
	messageHandler := handlers.NewMessageHandler(
		cfg.ChannelID,
		postLogger,       // Pass the specific post logger
		userActionLogger, // Pass the specific action logger
		userRepo,         // Pass the specific user repository
		suggestionManager,
	)

	// 4. Create the bot wrapper
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
