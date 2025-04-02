package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"vrcmemes-bot/config"
	// "vrcmemes-bot/database/models" // No longer needed here

	// "go.mongodb.org/mongo-driver/bson" // No longer needed here
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DB var DB *mongo.Database // Commented out or remove if not used globally

// ConnectDB establishes a connection to the MongoDB specified in the configuration.
// It returns a MongoDB client, a database instance, and an error if connection fails.
func ConnectDB(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.MongoDBURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		// Wrap error for context
		return nil, nil, fmt.Errorf("failed to connect to MongoDB at %s: %w", cfg.MongoDBURI, err)
	}

	// Verify the connection
	err = client.Ping(ctx, nil)
	if err != nil {
		// Ensure disconnect attempt happens before returning the ping error
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		if disconnectErr := client.Disconnect(disconnectCtx); disconnectErr != nil {
			log.Printf("Error disconnecting MongoDB after ping failure: %v", disconnectErr)
		}
		// Wrap ping error
		return nil, nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	db := client.Database(cfg.MongoDBName)
	log.Printf("Successfully connected to MongoDB database: %s", cfg.MongoDBName)
	return client, db, nil
}
