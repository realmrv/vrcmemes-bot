package models

import "time"

// PostLog stores information about a post published to the channel.
type PostLog struct {
	SenderID             int64     `bson:"sender_id"`
	SenderUsername       string    `bson:"sender_username,omitempty"`
	Caption              string    `bson:"caption,omitempty"`
	MessageType          string    `bson:"message_type"` // e.g., "media_group", "photo", "video"
	ReceivedAt           time.Time `bson:"received_at"`
	PublishedAt          time.Time `bson:"published_at"`
	ChannelID            int64     `bson:"channel_id"`
	ChannelPostID        int       `bson:"channel_post_id"`
	OriginalMessageID    int       `bson:"original_message_id,omitempty"`     // For single messages
	OriginalMediaGroupID string    `bson:"original_media_group_id,omitempty"` // For media groups
}
