package database

import (
	"context"
	"vrcmemes-bot/internal/database/models"

	"github.com/mymmrac/telego"
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

// SuggestionRepository defines the interface for suggestion data operations.
// This interface is defined in mongo_suggestion_repo.go or similar, removing redeclaration here.
/*
type SuggestionRepository interface {
	AddSuggestion(ctx context.Context, suggestion *models.Suggestion) (string, error)
	GetSuggestionByID(ctx context.Context, id string) (*models.Suggestion, error)
	UpdateSuggestionStatus(ctx context.Context, id string, status models.SuggestionStatus, adminID int64, adminUsername string) error
	GetPendingSuggestions(ctx context.Context) ([]models.Suggestion, error)
}
*/

// CaptionProvider defines the interface for retrieving captions.
type CaptionProvider interface {
	RetrieveMediaGroupCaption(groupID string) string
	GetActiveCaption(chatID int64) (string, bool)
	ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (processed bool, err error)
}

// CommandHandler defines the function signature for command handlers.
type CommandHandler func(ctx context.Context, bot *telego.Bot, message telego.Message) error

// HandlerProvider defines the interface for retrieving specific handlers.
type HandlerProvider interface {
	GetCommandHandler(command string) CommandHandler
	HandlePhoto(ctx context.Context, bot *telego.Bot, message telego.Message) error
	HandleText(ctx context.Context, bot *telego.Bot, message telego.Message) error
	// HandleVideo(ctx context.Context, bot *telego.Bot, message telego.Message) error // Placeholder for video
}

// FeedbackRepository defines the interface for feedback data operations.
type FeedbackRepository interface {
	AddFeedback(ctx context.Context, feedback *models.Feedback) error
}

// CallbackProcessor defines the interface for processing callback queries.
type CallbackProcessor interface {
	ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (processed bool, err error)
	// Add other callback processing methods if needed
}

// SuggestionManager defines the interface for handling suggestion interactions.
type SuggestionManager interface {
	HandleMessage(ctx context.Context, update telego.Update) (processed bool, err error)
	ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (processed bool, err error)
}
