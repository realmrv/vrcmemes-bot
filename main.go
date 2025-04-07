package main

import (
	"context"
	"fmt"
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
	"vrcmemes-bot/internal/mediagroups"
	"vrcmemes-bot/internal/suggestions"

	telegoBot "vrcmemes-bot/bot"

	sentry "github.com/getsentry/sentry-go"
	telego "github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/mongo"
	// _ "go.uber.org/automaxprocs" // Uncomment if needed
)

// initSentry initializes the Sentry client based on the configuration.
func initSentry(cfg *config.Config) error {
	if cfg.SentryDSN == "" {
		log.Println("Sentry DSN not provided, skipping initialization.")
		return nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Environment:      cfg.AppEnv,
		Release:          cfg.Version,
		EnableTracing:    true,
		TracesSampleRate: 1.0, // Adjust as needed
		Debug:            cfg.Debug,
	})
	if err != nil {
		return fmt.Errorf("sentry.Init: %w", err)
	}
	log.Println("Sentry initialized.")
	return nil
}

// connectDatabase establishes a connection to MongoDB.
// Returns the client, database instance, and any error.
func connectDatabase(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	client, _, err := database.ConnectDB(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	// Note: The context returned by ConnectDB might be useful for the initial connection setup,
	// but we don't typically need to keep it around. The client manages its own context.
	log.Println("Connected to MongoDB.")
	db := client.Database(cfg.MongoDBDatabase)
	return client, db, nil
}

// createRepositories initializes all necessary database repositories.
func createRepositories(db *mongo.Database) (
	database.SuggestionRepository,
	database.UserActionLogger,
	database.PostLogger,
	database.UserRepository,
	database.FeedbackRepository,
) {
	suggestionRepo := database.NewMongoSuggestionRepository(db)
	userActionLogger := database.NewMongoLogger(db) // Assumes MongoLogger implements UserActionLogger
	postLogger := database.NewMongoLogger(db)       // Assumes MongoLogger implements PostLogger
	userRepo := database.NewMongoLogger(db)         // Assumes MongoLogger implements UserRepository
	feedbackRepo := database.NewFeedbackRepository(db)

	return suggestionRepo, userActionLogger, postLogger, userRepo, feedbackRepo
}

// setupBotComponents creates the core application components like admin checker,
// suggestion manager, and message handler.
func setupBotComponents(
	cfg *config.Config,
	bot *telego.Bot,
	suggRepo database.SuggestionRepository,
	actionLogger database.UserActionLogger,
	postLogger database.PostLogger,
	userRepo database.UserRepository,
	feedbackRepo database.FeedbackRepository,
	mediaGroupMgr *mediagroups.Manager,
) (*auth.AdminChecker, *suggestions.Manager, *handlers.MessageHandler, error) {

	adminChecker, err := auth.NewAdminChecker(bot, cfg.ChannelID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create admin checker: %w", err)
	}

	suggestionManager := suggestions.NewManager(
		bot,
		suggRepo,
		cfg.ChannelID,
		adminChecker,
		feedbackRepo,
		mediaGroupMgr,
	)

	messageHandler := handlers.NewMessageHandler(
		cfg.ChannelID,
		postLogger,
		actionLogger,
		userRepo,
		suggestionManager,
		adminChecker,
		feedbackRepo,
	)

	return adminChecker, suggestionManager, messageHandler, nil
}

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize localization
	locales.Init(cfg.DefaultLanguage)

	// Initialize Sentry
	if err = initSentry(cfg); err != nil {
		log.Fatalf("Sentry initialization error: %v", err)
	}
	// Ensure Sentry flushes buffered events before exit (if initialized)
	if cfg.SentryDSN != "" {
		defer sentry.Flush(2 * time.Second)
	}

	// Connect to Database
	client, db, err := connectDatabase(cfg)
	if err != nil {
		sentry.CaptureException(err) // Capture connection error
		log.Fatal(err)
	}
	defer func() {
		log.Println("Attempting to disconnect from MongoDB...")
		disconnectCtx, cancelDisconnect := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDisconnect()
		if err = client.Disconnect(disconnectCtx); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
			sentry.CaptureException(err)
		} else {
			log.Println("Disconnected from MongoDB (shutdown).")
		}
	}()

	// Create Repositories
	suggestionRepo, userActionLogger, postLogger, userRepo, feedbackRepo := createRepositories(db)

	// Create Media Group Manager
	mediaGroupMgr := mediagroups.NewManager()

	// Creating application lifecycle context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Bot Initialization ---
	// 1. Create the raw telego bot instance
	botOpts := []telego.BotOption{telego.WithDefaultLogger(false, false)}
	if cfg.Debug {
		botOpts = []telego.BotOption{telego.WithDefaultDebugLogger()}
	}
	bot, err := telego.NewBot(cfg.BotToken, botOpts...)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to create telego bot: %v", err)
	}

	// 1.5 Get updates channel BEFORE creating components that need the BotAPI interface
	updatesChan, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to get updates channel: %v", err)
	}

	// 2. Setup Core Bot Components (Checker, Manager, Handler)
	// Pass the concrete *telego.Bot to components that need it for specific methods
	_, suggestionManager, messageHandler, err := setupBotComponents(
		cfg, bot, suggestionRepo, userActionLogger, postLogger, userRepo, feedbackRepo, mediaGroupMgr,
	)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to setup bot components: %v", err)
	}

	// 3. Create the Bot Application Wrapper
	// Pass the updatesChan to BotDeps
	appBotDeps := telegoBot.BotDeps{
		Bot:           bot,         // Pass the concrete bot as BotAPI
		UpdatesChan:   updatesChan, // Pass the channel
		Debug:         cfg.Debug,
		ChannelID:     cfg.ChannelID,
		CaptionProv:   messageHandler, // Assuming MessageHandler implements CaptionProvider
		PostLogger:    postLogger,
		HandlerProv:   messageHandler, // Assuming MessageHandler implements HandlerProvider
		SuggestionMgr: suggestionManager,
		CallbackProc:  messageHandler, // Assuming MessageHandler implements CallbackProcessor
		UserRepo:      userRepo,
		ActionLogger:  userActionLogger,
		MediaGroupMgr: mediaGroupMgr,
		Handler:       messageHandler, // Pass concrete handler
	}
	appBot, err := telegoBot.New(appBotDeps)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to create application bot wrapper: %v", err)
	}

	// Start the bot wrapper's processing loop
	go appBot.Start(ctx)

	// Wait for shutdown signal
	<-ctx.Done()

	log.Println("Shutting down bot...")
	// Stop the bot wrapper gracefully (appBot.Stop() might need context or other changes)
	// No need to call bot.StopLongPolling() here, context cancellation handles it.
	appBot.Stop()

	// Stop media group timers
	if mediaGroupMgr != nil {
		mediaGroupMgr.Shutdown()
	}

	// Disconnect from MongoDB using the application context
}
