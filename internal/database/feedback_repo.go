package database

import (
	"context"
	"fmt"
	"log"
	"time" // Needed for SubmittedAt
	"vrcmemes-bot/internal/database/models"

	"go.mongodb.org/mongo-driver/bson/primitive" // Needed for ObjectID
	"go.mongodb.org/mongo-driver/mongo"
)

// feedbackRepository is a MongoDB implementation of FeedbackRepository.
type feedbackRepository struct {
	collection *mongo.Collection
}

// NewFeedbackRepository creates a new instance of feedbackRepository.
// Make sure this function is exported (starts with a capital letter).
func NewFeedbackRepository(db *mongo.Database) FeedbackRepository { // Return interface type
	collectionName := "feedback" // Simpler collection name
	return &feedbackRepository{
		collection: db.Collection(collectionName),
	}
}

// AddFeedback saves a new feedback entry to the MongoDB collection.
func (r *feedbackRepository) AddFeedback(ctx context.Context, feedback *models.Feedback) error {
	// Generate ObjectID if it's not set
	if feedback.ID.IsZero() {
		feedback.ID = primitive.NewObjectID()
	}
	// Set submission time if not already set
	if feedback.SubmittedAt.IsZero() {
		feedback.SubmittedAt = time.Now()
	}

	_, err := r.collection.InsertOne(ctx, feedback)
	if err != nil {
		log.Printf("Error inserting feedback from user %d into MongoDB: %v", feedback.UserID, err)
		// Wrap the error for better context upstream
		return fmt.Errorf("failed to insert feedback into mongodb: %w", err)
	}
	log.Printf("Successfully inserted feedback (ID: %s) from user %d.", feedback.ID.Hex(), feedback.UserID)
	return nil
}
