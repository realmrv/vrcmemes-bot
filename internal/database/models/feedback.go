package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Feedback represents a feedback submission from a user.
type Feedback struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	UserID         int64              `bson:"user_id"`
	Username       string             `bson:"username,omitempty"`
	FirstName      string             `bson:"first_name,omitempty"`
	Text           string             `bson:"text"`
	PhotoIDs       []string           `bson:"photo_ids,omitempty"` // File IDs of attached photos
	VideoIDs       []string           `bson:"video_ids,omitempty"` // File IDs of attached videos
	MediaGroupID   string             `bson:"media_group_id,omitempty"`
	SubmittedAt    time.Time          `bson:"submitted_at"`
	OriginalChatID int64              `bson:"original_chat_id"`
	MessageID      int                `bson:"message_id"`
}
