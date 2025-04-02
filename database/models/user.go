package models

import "time"

// User represents a Telegram user with their activity information
type User struct {
	UserID       int64     `bson:"user_id"`
	Username     string    `bson:"username,omitempty"`
	FirstName    string    `bson:"first_name,omitempty"`
	LastName     string    `bson:"last_name,omitempty"`
	IsAdmin      bool      `bson:"is_admin"`
	FirstSeen    time.Time `bson:"first_seen"`
	LastSeen     time.Time `bson:"last_seen"`
	ActionsCount int       `bson:"actions_count"`
	LastAction   string    `bson:"last_action"`
}
