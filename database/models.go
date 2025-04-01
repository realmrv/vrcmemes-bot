package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// User represents a Telegram user with their activity information
type User struct {
	UserID       int64     `bson:"user_id"`
	Username     string    `bson:"username,omitempty"`
	FirstName    string    `bson:"first_name,omitempty"`
	LastName     string    `bson:"last_name,omitempty"`
	IsAdmin      bool      `bson:"is_admin"`
	FirstSeen    time.Time `bson:"first_seen"`
	LastSeen     time.Time `bson:"last_seen"`
	ActionsCount int       `bson:"actions_count"`
	LastAction   string    `bson:"last_action"`
}

// UpdateUser updates or creates a user record in the database
// It updates the user's information and increments their action count
func UpdateUser(db *mongo.Database, userID int64, username, firstName, lastName string, isAdmin bool, action string) error {
	collection := db.Collection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		},
	}

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"user_id": userID},
		update,
		options.Update().SetUpsert(true),
	)

	return err
}
