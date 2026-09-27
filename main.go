package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	telegoBot "vrcmemes-bot/bot"
	"vrcmemes-bot/internal/config"
	"vrcmemes-bot/internal/container"
	"vrcmemes-bot/internal/locales"

	"github.com/getsentry/sentry-go"
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

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	// Initialize localization
	locales.Init(cfg.DefaultLanguage)

	// Initialize Sentry
	if err = initSentry(cfg); err != nil {
		return fmt.Errorf("Sentry initialization: %w", err)
	}
	if cfg.SentryDSN != "" {
		defer sentry.Flush(2 * time.Second)
	}

	// Create the DI container
	c, err := container.New(cfg)
	if err != nil {
		sentry.CaptureException(err)
		return fmt.Errorf("create DI container: %w", err)
	}

	// Creating application lifecycle context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Defer closing container resources
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c.Close(shutdownCtx)
	}()

	// Get updates channel
	updatesChan, err := c.Bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("get updates channel: %w", err)
	}

	// Create the Bot Application Wrapper
	appBotDeps := telegoBot.BotDeps{
		Bot:           c.Bot,
		UpdatesChan:   updatesChan,
		Debug:         c.Cfg.Debug,
		ChannelID:     c.Cfg.ChannelID,
		CaptionProv:   c.MessageHandler,
		PostLogger:    c.PostLogger,
		HandlerProv:   c.MessageHandler,
		SuggestionMgr: c.SuggestionMgr,
		CallbackProc:  c.MessageHandler,
		UserRepo:      c.UserRepo,
		ActionLogger:  c.ActionLogger,
		MediaGroupMgr: c.MediaGroupMgr,
		Handler:       c.MessageHandler,
	}
	appBot, err := telegoBot.New(appBotDeps)
	if err != nil {
		sentry.CaptureException(err)
		return fmt.Errorf("create application bot wrapper: %w", err)
	}

	// Process updates until shutdown or a polling failure.
	err = appBot.Start(ctx)
	log.Println("Shutting down bot...")
	appBot.Stop()
	return err
}
