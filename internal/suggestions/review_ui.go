package suggestions

import (
	"context"
	"fmt"
	"log"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/pkg/utils"

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
	previousData := fmt.Sprintf("review:%s:previous:%d", suggestionIDHex, suggestionIndex) // Data for previous button

	// Get localized button texts
	btnApproveText := locales.GetMessage(localizer, "BtnApprove", nil, nil)
	btnRejectText := locales.GetMessage(localizer, "BtnReject", nil, nil)
	btnNextText := locales.GetMessage(localizer, "BtnNext", nil, nil)
	btnPreviousText := locales.GetMessage(localizer, "BtnPrevious", nil, nil)

	keyboardRows := [][]telego.InlineKeyboardButton{
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnApproveText).WithCallbackData(approveData),
			tu.InlineKeyboardButton(btnRejectText).WithCallbackData(rejectData),
		),
	}

	// Add navigation row if needed
	navRow := []telego.InlineKeyboardButton{}
	if suggestionIndex > 0 {
		navRow = append(navRow, tu.InlineKeyboardButton(btnPreviousText).WithCallbackData(previousData))
	}
	if suggestionIndex+1 < totalSuggestionsInBatch {
		navRow = append(navRow, tu.InlineKeyboardButton(btnNextText).WithCallbackData(nextData))
	}
	if len(navRow) > 0 {
		keyboardRows = append(keyboardRows, navRow)
	}

	keyboard := &telego.InlineKeyboardMarkup{
		InlineKeyboard: keyboardRows,
	}

	var sentMediaMessages []*telego.Message // Store sent media messages
	var sentControlMessage *telego.Message  // Store sent control message
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

	// --- Sending Media and Control Message --- //
	if len(inputMedia) == 1 {
		// Send single photo/video directly
		var fileID string
		if photoInput, ok := inputMedia[0].(*telego.InputMediaPhoto); ok {
			fileID = photoInput.Media.FileID
		}
		// TODO: Handle other media types like video if necessary

		sendParams := &telego.SendPhotoParams{
			ChatID:      tu.ID(chatID),
			Photo:       telego.InputFile{FileID: fileID}, // Use extracted fileID
			Caption:     messageText,                      // Combine text with the single media message
			ParseMode:   telego.ModeMarkdownV2,
			ReplyMarkup: keyboard,
		}
		msg, sendErr := m.bot.SendPhoto(ctx, sendParams)
		if sendErr != nil {
			log.Printf("[SendReviewMessage] Error sending single review photo for suggestion %s to admin %d: %v", suggestionIDHex, adminID, sendErr)
			err = sendErr
		} else {
			sentMediaMessages = append(sentMediaMessages, msg) // Store the single message
			sentControlMessage = msg                           // In this case, the media message is also the control message
			log.Printf("[SendReviewMessage] Sent single media message ID %d (also control) for suggestion %s to admin %d", msg.MessageID, suggestionIDHex, adminID)
		}
	} else if len(inputMedia) > 1 {
		// Send media group first
		sentMediaGroupMessages, sendErr := m.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(chatID),
			Media:  inputMedia,
		})
		if sendErr != nil {
			log.Printf("[SendReviewMessage] Error sending review media group for suggestion %s to admin %d: %v", suggestionIDHex, adminID, sendErr)
			err = sendErr
		} else {
			// Send the control message separately after the media group
			controlMsg, sendErrCtrl := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), messageText).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
			if sendErrCtrl != nil {
				log.Printf("[SendReviewMessage] Error sending control message for suggestion %s to admin %d after media group: %v", suggestionIDHex, adminID, sendErrCtrl)
				err = sendErrCtrl // Report the error from sending the control message
			} else {
				// Convert []telego.Message to []*telego.Message
				sentMediaPtrs := make([]*telego.Message, len(sentMediaGroupMessages))
				for i := range sentMediaGroupMessages {
					sentMediaPtrs[i] = &sentMediaGroupMessages[i]
				}
				sentMediaMessages = sentMediaPtrs // Store pointers
				sentControlMessage = controlMsg   // Store the separate control message
				log.Printf("[SendReviewMessage] Media group (%d msgs) sent, followed by control message ID %d", len(sentMediaGroupMessages), controlMsg.MessageID)
			}
		}
	}
	// --- End Sending Media and Control Message --- //

	if err != nil {
		// Handle error during sending
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("[SendReviewMessage] Error sending general error message after send failure: %v", sendErr)
		}
		return err // Return the original sending error
	}

	// --- Update Session --- //
	m.reviewSessionsMutex.Lock()
	if currentSession, exists := m.reviewSessions[adminID]; exists {
		if sentControlMessage != nil { // Check if control message was successfully sent
			// Collect IDs of sent media messages
			mediaIDs := make([]int, 0, len(sentMediaMessages))
			for _, msg := range sentMediaMessages {
				if msg != nil {
					mediaIDs = append(mediaIDs, msg.MessageID)
				}
			}

			currentSession.CurrentMediaMessageIDs = mediaIDs
			currentSession.CurrentControlMessageID = sentControlMessage.MessageID
			currentSession.CurrentIndex = suggestionIndex
			currentSession.LastReviewMessageID = 0 // Mark old field as unused (or remove it later)
			log.Printf("[SendReviewMessage] Stored MediaIDs: %v, ControlID: %d for session of admin %d", mediaIDs, sentControlMessage.MessageID, adminID)
		} else {
			log.Printf("[SendReviewMessage] Control message was not sent successfully, cannot update session for admin %d", adminID)
		}
	} else {
		log.Printf("[SendReviewMessage] Session for admin %d disappeared before storing message IDs.", adminID)
	}
	m.reviewSessionsMutex.Unlock()
	// --- End Update Session --- //

	return nil
}

