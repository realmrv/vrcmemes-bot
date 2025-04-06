package database

import (
	"context"
	"fmt"
	"log"
	"vrcmemes-bot/internal/config"

	// "vrcmemes-bot/database/models" // No longer needed here

	// "go.mongodb.org/mongo-driver/bson" // No longer needed here
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DB var DB *mongo.Database // Commented out or remove if not used globally

// ConnectDB establishes a connection to the MongoDB database using the provided configuration.
// It returns the MongoDB client, database object, and an error if connection fails.
func ConnectDB(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(cfg.MongoDBURI).SetServerAPIOptions(serverAPI)

	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Send a ping to confirm a successful connection
	var result bson.M
	if err := client.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "ping", Value: 1}}).Decode(&result); err != nil {
		_ = client.Disconnect(context.TODO()) // Attempt to disconnect on ping failure
		return nil, nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	log.Println("Successfully connected and pinged MongoDB!")

	db := client.Database(cfg.MongoDBDatabase) // Use MongoDBDatabase here

	return client, db, nil
}
