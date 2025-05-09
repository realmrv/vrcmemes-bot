package suggestions

import (
	"context"
	"fmt"
	"log"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/pkg/utils"

	"github.com/getsentry/sentry-go" // Import Sentry
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// SendReviewMessage sends a message to the admin with the suggestion details and action buttons.
// If sending media fails, it sends an error message with controls instead.
func (m *Manager) SendReviewMessage(ctx context.Context, chatID, adminID int64, suggestionIndex int) error {
	m.reviewSessionsMutex.RLock()
	session, ok := m.reviewSessions[adminID]
	m.reviewSessionsMutex.RUnlock()

	if !ok || suggestionIndex < 0 || suggestionIndex >= len(session.Suggestions) {
		log.Printf("[SendReviewMessage] Invalid session or index for admin %d, index %d", adminID, suggestionIndex)
		lang := locales.GetDefaultLanguageTag().String()
		localizer := locales.NewLocalizer(lang)
		msgText := locales.GetMessage(localizer, "MsgReviewQueueIsEmpty", nil, nil) // Use msgText for clarity
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msgText))
		m.reviewSessionsMutex.Lock()
		delete(m.reviewSessions, adminID)
		m.reviewSessionsMutex.Unlock()
		return err
	}

	suggestion := session.Suggestions[suggestionIndex]
	totalSuggestionsInBatch := len(session.Suggestions)

	lang := locales.GetDefaultLanguageTag().String() // Admin's language, can be made more flexible
	localizer := locales.NewLocalizer(lang)

	messageText := m.buildReviewMessageText(localizer, &suggestion, suggestionIndex, totalSuggestionsInBatch)
	suggestionIDHex := suggestion.ID.Hex()

	// --- Keyboard ---
	approveData := fmt.Sprintf("review:%s:approve:%d", suggestionIDHex, suggestionIndex)
	rejectData := fmt.Sprintf("review:%s:reject:%d", suggestionIDHex, suggestionIndex)
	nextData := fmt.Sprintf("review:%s:next:%d", suggestionIDHex, suggestionIndex)
	previousData := fmt.Sprintf("review:%s:previous:%d", suggestionIDHex, suggestionIndex)

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
	// --- End Keyboard ---

	var sentMediaMessages []*telego.Message
	var sentControlMessage *telego.Message
	var mediaSendError error

	inputMedia := m.createInputMediaFromSuggestion(suggestion)

	if len(inputMedia) == 0 {
		// If no media initially (e.g., text suggestion, though current logic doesn't assume this)
		// or error in createInputMediaFromSuggestion
		log.Printf("[SendReviewMessage] No valid media found for suggestion ID %s. Sending text only.", suggestionIDHex)
		// Send only text message with controls
		controlMsg, errCtrl := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), messageText).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
		if errCtrl != nil {
			log.Printf("[SendReviewMessage] Error sending text-only review message for suggestion %s: %v", suggestionIDHex, errCtrl)
			sentry.CaptureException(fmt.Errorf("failed to send text-only review message for suggestion %s: %w", suggestionIDHex, errCtrl))
			// Notify user of general error if even this fails
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
			return errCtrl // Return error as failed to send even control message
		}
		sentControlMessage = controlMsg
	} else if len(inputMedia) == 1 {
		var fileID string
		if photoInput, ok := inputMedia[0].(*telego.InputMediaPhoto); ok {
			fileID = photoInput.Media.FileID
		}
		// TODO: Handle other media types like video if necessary

		sendParams := &telego.SendPhotoParams{
			ChatID:      tu.ID(chatID),
			Photo:       telego.InputFile{FileID: fileID},
			Caption:     messageText,
			ParseMode:   telego.ModeMarkdownV2,
			ReplyMarkup: keyboard,
		}
		msg, err := m.bot.SendPhoto(ctx, sendParams)
		if err != nil {
			log.Printf("[SendReviewMessage] Error sending single review photo for suggestion %s to admin %d: %v", suggestionIDHex, adminID, err)
			mediaSendError = err // Save media send error
		} else {
			sentMediaMessages = append(sentMediaMessages, msg)
			sentControlMessage = msg
			log.Printf("[SendReviewMessage] Sent single media message ID %d (also control) for suggestion %s to admin %d", msg.MessageID, suggestionIDHex, adminID)
		}
	} else { // len(inputMedia) > 1
		// First, send media group without caption and keyboard
		bareMediaGroup := make([]telego.InputMedia, len(inputMedia))
		for i, item := range inputMedia {
			switch media := item.(type) {
			case *telego.InputMediaPhoto:
				bareMediaGroup[i] = &telego.InputMediaPhoto{Type: media.Type, Media: media.Media} // Copy without Caption
			case *telego.InputMediaVideo:
				bareMediaGroup[i] = &telego.InputMediaVideo{Type: media.Type, Media: media.Media} // Copy without Caption
				// Add other media types if supported
			default:
				log.Printf("[SendReviewMessage] Unsupported media type in media group for suggestion %s", suggestionIDHex)
				mediaSendError = fmt.Errorf("unsupported media type in group for suggestion %s", suggestionIDHex)
				break // Exit loop if type is not supported
			}
		}

		if mediaSendError == nil { // Proceed only if no media type error
			groupMessages, err := m.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
				ChatID: tu.ID(chatID),
				Media:  bareMediaGroup, // Send media group without captions
			})
			if err != nil {
				log.Printf("[SendReviewMessage] Error sending review media group for suggestion %s to admin %d: %v", suggestionIDHex, adminID, err)
				mediaSendError = err // Save media send error
			} else {
				for i := range groupMessages { // Convert to []*telego.Message
					sentMediaMessages = append(sentMediaMessages, &groupMessages[i])
				}
				// Then send a separate message with text and keyboard
				controlMsg, errCtrl := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), messageText).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
				if errCtrl != nil {
					log.Printf("[SendReviewMessage] Error sending control message for suggestion %s after media group: %v", suggestionIDHex, errCtrl)
					// If media group sent but control message failed - this is a problem.
					// Record error, but try to update session with media ID at least.
					mediaSendError = fmt.Errorf("media group sent, but control message failed for %s: %w", suggestionIDHex, errCtrl)
				} else {
					sentControlMessage = controlMsg
					log.Printf("[SendReviewMessage] Media group (%d msgs) sent, followed by control message ID %d", len(sentMediaMessages), controlMsg.MessageID)
				}
			}
		}
	}

	// --- Handle media send error ---
	if mediaSendError != nil {
		sentry.CaptureException(fmt.Errorf("media send error for suggestion %s (admin %d): %w", suggestionIDHex, adminID, mediaSendError))

		// Delete already sent media if any (e.g., part of a group)
		// This is important to avoid orphaned media without a control message
		if len(sentMediaMessages) > 0 {
			tempMediaIDs := make([]int, 0, len(sentMediaMessages))
			for _, smm := range sentMediaMessages {
				if smm != nil {
					tempMediaIDs = append(tempMediaIDs, smm.MessageID)
				}
			}
			// Delete without waiting and without interrupting the main flow if it fails
			go func() {
				m.deleteReviewMessages(context.Background(), chatID, tempMediaIDs, 0)
				log.Printf("[SendReviewMessage] Attempted to clean up %d media messages after send error for suggestion %s", len(tempMediaIDs), suggestionIDHex)
			}()
			sentMediaMessages = nil // Clear as they will be or were deleted
		}

		// Send error message about media display WITH KEYBOARD
		// Use messageText (which contains description) + error message
		errorNotificationTextKey := "MsgReviewErrorDisplayingMedia" // New localization string
		rawErrorNotificationText := locales.GetMessage(localizer, errorNotificationTextKey, map[string]interface{}{"SuggestionID": suggestionIDHex}, nil)
		errorNotificationText := utils.EscapeMarkdownV2(rawErrorNotificationText) // <<<< ESCAPE THIS PART

		// Form text that DEFINITELY won't cause Markdown issues
		// First error text, then main suggestion text (already escaped)
		// Important: messageText already contains \n newlines and is escaped for MarkdownV2
		finalErrorTextPayload := errorNotificationText + "\n\n" + messageText

		// Try to send error message with controls
		errorDisplayMsg, errSendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), finalErrorTextPayload).WithReplyMarkup(keyboard).WithParseMode(telego.ModeMarkdownV2))
		if errSendErr != nil {
			log.Printf("[SendReviewMessage] CRITICAL: Failed to send media error notification for suggestion %s: %v", suggestionIDHex, errSendErr)
			sentry.CaptureException(fmt.Errorf("CRITICAL: failed to send media error notification for %s: %w", suggestionIDHex, errSendErr))
			// If even this fails, send the most generic message without anything
			fallbackErrorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), fallbackErrorMsg))
			// In this case, return error as admin is left without controls
			return fmt.Errorf("failed to send any review message for suggestion %s: original media error: %v, fallback error: %v", suggestionIDHex, mediaSendError, errSendErr)
		}
		// If error message with controls sent, consider this a "success" from UI perspective
		sentControlMessage = errorDisplayMsg
		log.Printf("[SendReviewMessage] Sent media display error notification with controls for suggestion %s, message ID %d", suggestionIDHex, errorDisplayMsg.MessageID)
	}
	// --- End handle media send error ---

	// --- Update Session --- //
	// Update session only if a control message was sent (normal or error message with controls)
	if sentControlMessage != nil {
		m.reviewSessionsMutex.Lock()
		if currentSession, exists := m.reviewSessions[adminID]; exists {
			mediaIDs := make([]int, 0, len(sentMediaMessages)) // sentMediaMessages can be nil if there was an error
			for _, msg := range sentMediaMessages {
				if msg != nil {
					mediaIDs = append(mediaIDs, msg.MessageID)
				}
			}
			currentSession.CurrentMediaMessageIDs = mediaIDs // Will be empty if media didn't send or were deleted
			currentSession.CurrentControlMessageID = sentControlMessage.MessageID
			currentSession.CurrentIndex = suggestionIndex
			log.Printf("[SendReviewMessage] Stored MediaIDs: %v, ControlID: %d for session of admin %d", mediaIDs, sentControlMessage.MessageID, adminID)
		} else {
			log.Printf("[SendReviewMessage] Session for admin %d disappeared before storing message IDs.", adminID)
			// If session disappeared and message was sent, try to delete this "orphaned" control message
			// This is unlikely, but for cleanliness
			if sentControlMessage.MessageID != 0 {
				go m.bot.DeleteMessage(context.Background(), &telego.DeleteMessageParams{ChatID: tu.ID(chatID), MessageID: sentControlMessage.MessageID})
			}
		}
		m.reviewSessionsMutex.Unlock()
	} else {
		// This situation should not occur if logic above is correct
		// (i.e., either sentControlMessage is set, or an error is returned)
		log.Printf("[SendReviewMessage] CRITICAL: No control message was sent and no error returned for suggestion %s, admin %d. This should not happen.", suggestionIDHex, adminID)
		sentry.CaptureException(fmt.Errorf("critical logic flaw: no control message and no error for suggestion %s, admin %d", suggestionIDHex, adminID))
		// Send general error message as admin is left without anything
		fallbackErrorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), fallbackErrorMsg))
		// And return error to interrupt
		return fmt.Errorf("no control message sent for suggestion %s", suggestionIDHex)
	}

	return nil // Do not return error here to not interrupt /review
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

	log.Printf("[createInputMediaFromSuggestion] Processing FileIDs for suggestion %s: %v", suggestion.ID.Hex(), suggestion.FileIDs) // Log the FileIDs

	for i := 0; i < maxItems; i++ {
		fileID := suggestion.FileIDs[i]
		if fileID == "" {
			log.Printf("[createInputMediaFromSuggestion] Warning: Empty FileID at index %d for suggestion %s", i, suggestion.ID.Hex())
			continue // Skip empty file IDs
		}
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
	// Part 1: Index text
	// Get raw localized string
	rawIndexText := locales.GetMessage(localizer, "MsgReviewCurrentSuggestionIndex", map[string]interface{}{
		"Index": index + 1, // User-friendly 1-based index
		"Total": total,
	}, nil)
	// Escape the entire localized string
	escapedIndexText := utils.EscapeMarkdownV2(rawIndexText)

	// Part 2: From text
	// Use raw user-provided FirstName and Username for interpolation
	var rawUsernameDisplay string
	if suggestion.Username != "" {
		rawUsernameDisplay = suggestion.Username
	} else {
		// Get raw placeholder from locale
		rawUsernameDisplay = locales.GetMessage(localizer, "MsgReviewNoUsernamePlaceholder", nil, nil)
	}

	// Get raw localized "From" text using raw user data
	rawFromText := locales.GetMessage(localizer, "MsgReviewFrom", map[string]interface{}{
		"FirstName": suggestion.FirstName, // Raw
		"Username":  rawUsernameDisplay,   // Raw
		"UserID":    suggestion.SuggesterID,
	}, nil)
	// Escape the entire localized "From" string
	escapedFromText := utils.EscapeMarkdownV2(rawFromText)

	// Part 3: Caption text
	var rawCaptionContent string
	if suggestion.Caption != "" {
		rawCaptionContent = suggestion.Caption // Raw
	} else {
		// Get raw placeholder text from locale for no caption
		rawCaptionContent = locales.GetMessage(localizer, "MsgReviewNoCaptionPlaceholder", nil, nil)
	}

	// Get raw caption prefix from locale
	rawCaptionPrefix := locales.GetMessage(localizer, "MsgReviewCaptionPrefix", nil, nil)

	// Assemble raw "Caption" line
	rawCaptionLine := rawCaptionPrefix + " " + rawCaptionContent
	// Escape the entire localized "Caption" line
	escapedCaptionLine := utils.EscapeMarkdownV2(rawCaptionLine)

	// Combine all parts with actual newlines.
	return fmt.Sprintf("%s\n%s\n%s", escapedIndexText, escapedFromText, escapedCaptionLine)
}
