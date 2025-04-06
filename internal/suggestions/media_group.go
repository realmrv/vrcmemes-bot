package suggestions

import (
	"context"
	"log"
	"sort"
	"time"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// processSuggestionMediaGroup is called by the timer to finalize a media group suggestion.
func (m *Manager) processSuggestionMediaGroup(ctx context.Context, userID, chatID int64, mediaGroupID string) {
	defer m.deleteSuggestionGroup(mediaGroupID)

	group := m.getSuggestionGroup(mediaGroupID)
	if len(group) == 0 {
		log.Printf("[processSuggestionMediaGroup] Group %s for user %d is empty after timer. Ignoring.", mediaGroupID, userID)
		return
	}

	var fileIDs []string
	var caption string
	var firstMessageID int = -1

	for _, msg := range group {
		if len(msg.Photo) > 0 {
			fileIDs = append(fileIDs, msg.Photo[len(msg.Photo)-1].FileID)
			if msg.Caption != "" {
				caption = msg.Caption
			}
			if firstMessageID == -1 {
				firstMessageID = msg.MessageID
			}
		}
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	localizer := locales.NewLocalizer(lang)

	if len(fileIDs) == 0 {
		log.Printf("[processSuggestionMediaGroup] Group %s for user %d contained no valid photos. Sending error.", mediaGroupID, userID)
		errorMsg := locales.GetMessage(localizer, "MsgSuggestionRequiresPhoto", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if err != nil {
			log.Printf("[processSuggestionMediaGroup] Error sending no-photo error to user %d: %v", userID, err)
		}
		m.SetUserState(userID, StateIdle)
		return
	}

	if len(fileIDs) > 10 {
		log.Printf("[processSuggestionMediaGroup] Group %s for user %d has too many photos (%d). Sending error.", mediaGroupID, userID, len(fileIDs))
		errorMsg := locales.GetMessage(localizer, "MsgSuggestionTooManyPhotosError", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if err != nil {
			log.Printf("[processSuggestionMediaGroup] Error sending too-many-photos error to user %d: %v", userID, err)
		}
		m.SetUserState(userID, StateIdle)
		return
	}

	var username, firstName string
	if len(group) > 0 && group[0].From != nil {
		username = group[0].From.Username
		firstName = group[0].From.FirstName
	}

	suggestionForDB := &models.Suggestion{
		SuggesterID: userID,
		Username:    username,
		FirstName:   firstName,
		MessageID:   firstMessageID,
		ChatID:      chatID,
		FileIDs:     fileIDs,
		Caption:     caption,
		Status:      string(StatusPending),
		SubmittedAt: time.Now(),
	}

	err := m.AddSuggestion(ctx, suggestionForDB)
	if err != nil {
		log.Printf("[processSuggestionMediaGroup] Error saving media group suggestion for user %d: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("[processSuggestionMediaGroup] Error sending internal error message to user %d: %v", userID, sendErr)
		}
		m.SetUserState(userID, StateIdle)
		return
	}

	m.SetUserState(userID, StateIdle)
	confirmationMsg := locales.GetMessage(localizer, "MsgSuggestionReceivedConfirmation", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
	if err != nil {
		log.Printf("[processSuggestionMediaGroup] Error sending confirmation for group %s to user %d: %v", mediaGroupID, userID, err)
	}

	log.Printf("[processSuggestionMediaGroup] Successfully processed media group %s for user %d", mediaGroupID, userID)
}

// addMessageToSuggestionGroup adds a message to the temporary storage for its media group.
func (m *Manager) addMessageToSuggestionGroup(message telego.Message) {
	if message.MediaGroupID == "" {
		return
	}
	m.muSuggestionMediaGroups.Lock()
	defer m.muSuggestionMediaGroups.Unlock()

	group := m.suggestionMediaGroups[message.MediaGroupID]
	for _, msg := range group {
		if msg.MessageID == message.MessageID {
			return // Avoid duplicates
		}
	}
	group = append(group, message)
	sort.Slice(group, func(i, j int) bool {
		return group[i].MessageID < group[j].MessageID
	})

	m.suggestionMediaGroups[message.MediaGroupID] = group
	log.Printf("[Suggestions] Added message %d to suggestion media group %s (current size: %d)", message.MessageID, message.MediaGroupID, len(group))
}

// getSuggestionGroup retrieves a copy of the messages for a media group.
func (m *Manager) getSuggestionGroup(mediaGroupID string) []telego.Message {
	m.muSuggestionMediaGroups.Lock()
	defer m.muSuggestionMediaGroups.Unlock()
	group := m.suggestionMediaGroups[mediaGroupID]
	groupCopy := make([]telego.Message, len(group))
	copy(groupCopy, group)
	return groupCopy
}

// deleteSuggestionGroup removes a media group from temporary storage.
func (m *Manager) deleteSuggestionGroup(mediaGroupID string) {
	m.muSuggestionMediaGroups.Lock()
	defer m.muSuggestionMediaGroups.Unlock()
	delete(m.suggestionMediaGroups, mediaGroupID)
	log.Printf("[Suggestions] Deleted suggestion media group %s from temp storage", mediaGroupID)
}
