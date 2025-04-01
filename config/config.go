package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration parameters
type Config struct {
	BotToken    string
	ChannelID   int64
	Debug       bool
	Version     string
	SentryDSN   string
	AppEnv      string
	MongoDBURI  string
	MongoDBName string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Loading environment variables
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	// Getting debug mode from environment variables
	debug := os.Getenv("DEBUG") == "true"

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

	// Getting version from environment variables
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
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

	// Getting MongoDB URI from environment variables
	mongoDBURI := os.Getenv("MONGODB_URI")
	if mongoDBURI == "" {
		return nil, fmt.Errorf("MONGODB_URI is not set")
	}

	// Getting MongoDB database name from environment variables
	mongoDBName := os.Getenv("MONGODB_DATABASE")
	if mongoDBName == "" {
		return nil, fmt.Errorf("MONGODB_DATABASE is not set")
	}

	return &Config{
		BotToken:    token,
		ChannelID:   channelID,
		Debug:       debug,
		Version:     version,
		SentryDSN:   sentryDSN,
		AppEnv:      appEnv,
		MongoDBURI:  mongoDBURI,
		MongoDBName: mongoDBName,
	}, nil
}
