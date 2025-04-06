package database

import (
	"context"
	"errors"
	"vrcmemes-bot/internal/database/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PostLogger defines methods for logging published posts
// ... (existing interface) ...

// UserActionLogger defines methods for logging user actions
// ... (existing interface) ...

// UserRepository defines methods for user data storage
// ... (existing interface) ...

// SuggestionRepository defines methods for suggestion data storage.
type SuggestionRepository interface {
	CreateSuggestion(ctx context.Context, suggestion *models.Suggestion) error
	GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error) // Returns suggestions, total count, error
	GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error)
	UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status string, reviewerID int64) error
	DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error
}

// ErrSuggestionNotFound is returned when a suggestion is not found.
var ErrSuggestionNotFound = errors.New("suggestion not found")

func LogUserAction(userID int64, actionType string, details map[string]interface{}) error {
	// Implementation of LogUserAction function
	return nil
}
