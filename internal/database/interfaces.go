package database

import (
	"context"
	"vrcmemes-bot/internal/database/models"
	telegoapi "vrcmemes-bot/pkg/telegoapi" // Import telegoapi for BotAPI

	"github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
// Actual definition is likely in mongo_suggestion_repo.go or similar.
type SuggestionRepository interface {
	CreateSuggestion(ctx context.Context, suggestion *models.Suggestion) error
	GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error)
	UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status string, adminID int64, adminUsername string) error
	GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error)
	DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error
	ResetDailyLimits(ctx context.Context) error
	// Add other methods as needed
}

// CaptionProvider defines the interface for retrieving captions.
type CaptionProvider interface {
	RetrieveMediaGroupCaption(groupID string) string
	GetActiveCaption(chatID int64) (string, bool)
	// ProcessSuggestionCallback might be part of SuggestionManager interface now
}

// CommandHandler defines the function signature for command handlers.
// Match the signature used in handlers.MessageHandler.GetCommandHandler
type CommandHandler func(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error

// HandlerProvider defines the interface for retrieving specific handlers.
type HandlerProvider interface {
	// Use the exact type from handlers/handler.go
	GetCommandHandler(command string) func(context.Context, telegoapi.BotAPI, telego.Message) error
	HandlePhoto(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error
	HandleText(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error
	HandleVideo(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error
}

// FeedbackRepository defines the interface for feedback data operations.
type FeedbackRepository interface {
	AddFeedback(ctx context.Context, feedback *models.Feedback) error
}

// CallbackProcessor defines the interface for processing callback queries.
type CallbackProcessor interface {
	// Use SuggestionManagerInterface.HandleCallbackQuery instead
	// ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (processed bool, err error)
}

// SuggestionManager interface might be defined in handlers/interfaces.go now
/*
type SuggestionManager interface {
	HandleMessage(ctx context.Context, update telego.Update) (processed bool, err error)
	HandleCallbackQuery(ctx context.Context, query telego.CallbackQuery) (processed bool, err error)
}
*/
