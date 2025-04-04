package suggestions

import (
	"context"
	"fmt"
	"log"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// SendReviewMessage sends a message to the admin with the suggestion details and action buttons.
func (m *Manager) SendReviewMessage(ctx context.Context, chatID, adminID int64, suggestionIndex int) error {
	m.reviewSessionsMutex.RLock()
	session, ok := m.reviewSessions[adminID]
	m.reviewSessionsMutex.RUnlock()

	if !ok || suggestionIndex < 0 || suggestionIndex >= len(session.Suggestions) {
		log.Printf("[SendReviewMessage] Invalid session or index for admin %d, index %d", adminID, suggestionIndex)
		lang := locales.DefaultLanguage
		localizer := locales.NewLocalizer(lang)
		msg := locales.GetMessage(localizer, "MsgReviewQueueIsEmpty", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		m.reviewSessionsMutex.Lock()
		delete(m.reviewSessions, adminID)
		m.reviewSessionsMutex.Unlock()
		return err
	}

	suggestion := session.Suggestions[suggestionIndex]
	totalSuggestionsInBatch := len(session.Suggestions)

	lang := locales.DefaultLanguage
	localizer := locales.NewLocalizer(lang)

	// Build the message text using the dedicated helper function
	messageText := m.buildReviewMessageText(localizer, &suggestion, suggestionIndex, totalSuggestionsInBatch)

	inputMedia := m.createInputMediaFromSuggestion(suggestion)
	if len(inputMedia) == 0 {
		log.Printf("[SendReviewMessage] No valid media found for suggestion ID %s", suggestion.ID.Hex())
		errorMsg := locales.GetMessage(localizer, "MsgReviewErrorLoadMedia", map[string]interface{}{
			"SuggestionID": suggestion.ID.Hex(),
		}, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg).WithParseMode(telego.ModeMarkdownV2))
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

	if len(inputMedia) > 0 {
		if photo, ok := inputMedia[0].(*telego.InputMediaPhoto); ok {
			if suggestion.Caption != "" {
				photo.Caption = suggestion.Caption
			}
		} else if video, ok := inputMedia[0].(*telego.InputMediaVideo); ok {
			if suggestion.Caption != "" {
				video.Caption = suggestion.Caption
			}
		}
	}

	_, err = m.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(chatID),
		Media:  inputMedia,
	})

	if err != nil {
		log.Printf("[SendReviewMessage] Error sending review media group for suggestion %s to admin %d: %v", suggestionIDHex, adminID, err)
	} else {
		sentMessage, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), messageText).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
		if err != nil {
			log.Printf("[SendReviewMessage] Error sending control message for suggestion %s to admin %d after media group: %v", suggestionIDHex, adminID, err)
		} else {
			log.Printf("[SendReviewMessage] Media group sent, followed by control message ID %d", sentMessage.MessageID)
		}
	}

	if err != nil {
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("[SendReviewMessage] Error sending general error message after send failure: %v", sendErr)
		}
		return err
	}

	m.reviewSessionsMutex.Lock()
	if currentSession, exists := m.reviewSessions[adminID]; exists {
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

// buildReviewMessageText formats the text for the review message.
func (m *Manager) buildReviewMessageText(localizer *i18n.Localizer, suggestion *models.Suggestion, index, total int) string {
	indexText := locales.GetMessage(localizer, "MsgReviewCurrentSuggestionIndex", map[string]interface{}{
		"Index": index + 1, // User-friendly 1-based index
		"Total": total,
	}, nil)

	// Prepare username display
	var usernameDisplay string
	if suggestion.Username != "" {
		// Don't escape the @, but escape the username itself if it contains special chars?
		// For now, assume usernames are safe or handle specific cases if needed.
		usernameDisplay = suggestion.Username
	} else {
		usernameDisplay = locales.GetMessage(localizer, "MsgReviewNoUsernamePlaceholder", nil, nil)
	}

	fromText := locales.GetMessage(localizer, "MsgReviewFrom", map[string]interface{}{
		"FirstName": escapeMarkdownV2(suggestion.FirstName),
		"Username":  usernameDisplay, // Username part is already handled
		"UserID":    suggestion.SuggesterID,
	}, nil)

	// Prepare caption text
	var captionContent string
	if suggestion.Caption != "" {
		captionContent = escapeMarkdownV2(suggestion.Caption)
	} else {
		captionContent = locales.GetMessage(localizer, "MsgReviewNoCaptionPlaceholder", nil, nil)
	}
	// Get the prefix like "Caption: " or "Подпись: " using the new key
	captionPrefix := locales.GetMessage(localizer, "MsgReviewCaptionPrefix", nil, nil)
	// Manually construct the final line, ensuring content is escaped
	captionLine := captionPrefix + " " + captionContent // Add space manually

	return fmt.Sprintf("%s\n%s\n%s", indexText, fromText, captionLine)
}
