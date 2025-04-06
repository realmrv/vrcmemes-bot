package suggestions

import (
	"vrcmemes-bot/internal/database/models"
)

// SuggestionStatus defines the possible states of a suggestion.
type SuggestionStatus string

const (
	StatusPending  SuggestionStatus = "pending"
	StatusApproved SuggestionStatus = "approved"
	StatusRejected SuggestionStatus = "rejected"
)

// UserState represents the current state of a user interaction regarding suggestions.
type UserState string

const (
	StateIdle               UserState = ""                    // Default state
	StateAwaitingSuggestion UserState = "awaiting_suggestion" // Bot is waiting for the user to send suggestion content
	StateAwaitingFeedback   UserState = "awaiting_feedback"   // Bot is waiting for the user to send feedback content
)

// ReviewSession stores the state for an admin's review process.
type ReviewSession struct {
	AdminID                 int64               // ID of the admin performing the review
	ReviewChatID            int64               // ID of the chat where the review messages are sent
	Suggestions             []models.Suggestion // The batch of suggestions being reviewed
	CurrentIndex            int                 // Index of the suggestion currently being viewed
	CurrentMediaMessageIDs  []int               // IDs of the messages containing the media being reviewed
	CurrentControlMessageID int                 // ID of the message containing the Approve/Reject/Next buttons
	LastReviewMessageID     int                 // Message ID of the last sent review prompt
}

// Note: The 'Suggestion' struct defined in the original file seems like a local representation
// and might differ from models.Suggestion used in ReviewSession.
// If 'Suggestion' was intended for internal use within the manager,
// it might need to stay in manager.go or be adjusted.
// For now, I'm assuming the models.Suggestion from the database package is sufficient.
// If the original 'Suggestion' struct (lines 23-31) is needed, it should be added here too.
