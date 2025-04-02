package suggestions

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"vrcmemes-bot/database"
	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"
)

// Manager handles the suggestion logic and storage.
type Manager struct {
	userStates   map[int64]UserState
	muUserStates sync.RWMutex

	suggestionMediaGroups   map[string][]telego.Message
	muSuggestionMediaGroups sync.Mutex

	bot             *telego.Bot
	targetChannelID int64
	repo            database.SuggestionRepository

	adminCache      []telego.ChatMember
	adminCacheTime  time.Time
	adminCacheMutex sync.RWMutex
	adminCacheTTL   time.Duration

	reviewSessions      map[int64]*ReviewSession
	reviewSessionsMutex sync.RWMutex
}

// NewManager creates a new suggestion manager.
func NewManager(bot *telego.Bot, targetChannelID int64, repo database.SuggestionRepository) *Manager {
	return &Manager{
		userStates:            make(map[int64]UserState),
		suggestionMediaGroups: make(map[string][]telego.Message),
		bot:                   bot,
		targetChannelID:       targetChannelID,
		repo:                  repo,
		adminCache:            make([]telego.ChatMember, 0),
		adminCacheTTL:         5 * time.Minute,
		reviewSessions:        make(map[int64]*ReviewSession),
	}
}

// HandleSuggestCommand handles the /suggest command.
func (m *Manager) HandleSuggestCommand(ctx context.Context, update telego.Update) error {
	if update.Message == nil || update.Message.From == nil {
		log.Println("HandleSuggestCommand: Received update without Message or From field.")
		return fmt.Errorf("invalid update received for suggest command")
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	// Default to Russian
	lang := locales.DefaultLanguage
	localizer := locales.NewLocalizer(lang)

	if m.GetUserState(userID) == StateAwaitingSuggestion {
		msg := locales.GetMessage(localizer, "MsgSuggestAlreadyWaitingForContent", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return err
	}

	isSubscribed, err := m.CheckSubscription(ctx, userID)
	if err != nil {
		log.Printf("Error checking subscription for user %d: %v", userID, err)
		msg := locales.GetMessage(localizer, "MsgSuggestErrorCheckingSubscription", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		if sendErr != nil {
			log.Printf("Error sending subscription check error message to user %d: %v", userID, sendErr)
		}
		return err
	}

	if !isSubscribed {
		msg := locales.GetMessage(localizer, "MsgSuggestRequiresSubscription", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return err
	}

	m.SetUserState(userID, StateAwaitingSuggestion)
	promptMsg := locales.GetMessage(localizer, "MsgSuggestSendContentPrompt", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), promptMsg))
	if err != nil {
		m.SetUserState(userID, StateIdle) // Rollback state if sending prompt fails
		log.Printf("Error sending suggest prompt to user %d: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("Error sending internal error message after prompt failure to user %d: %v", userID, sendErr)
		}
		return err
	}

	return nil
}

// HandleMessage processes incoming messages potentially containing suggestion content.
func (m *Manager) HandleMessage(ctx context.Context, update telego.Update) (processed bool, err error) {
	if update.Message == nil || update.Message.From == nil || m.GetUserState(update.Message.From.ID) != StateAwaitingSuggestion {
		return false, nil
	}

	message := update.Message
	userID := message.From.ID
	chatID := message.Chat.ID

	// Default to Russian
	lang := locales.DefaultLanguage
	if message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Handle Media Group
	if message.MediaGroupID != "" {
		if len(message.Photo) == 0 {
			log.Printf("[HandleMessage] Received non-photo message part of media group %s from user %d. Ignoring.", message.MediaGroupID, userID)
			return true, nil
		}

		m.addMessageToSuggestionGroup(*message)

		group := m.getSuggestionGroup(message.MediaGroupID)
		if len(group) == 1 {
			log.Printf("[HandleMessage] Started media group timer for group %s, user %d", message.MediaGroupID, userID)
			msg := locales.GetMessage(localizer, "MsgSuggestionMediaGroupPartReceived", nil, nil)
			_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
			if sendErr != nil {
				log.Printf("[HandleMessage] Error sending media group part received confirmation to user %d: %v", userID, sendErr)
			}

			bgCtx := context.Background()
			time.AfterFunc(3*time.Second, func() {
				m.processSuggestionMediaGroup(bgCtx, userID, chatID, message.MediaGroupID)
			})
		}
		return true, nil
	}

	// Handle Single Photo
	if message.Photo != nil && len(message.Photo) > 0 {
		fileIDs := []string{message.Photo[len(message.Photo)-1].FileID}
		caption := message.Caption

		suggestionForDB := &models.Suggestion{
			SuggesterID: userID,
			Username:    message.From.Username,
			FirstName:   message.From.FirstName,
			MessageID:   message.MessageID,
			ChatID:      chatID,
			FileIDs:     fileIDs,
			Caption:     caption,
			Status:      string(StatusPending),
		}
		err = m.AddSuggestion(ctx, suggestionForDB)
		if err != nil {
			log.Printf("[HandleMessage] Error saving single photo suggestion for user %d: %v", userID, err)
			errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
			_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
			if sendErr != nil {
				log.Printf("[HandleMessage] Error sending internal error message to user %d: %v", userID, sendErr)
			}
			m.SetUserState(userID, StateIdle)
			return true, err
		}

		m.SetUserState(userID, StateIdle)
		confirmationMsg := locales.GetMessage(localizer, "MsgSuggestionReceivedConfirmation", nil, nil)
		_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
		if err != nil {
			log.Printf("[HandleMessage] Error sending single photo confirmation to user %d: %v", userID, err)
		}
		return true, nil
	}

	// Wrong Message Type
	log.Printf("[HandleMessage] User %d sent non-photo/non-media-group message while awaiting suggestion.", userID)
	errorMsg := locales.GetMessage(localizer, "MsgSuggestionRequiresPhoto", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
	return true, err
}

// SetUserState sets the state for a given user.
func (m *Manager) SetUserState(userID int64, state UserState) {
	m.muUserStates.Lock()
	defer m.muUserStates.Unlock()
	if state == StateIdle {
		delete(m.userStates, userID)
	} else {
		m.userStates[userID] = state
	}
}

// GetUserState retrieves the state for a given user.
func (m *Manager) GetUserState(userID int64) UserState {
	m.muUserStates.RLock()
	defer m.muUserStates.RUnlock()
	return m.userStates[userID]
}

// CheckSubscription checks if a user is a member of the target channel.
func (m *Manager) CheckSubscription(ctx context.Context, userID int64) (bool, error) {
	member, err := m.bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: telego.ChatID{ID: m.targetChannelID},
		UserID: userID,
	})
	if err != nil {
		// Consider specific API errors if needed
		log.Printf("Error getting chat member %d from channel %d: %v", userID, m.targetChannelID, err)
		return false, fmt.Errorf("failed to get chat member info: %w", err)
	}

	allowedStatuses := []string{"creator", "administrator", "member", "restricted"}
	status := member.MemberStatus()
	for _, allowed := range allowedStatuses {
		if status == allowed {
			return true, nil
		}
	}
	return false, nil
}

// AddSuggestion saves a new suggestion to the database.
func (m *Manager) AddSuggestion(ctx context.Context, suggestion *models.Suggestion) error {
	suggestion.Status = string(StatusPending)
	err := m.repo.CreateSuggestion(ctx, suggestion)
	if err != nil {
		log.Printf("Error creating suggestion in DB for user %d: %v", suggestion.SuggesterID, err)
		return err
	}
	log.Printf("Created suggestion in DB with ID %s from user %d", suggestion.ID.Hex(), suggestion.SuggesterID)
	return nil
}

// GetPendingSuggestions retrieves pending suggestions from the database.
func (m *Manager) GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error) {
	return m.repo.GetPendingSuggestions(ctx, limit, offset)
}

// GetSuggestionByID retrieves a suggestion by its MongoDB ObjectID.
func (m *Manager) GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error) {
	return m.repo.GetSuggestionByID(ctx, id)
}

// UpdateSuggestionStatus updates the status of a suggestion in the database.
func (m *Manager) UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status models.SuggestionStatus, reviewerID int64) error {
	err := m.repo.UpdateSuggestionStatus(ctx, id, string(status), reviewerID)
	if err != nil {
		log.Printf("Error updating suggestion status in DB for ID %s: %v", id.Hex(), err)
		return err
	}
	log.Printf("Updated suggestion status in DB for ID %s to %s by reviewer %d", id.Hex(), status, reviewerID)
	return nil
}

// DeleteSuggestion removes a suggestion from the database.
func (m *Manager) DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error {
	err := m.repo.DeleteSuggestion(ctx, id)
	if err != nil {
		log.Printf("Error deleting suggestion from DB with ID %s: %v", id.Hex(), err)
		return err
	}
	log.Printf("Deleted suggestion from DB with ID %s", id.Hex())
	return nil
}
