package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"vrcmemes-bot/database/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoLogger implements logger interfaces using MongoDB.
// It handles logging user actions, published posts, and updating user info.
type MongoLogger struct {
	db *mongo.Database
}

// NewMongoLogger creates and returns a new MongoLogger instance.
// It requires a connected MongoDB database instance.
func NewMongoLogger(db *mongo.Database) *MongoLogger {
	return &MongoLogger{db: db}
}

// LogUserAction writes a user action log entry to the database.
// It records the user ID, action type, additional details, and timestamp.
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

	// Consider wrapping the error here as well for context
	if err != nil {
		return fmt.Errorf("failed to insert user action log for user %d: %w", userID, err)
	}
	return nil
}

// LogPublishedPost writes a log entry for a successfully published post to the database.
// It records details about the post, such as message ID and author.
// If the database insertion fails, it logs an error with context and returns the error.
func (m *MongoLogger) LogPublishedPost(logEntry models.PostLog) error {
	collection := m.db.Collection("post_logs")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, logEntry)
	if err != nil {
		// Add context to the error before logging and returning
		wrappedErr := fmt.Errorf("failed to insert post log into collection '%s': %w", "post_logs", err)
		log.Printf("%v", wrappedErr) // Log the contextualized error
		return wrappedErr            // Return the contextualized error
	}
	return nil // Return nil on success
}

// UpdateUser updates or inserts user information in the database.
// It sets user details (username, names, admin status), timestamps, action counts,
// and uses upsert to create the user if they don't exist.
func (m *MongoLogger) UpdateUser(ctx context.Context, userID int64, username, firstName, lastName string, isAdmin bool, action string) error {
	collection := m.db.Collection("users")

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
			"user_id":    userID,
		},
	}

	_, err := collection.UpdateOne(
		ctx, // Use the passed-in context
		bson.M{"user_id": userID},
		update,
		options.Update().SetUpsert(true),
	)

	// Consider wrapping the error here as well
	if err != nil {
		return fmt.Errorf("failed to update user %d: %w", userID, err)
	}
	return nil
}
