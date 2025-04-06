package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"vrcmemes-bot/internal/auth"
	"vrcmemes-bot/internal/config"
	"vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/handlers"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/internal/suggestions"

	telegoBot "vrcmemes-bot/bot"

	sentry "github.com/getsentry/sentry-go"
	telego "github.com/mymmrac/telego"
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
	if cfg.SentryDSN != "" {
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
		log.Println("Sentry initialized.")
	} else {
		log.Println("Sentry DSN not provided, skipping initialization.")
	}

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

	// 2. Create the Admin Checker
	adminChecker, err := auth.NewAdminChecker(bot, cfg.ChannelID)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to create admin checker: %v", err)
	}

	// 3. Create the suggestion manager
	suggestionManager := suggestions.NewManager(
		bot,
		suggestionRepo, // Pass the specific suggestion repository
		cfg.ChannelID,  // Keep channel ID here for now, maybe refactor manager later
		adminChecker,   // Pass admin checker
	)

	// 4. Create message handler with dependencies
	messageHandler := handlers.NewMessageHandler(
		cfg.ChannelID,
		postLogger,        // dbi.PostLogger
		userActionLogger,  // dbi.UserActionLogger
		userRepo,          // dbi.UserRepository
		suggestionManager, // dbi.SuggestionManager (implements parts)
		adminChecker,
	)

	// 5. Create the bot wrapper with dependencies
	appBotDeps := telegoBot.BotDeps{
		Bot:           bot,
		Debug:         cfg.Debug,
		ChannelID:     cfg.ChannelID,
		CaptionProv:   messageHandler,    // MessageHandler implements CaptionProvider
		PostLogger:    postLogger,        // Pass the specific logger
		HandlerProv:   messageHandler,    // MessageHandler implements HandlerProvider
		SuggestionMgr: suggestionManager, // Pass the manager instance
		CallbackProc:  messageHandler,    // MessageHandler implements CallbackProcessor
		UserRepo:      userRepo,
		ActionLogger:  userActionLogger,
	}
	appBot, err := telegoBot.New(appBotDeps)
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

	// Disconnect from MongoDB using the application context
	log.Println("Attempting to disconnect from MongoDB...")
	disconnectCtx, cancelDisconnect := context.WithTimeout(context.Background(), 5*time.Second) // Add timeout for disconnect
	defer cancelDisconnect()
	if err = client.Disconnect(disconnectCtx); err != nil {
		log.Printf("Error disconnecting from MongoDB: %v", err)
		sentry.CaptureException(err) // Report disconnect error if Sentry is enabled
	} else {
		log.Println("Disconnected from MongoDB (shutdown).")
	}

	log.Println("Bot shutdown complete.")
}