// deleteReviewMessages attempts to delete the review prompt message and associated media messages.
func (m *Manager) deleteReviewMessages(ctx context.Context, chatID int64, mediaMessageIDs []int, controlMessageID int) {
	// Delete media messages first
	for _, msgID := range mediaMessageIDs {
		if msgID != 0 {
			err := m.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
				ChatID:    tu.ID(chatID),
				MessageID: msgID,
			})
			if err != nil {
				// Log error but continue trying to delete others
				log.Printf("[deleteReviewMessages] Failed to delete media message %d in chat %d: %v", msgID, chatID, err)
			} else {
				log.Printf("[deleteReviewMessages] Deleted media message %d in chat %d", msgID, chatID)
			}
		}
	}

	// Delete control message
	if controlMessageID != 0 {
		err := m.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
			ChatID:    tu.ID(chatID),
			MessageID: controlMessageID,
		})
		if err != nil {
			log.Printf("[deleteReviewMessages] Failed to delete control message %d in chat %d: %v", controlMessageID, chatID, err)
		} else {
			log.Printf("[deleteReviewMessages] Deleted control message %d in chat %d", controlMessageID, chatID)
		}
	}
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
		"FirstName": utils.EscapeMarkdownV2(suggestion.FirstName),
		"Username":  usernameDisplay, // Username part is already handled
		"UserID":    suggestion.SuggesterID,
	}, nil)

	// Prepare caption text
	var captionContent string
	if suggestion.Caption != "" {
		captionContent = utils.EscapeMarkdownV2(suggestion.Caption)
	} else {
		captionContent = locales.GetMessage(localizer, "MsgReviewNoCaptionPlaceholder", nil, nil)
	}
	// Get the prefix like "Caption: " or "Подпись: " using the new key
	captionPrefix := locales.GetMessage(localizer, "MsgReviewCaptionPrefix", nil, nil)
	// Manually construct the final line, ensuring content is escaped
	captionLine := captionPrefix + " " + captionContent // Add space manually

	return fmt.Sprintf("%s\n%s\n%s", indexText, fromText, captionLine)
}
