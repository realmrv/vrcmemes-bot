package database

import (
	"context"
	"fmt"
	"log"
	"time"
	"vrcmemes-bot/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectDB establishes a connection to the MongoDB database using the provided configuration.
// It returns the MongoDB client, database object, and an error if connection fails.
func ConnectDB(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(cfg.MongoDBURI).SetServerAPIOptions(serverAPI)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	db := client.Database(cfg.MongoDBDatabase)
	// Listing collections verifies both connectivity and the runtime user's permissions.
	if _, err := db.ListCollectionNames(ctx, bson.D{}); err != nil {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
		return nil, nil, fmt.Errorf("failed to authenticate MongoDB access: %w", err)
	}
	log.Println("Successfully authenticated MongoDB access.")

	return client, db, nil
}
