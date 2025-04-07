package database

import (
	"context"
	"fmt"
	"time"
	"vrcmemes-bot/internal/database/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const suggestionCollectionName = "suggestions"

// MongoSuggestionRepository implements SuggestionRepository for MongoDB.
type MongoSuggestionRepository struct {
	collection *mongo.Collection
}

// NewMongoSuggestionRepository creates a new MongoDB suggestion repository.
func NewMongoSuggestionRepository(db *mongo.Database) *MongoSuggestionRepository {
	return &MongoSuggestionRepository{
		collection: db.Collection(suggestionCollectionName),
	}
}

// CreateSuggestion adds a new suggestion to the database.
func (r *MongoSuggestionRepository) CreateSuggestion(ctx context.Context, suggestion *models.Suggestion) error {
	suggestion.ID = primitive.NewObjectID() // Generate new ID if empty
	suggestion.SubmittedAt = time.Now()

	_, err := r.collection.InsertOne(ctx, suggestion)
	if err != nil {
		return fmt.Errorf("failed to insert suggestion: %w", err)
	}
	return nil
}

// GetPendingSuggestions retrieves a paginated list of suggestions with 'pending' status.
func (r *MongoSuggestionRepository) GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error) {
	filter := bson.M{"status": "pending"}

	// Get total count
	totalCount, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count pending suggestions: %w", err)
	}

	if totalCount == 0 {
		return []models.Suggestion{}, 0, nil
	}

	// Find documents with pagination
	findOptions := options.Find()
	findOptions.SetLimit(int64(limit))
	findOptions.SetSkip(int64(offset))
	findOptions.SetSort(bson.D{{Key: "submitted_at", Value: 1}}) // Oldest first

	cursor, err := r.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find pending suggestions: %w", err)
	}
	defer cursor.Close(ctx)

	var suggestions []models.Suggestion
	if err = cursor.All(ctx, &suggestions); err != nil {
		return nil, 0, fmt.Errorf("failed to decode pending suggestions: %w", err)
	}

	return suggestions, totalCount, nil
}

// GetSuggestionByID retrieves a single suggestion by its MongoDB ObjectID.
// It returns ErrSuggestionNotFound if no suggestion matches the ID.
func (r *MongoSuggestionRepository) GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error) {
	var suggestion models.Suggestion
	filter := bson.M{"_id": id}

	err := r.collection.FindOne(ctx, filter).Decode(&suggestion)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Return the specific error defined in the package
			return nil, ErrSuggestionNotFound
		}
		return nil, fmt.Errorf("failed to find suggestion by ID %s: %w", id.Hex(), err)
	}
	return &suggestion, nil
}

// UpdateSuggestionStatus updates the status, reviewer ID, and reviewer username of a suggestion.
func (r *MongoSuggestionRepository) UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status string, reviewerID int64, reviewerUsername string) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"status":            status,
			"reviewed_by":       reviewerID,
			"reviewer_username": reviewerUsername,
			"reviewed_at":       time.Now(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update suggestion status for ID %s: %w", id.Hex(), err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("suggestion with ID %s not found for update", id.Hex()) // Or return nil? Depends on desired behavior.
	}

	return nil
}

// DeleteSuggestion removes a suggestion from the database by ID.
func (r *MongoSuggestionRepository) DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete suggestion with ID %s: %w", id.Hex(), err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("suggestion with ID %s not found for deletion", id.Hex()) // Or return nil?
	}
	return nil
}

// ResetDailyLimits is intended to reset daily submission counters if they exist in the Suggestion model.
// Currently, the Suggestion model doesn't have daily limit fields, so this is a placeholder.
func (r *MongoSuggestionRepository) ResetDailyLimits(ctx context.Context) error {
	// Placeholder implementation
	// If Suggestion model had fields like `daily_submissions`, this method would update them.
	// Example (hypothetical): update := bson.M{"$set": bson.M{"daily_submissions": 0}}
	// _, err := r.collection.UpdateMany(ctx, bson.M{}, update)
	// return err
	return nil // No-op for now
}
