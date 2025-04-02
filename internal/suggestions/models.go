package suggestions

import (
	"vrcmemes-bot/database/models" // Keep models import if ReviewSession uses it
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
)

// ReviewSession stores the state for an admin reviewing suggestions.
type ReviewSession struct {
	Suggestions         []models.Suggestion // The batch of suggestions being reviewed
	CurrentIndex        int                 // Index of the suggestion currently shown
	LastReviewMessageID int                 // Message ID of the last review prompt sent
}

// Note: The 'Suggestion' struct defined in the original file seems like a local representation
// and might differ from models.Suggestion used in ReviewSession.
// If 'Suggestion' was intended for internal use within the manager,
// it might need to stay in manager.go or be adjusted.
// For now, I'm assuming the models.Suggestion from the database package is sufficient.
// If the original 'Suggestion' struct (lines 23-31) is needed, it should be added here too.
