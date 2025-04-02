package database

import (
	"context"
	"log"
	"time"

	"vrcmemes-bot/config"
	"vrcmemes-bot/database/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DB var DB *mongo.Database

// MongoLogger implements PostLogger and UserActionLogger using MongoDB.
type MongoLogger struct {
	db *mongo.Database
}

// NewMongoLogger creates a new MongoLogger.
func NewMongoLogger(db *mongo.Database) *MongoLogger {
	return &MongoLogger{db: db}
}

// ConnectDB establishes a connection to MongoDB and returns the client and database
func ConnectDB(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.MongoDBURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, nil, err
	}

	// Verify the connection
	err = client.Ping(ctx, nil)
	if err != nil {
		client.Disconnect(context.Background())
		return nil, nil, err
	}

	db := client.Database(cfg.MongoDBName)
	log.Println("Successfully connected to MongoDB")
	return client, db, nil
}

// LogUserAction writes a user action to the database
func (m *MongoLogger) LogUserAction(userID int64, action string, details interface{}) error {
	collection := m.db.Collection("user_actions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, map[string]interface{}{
		"user_id": userID,
		"action":  action,
		"details": details,
		"time":    time.Now(),
	})

	return err
}

// LogPublishedPost implements the PostLogger interface
func (m *MongoLogger) LogPublishedPost(logEntry models.PostLog) error {
	collection := m.db.Collection("post_logs")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, logEntry)
	if err != nil {
		log.Printf("Error inserting post log into collection '%s': %v", "post_logs", err)
	}
	return err
}

// UpdateUser implements the UserRepository interface
func (m *MongoLogger) UpdateUser(ctx context.Context, userID int64, username, firstName, lastName string, isAdmin bool, action string) error {
	collection := m.db.Collection("users")
	// Use the provided context, adding a timeout if desired, but respecting the original context's deadline/cancellation.
	// ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	// defer cancel() // Manage context cancellation carefully if you add timeouts here

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"username":    username,
			"first_name":  firstName,
			"last_name":   lastName,
			"is_admin":    isAdmin,
			"last_seen":   now,
			"last_action": action,
		},
		"$inc": bson.M{
			"actions_count": 1,
		},
		"$setOnInsert": bson.M{
			"first_seen": now,
			// Store UserID on insert as well, in case the filter doesn't match initially
			"user_id": userID,
		},
	}

	_, err := collection.UpdateOne(
		ctx, // Use the passed-in context
		bson.M{"user_id": userID},
		update,
		options.Update().SetUpsert(true),
	)

	return err
}
