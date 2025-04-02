package suggestions

import (
	"context"
	"fmt"
	"log"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// SendReviewMessage sends a message to the admin with the suggestion details and action buttons.
func (m *Manager) SendReviewMessage(ctx context.Context, chatID, adminID int64, suggestionIndex int) error {
	m.reviewSessionsMutex.RLock()
	session, ok := m.reviewSessions[adminID]
	m.reviewSessionsMutex.RUnlock()

	if !ok || suggestionIndex < 0 || suggestionIndex >= len(session.Suggestions) {
		log.Printf("[SendReviewMessage] Invalid session or index for admin %d, index %d", adminID, suggestionIndex)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), locales.MsgReviewQueueIsEmpty))
		m.reviewSessionsMutex.Lock()
		delete(m.reviewSessions, adminID)
		m.reviewSessionsMutex.Unlock()
		return err
	}

	suggestion := session.Suggestions[suggestionIndex]
	totalSuggestionsInBatch := len(session.Suggestions)

	indexText := fmt.Sprintf(locales.MsgReviewCurrentSuggestionIndex, suggestionIndex+1, totalSuggestionsInBatch)
	submitterUsername := suggestion.Username
	if submitterUsername == "" {
		submitterUsername = "(no username)"
	} else {
		submitterUsername = escapeMarkdownV2(submitterUsername)
	}
	submitterFirstName := escapeMarkdownV2(suggestion.FirstName)
	// Escape the literal parentheses in the format string for MarkdownV2
	submitterInfo := fmt.Sprintf("From: %s \\(@%s, ID: %d\\)", submitterFirstName, submitterUsername, suggestion.SuggesterID)
	captionText := "(No Caption)"
	if suggestion.Caption != "" {
		captionText = escapeMarkdownV2(suggestion.Caption)
	}
	messageText := fmt.Sprintf("%s\n%s\nCaption: `%s`", indexText, submitterInfo, captionText)

	inputMedia := m.createInputMediaFromSuggestion(suggestion)
	if len(inputMedia) == 0 {
		log.Printf("[SendReviewMessage] No valid media found for suggestion ID %s", suggestion.ID.Hex())
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "*Error:* Could not load media for suggestion `"+suggestion.ID.Hex()+"`.").WithParseMode(telego.ModeMarkdownV2))
		return err
	}

	suggestionIDHex := suggestion.ID.Hex()
	approveData := fmt.Sprintf("review:%s:approve:%d", suggestionIDHex, suggestionIndex)
	rejectData := fmt.Sprintf("review:%s:reject:%d", suggestionIDHex, suggestionIndex)
	nextData := fmt.Sprintf("review:%s:next:%d", suggestionIDHex, suggestionIndex)

	keyboard := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("✅ Approve").WithCallbackData(approveData),
			tu.InlineKeyboardButton("❌ Reject").WithCallbackData(rejectData),
		),
	)

	if suggestionIndex+1 < totalSuggestionsInBatch {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("➡️ Next").WithCallbackData(nextData),
		))
	}

	var sentMessage *telego.Message
	var err error

	// Prepare the first media item with the original caption if it exists
	if len(inputMedia) > 0 {
		if photo, ok := inputMedia[0].(*telego.InputMediaPhoto); ok {
			if suggestion.Caption != "" {
				photo.Caption = suggestion.Caption // Keep original caption for the media group itself
				// Consider escaping for the specific ParseMode if you set one for the media
			}
		} else if video, ok := inputMedia[0].(*telego.InputMediaVideo); ok {
			if suggestion.Caption != "" {
				video.Caption = suggestion.Caption
				// Consider escaping
			}
		}
		// Do NOT set messageText or ParseMode here for the media group itself
	}

	// Send the media group first
	_, err = m.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(chatID),
		Media:  inputMedia,
	})

	if err != nil {
		log.Printf("[SendReviewMessage] Error sending review media group for suggestion %s to admin %d: %v", suggestionIDHex, adminID, err)
	} else {
		// If media group sent successfully, send the control message with text and buttons
		sentMessage, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), messageText).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
		if err != nil {
			log.Printf("[SendReviewMessage] Error sending control message for suggestion %s to admin %d after media group: %v", suggestionIDHex, adminID, err)
		} else {
			log.Printf("[SendReviewMessage] Media group sent, followed by control message ID %d", sentMessage.MessageID)
		}
	}

	if err != nil {
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), locales.MsgErrorGeneral))
		if sendErr != nil {
			log.Printf("[SendReviewMessage] Error sending general error message after send failure: %v", sendErr)
		}
		return err
	}

	// Store the message ID of the control message
	m.reviewSessionsMutex.Lock()
	if currentSession, exists := m.reviewSessions[adminID]; exists {
		// Ensure sentMessage is not nil before accessing MessageID
		if sentMessage != nil {
			currentSession.LastReviewMessageID = sentMessage.MessageID
			currentSession.CurrentIndex = suggestionIndex
			log.Printf("[SendReviewMessage] Sent suggestion %d (ID: %s) for review to admin %d. Control MessageID: %d", suggestionIndex+1, suggestionIDHex, adminID, sentMessage.MessageID)
		} else {
			log.Printf("[SendReviewMessage] Control message was not sent successfully, cannot store MessageID for session of admin %d", adminID)
		}
	} else {
		log.Printf("[SendReviewMessage] Session for admin %d disappeared before storing message ID.", adminID)
	}
	m.reviewSessionsMutex.Unlock()

	return nil
}

// deleteReviewMessage attempts to delete the review prompt message.
func (m *Manager) deleteReviewMessage(ctx context.Context, chatID int64, messageID int) error {
	if messageID == 0 {
		log.Printf("[deleteReviewMessage] Cannot delete message, ID is 0 for chat %d", chatID)
		return nil
	}
	err := m.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
	})
	if err != nil {
		log.Printf("[deleteReviewMessage] Failed to delete review message %d in chat %d: %v", messageID, chatID, err)
		return err
	}
	log.Printf("[deleteReviewMessage] Deleted review message %d in chat %d", messageID, chatID)
	return nil
}

// answerCallbackQuery is a helper to answer callback queries.
func (m *Manager) answerCallbackQuery(ctx context.Context, queryID string, text string, showAlert bool) error {
	err := m.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: queryID,
		Text:            text,
		ShowAlert:       showAlert,
	})
	if err != nil {
		log.Printf("Error answering callback query %s: %v", queryID, err)
	}
	return err
}

// createInputMediaFromSuggestion converts suggestion FileIDs to telego.InputMedia.
func (m *Manager) createInputMediaFromSuggestion(suggestion models.Suggestion) []telego.InputMedia {
	var inputMedia []telego.InputMedia
	maxItems := len(suggestion.FileIDs)
	if maxItems > 10 {
		log.Printf("[createInputMediaFromSuggestion] Suggestion %s has more than 10 photos (%d), truncating.", suggestion.ID.Hex(), maxItems)
		maxItems = 10
	}

	for i := 0; i < maxItems; i++ {
		fileID := suggestion.FileIDs[i]
		mediaPhoto := &telego.InputMediaPhoto{
			Type:  "photo",
			Media: telego.InputFile{FileID: fileID},
		}
		inputMedia = append(inputMedia, mediaPhoto)
	}
	return inputMedia
}
