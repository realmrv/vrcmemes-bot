package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds the application configuration.
type Config struct {
	AppEnv          string
	Debug           bool
	Version         string
	BotToken        string
	ChannelID       int64
	SentryDSN       string
	MongoDBURI      string
	MongoDBDatabase string
}

// LoadConfig loads configuration from environment variables.
// It attempts to load a .env file if present but prioritizes
// actual environment variables set in the system (e.g., by Docker).
func LoadConfig() (*Config, error) {
	// Load .env file if it exists (useful for development)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	debug, _ := strconv.ParseBool(getEnv("DEBUG", "false"))

	channelIDStr := getEnv("CHANNEL_ID", "")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil && channelIDStr != "" {
		return nil, fmt.Errorf("invalid CHANNEL_ID: %w", err)
	} else if channelIDStr == "" {
		log.Println("Warning: CHANNEL_ID is not set") // Warning instead of error?
	}

	cfg := &Config{
		AppEnv:          getEnv("APP_ENV", "development"),
		Debug:           debug,
		Version:         getEnv("VERSION", "dev"),
		BotToken:        getEnv("TELEGRAM_BOT_TOKEN", ""),
		ChannelID:       channelID,
		SentryDSN:       getEnv("SENTRY_DSN", ""),
		MongoDBURI:      getEnv("MONGODB_URI", ""), // URI might be complex, handle validation carefully if needed
		MongoDBDatabase: getEnv("MONGODB_DATABASE", ""),
	}

	// Basic validation for essential variables
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.ChannelID == 0 {
		return nil, fmt.Errorf("CHANNEL_ID is required")
	}
	if cfg.SentryDSN == "" {
		log.Println("Warning: SENTRY_DSN is not set. Error tracking disabled.")
	}
	if cfg.MongoDBURI == "" {
		return nil, fmt.Errorf("MONGODB_URI is required")
	}
	if cfg.MongoDBDatabase == "" {
		return nil, fmt.Errorf("MONGODB_DATABASE is required")
	}

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
