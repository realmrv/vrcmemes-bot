package handlers

import (
	"context"
	"vrcmemes-bot/internal/suggestions" // Assuming UserState is defined here

	// Import telegoapi "vrcmemes-bot/pkg/telegoapi" // Not needed here anymore

	"github.com/mymmrac/telego"
)

// --- BotAPI removed, moved to pkg/telegoapi ---

// --- AdminCheckerInterface removed, moved to internal/auth ---

// SuggestionManagerInterface defines the interface for suggestion workflow operations used by MessageHandler.
type SuggestionManagerInterface interface {
	GetUserState(userID int64) suggestions.UserState
	SetUserState(userID int64, state suggestions.UserState) // Used internally? Check if needed here or only in mock. Let's include for now.
	HandleSuggestCommand(ctx context.Context, update telego.Update) error
	HandleReviewCommand(ctx context.Context, update telego.Update) error   // Assuming this method exists
	HandleFeedbackCommand(ctx context.Context, update telego.Update) error // Assuming this method exists
	HandleMessage(ctx context.Context, update telego.Update) (processed bool, err error)
	HandleCallbackQuery(ctx context.Context, query telego.CallbackQuery) (processed bool, err error) // Renamed from ProcessSuggestionCallback for consistency
	HandleCombinedMediaGroup(ctx context.Context, groupID string, messages []telego.Message) error   // Added based on usage in bot/bot.go

	// Add other methods like StartReviewSession etc. if called directly by MessageHandler
}

// Ensure MockSuggestionManager has SetUserState method if included here
// Ensure suggestions.Manager has HandleReviewCommand, HandleFeedbackCommand if included here
// Ensure suggestions.Manager has HandleCallbackQuery method
// Ensure suggestions.Manager has HandleCombinedMediaGroup method
