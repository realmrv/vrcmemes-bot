package handlers

import (
	"context"
	"fmt"
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

// isUserAdmin checks if the user has administrative privileges in the target channel.
// It fetches the chat member information for the user in the configured channel ID.
// Returns true if the user is a creator or administrator, false otherwise.
// Returns an error if the check fails (e.g., network issue, bot permissions).
func (h *MessageHandler) isUserAdmin(ctx context.Context, bot *telego.Bot, userID int64) (bool, error) {
	// Perform the check against the configured channel ID
	member, err := bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: tu.ID(h.channelID),
		UserID: userID,
	})
	if err != nil {
		// Wrap the error for context
		return false, fmt.Errorf("failed to get chat member info for user %d in channel %d: %w", userID, h.channelID, err)
	}

	// Check the member status using the MemberStatus() method
	status := member.MemberStatus()
	isAdmin := status == telego.MemberStatusCreator || status == telego.MemberStatusAdministrator
	log.Printf("Admin check for user %d in channel %d: Status=%s, IsAdmin=%t", userID, h.channelID, status, isAdmin) // Log the result
	return isAdmin, nil
}
