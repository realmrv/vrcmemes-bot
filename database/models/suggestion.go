package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Suggestion represents a user suggestion stored in the database.
type Suggestion struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"` // MongoDB default ID
	SuggesterID int64              `bson:"suggester_id"`
	Username    string             `bson:"username,omitempty"`
	FirstName   string             `bson:"first_name,omitempty"`
	MessageID   int                `bson:"message_id"`        // Original message ID in the bot chat
	ChatID      int64              `bson:"chat_id"`           // Chat ID where the suggestion was sent (bot chat)
	FileIDs     []string           `bson:"file_ids"`          // File IDs of the photos
	Caption     string             `bson:"caption,omitempty"` // User-provided caption
	Status      string             `bson:"status"`            // e.g., "pending", "approved", "rejected"
	SubmittedAt time.Time          `bson:"submitted_at"`
	ReviewedBy  int64              `bson:"reviewed_by,omitempty"` // Admin who reviewed it
	ReviewedAt  time.Time          `bson:"reviewed_at,omitempty"`
}

// SuggestionStatus defines the possible states of a suggestion.
type SuggestionStatus string

const (
	StatusPending  SuggestionStatus = "pending"
	StatusApproved SuggestionStatus = "approved"
	StatusRejected SuggestionStatus = "rejected"
)
