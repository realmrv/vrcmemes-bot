package handlers

import (
	"context"
	"log"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// sendSuccess sends a simple success message to the user.
func (h *MessageHandler) sendSuccess(ctx context.Context, bot *telego.Bot, chatID int64, text string) error {
	_, err := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	if err != nil {
		log.Printf("Failed to send success message to chat %d: %v", chatID, err)
	}
	return err // Return the error if sending failed
}

// Function isUserAdmin removed as it checked channel admin status, not bot admin status.
// Bot admin status is checked via suggestionManager.IsAdmin.
