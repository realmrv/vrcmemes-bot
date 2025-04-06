package database

import (
	"context"
	"vrcmemes-bot/internal/database/models"
)

// PostLogger defines the interface for logging published posts.
type PostLogger interface {
	// LogPublishedPost logs information about a post published to the channel.
	LogPublishedPost(log models.PostLog) error
}

// UserActionLogger defines the interface for logging user actions.
type UserActionLogger interface {
	// LogUserAction logs an action performed by a user.
	LogUserAction(userID int64, action string, details interface{}) error
}

// UserRepository defines the interface for user data operations.
type UserRepository interface {
	// UpdateUser updates or creates a user record in the database.
	UpdateUser(ctx context.Context, userID int64, username, firstName, lastName string, isAdmin bool, action string) error
}
