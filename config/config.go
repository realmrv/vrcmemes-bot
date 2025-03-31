package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration parameters
type Config struct {
	BotToken  string
	ChannelID int64
	Debug     bool
	Version   string
	SentryDSN string
	AppEnv    string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Loading environment variables
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	// Getting bot token from environment variables
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}

	// Getting channel ID from environment variables
	channelIDStr := os.Getenv("CHANNEL_ID")
	if channelIDStr == "" {
		return nil, fmt.Errorf("CHANNEL_ID is not set")
	}

	// Converting string to int64 for ChatID
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CHANNEL_ID format: %w", err)
	}

	// Getting debug mode from environment variables
	debug := os.Getenv("DEBUG") == "true"

	// Getting version from environment variables
	version := os.Getenv("VERSION")
	if version == "" {
		return nil, fmt.Errorf("VERSION is not set")
	}

	// Getting sentry DSN from environment variables
	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN == "" {
		return nil, fmt.Errorf("SENTRY_DSN is not set")
	}

	// Getting app environment from environment variables
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development" // Default to development if not set
	}

	return &Config{
		BotToken:  token,
		ChannelID: channelID,
		Debug:     debug,
		Version:   version,
		SentryDSN: sentryDSN,
		AppEnv:    appEnv,
	}, nil
}
