package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// sendError logs an error and sends a generic error message to the specified chat.
// It returns the error encountered while trying to send the message, or nil if successful.
func (h *MessageHandler) sendError(ctx context.Context, bot *telego.Bot, chatID int64, err error) error {
	// Log the original error with more details
	log.Printf("ERROR for chat %d: %v", chatID, err)

	// Send a user-friendly message
	_, sendErr := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), fmt.Sprintf("❌ An error occurred: %v", err)))
	if sendErr != nil {
		// Log the error encountered while sending the error message itself
		log.Printf("Failed to send error message to chat %d: %v", chatID, sendErr)
	}
	// Return the error from sending the message, as this indicates if the user was notified.
	return sendErr
}

// sendSuccess sends a standard success message to the specified chat.
// It logs an error if sending the message fails.
// It returns the error encountered while trying to send the message, or nil if successful.
func (h *MessageHandler) sendSuccess(ctx context.Context, bot *telego.Bot, chatID int64, message string) error {
	_, err := bot.SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		message,
	))
	if err != nil {
		log.Printf("Error sending success message to chat %d: %v", chatID, err)
	}
	// Return the error to indicate if the message sending failed.
	return err
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
