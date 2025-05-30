package handlers

import (
	"context"
	"errors"
	"log"
	"vrcmemes-bot/internal/locales"
	telegoapi "vrcmemes-bot/pkg/telegoapi" // Import for BotAPI
	"vrcmemes-bot/pkg/utils"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
)

// HandleCaption handles the /caption command.
// It allows admins to set or update the active caption.
// If no text is provided after the command, it asks the user for input
// and waits for the next text message.
func (h *MessageHandler) HandleCaption(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
	chatID := message.Chat.ID
	userID := message.From.ID
	localizer := h.getLocalizer(message.From) // Use helper

	// --- Admin Check ---
	isAdmin, err := h.adminChecker.IsAdmin(ctx, userID) // Use checker method
	if err != nil {
		// Handle the error from IsAdmin appropriately
		if errors.Is(err, errors.New("failed to get chat member info")) {
			log.Printf("[Cmd:caption User:%d] Error checking admin status: %v", userID, err)
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			// Consider returning a more specific error?
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
		}
		// Other unexpected errors, assume non-admin
		log.Printf("[Cmd:caption User:%d] Unexpected error during admin check: %v. Assuming non-admin.", userID, err)
		isAdmin = false
	}

	if !isAdmin {
		log.Printf("[Cmd:caption User:%d] Non-admin attempted to use /caption.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}
	// --- End Admin Check ---

	// Mark that we are waiting for the next message as caption
	h.waitingForCaption.Store(chatID, true)

	// Record activity
	h.RecordUserActivity(ctx, message.From, ActionCommandCaption, isAdmin, map[string]interface{}{
		"chat_id": chatID,
	})

	// Send prompt message using localized text
	promptMsg := locales.GetMessage(localizer, "MsgCaptionPrompt", nil, nil)
	// Use direct SendMessage with escaping, assuming prompt might contain special chars
	params := &telego.SendMessageParams{
		ChatID:    telegoutil.ID(chatID),
		Text:      utils.EscapeMarkdownV2(promptMsg),
		ParseMode: telego.ModeMarkdownV2,
	}
	_, err = bot.SendMessage(ctx, params)
	if err != nil {
		log.Printf("Error sending caption prompt message to chat %d: %v", chatID, err)
		return nil // Logged error
	}
	return nil
}

// HandleShowCaption handles the /showcaption command.
// It allows admins to see the currently active caption.
func (h *MessageHandler) HandleShowCaption(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
	userID := message.From.ID
	// --- Admin Check ---
	isAdmin, checkErr := h.adminChecker.IsAdmin(ctx, userID) // Use checker method
	if checkErr != nil {
		// Handle the error from IsAdmin
		if errors.Is(checkErr, errors.New("failed to get chat member info")) {
			log.Printf("[Cmd:showcaption User:%d] Error checking admin status: %v. Assuming non-admin.", userID, checkErr)
		} else {
			log.Printf("[Cmd:showcaption User:%d] Unexpected error during admin check: %v. Assuming non-admin.", userID, checkErr)
		}
		isAdmin = false // Assume non-admin on any error
	}

	if !isAdmin {
		log.Printf("User %d (not admin) attempted to use /showcaption.", userID)
		lang := locales.GetDefaultLanguageTag().String()
		localizer := locales.NewLocalizer(lang)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(msg))
	}
	// --- End Admin Check ---

	chatID := message.Chat.ID
	captionText, exists := h.GetActiveCaption(chatID)

	lang := locales.GetDefaultLanguageTag().String()
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode // Keep default for now
	}
	localizer := locales.NewLocalizer(lang)

	var msg string
	if exists {
		msg = locales.GetMessage(localizer, "MsgCaptionShowCurrent", map[string]interface{}{"Caption": captionText}, nil)
	} else {
		msg = locales.GetMessage(localizer, "MsgCaptionShowEmpty", nil, nil)
	}

	// Log action
	err := h.actionLogger.LogUserAction(userID, "show_caption", map[string]interface{}{
		"chat_id":        chatID,
		"caption_exists": exists,
	})
	if err != nil {
		log.Printf("Failed to log show_caption action for user %d: %v", userID, err)
	}

	// Escape text for MarkdownV2 and send the message
	escapedMsg := utils.EscapeMarkdownV2(msg)
	params := &telego.SendMessageParams{
		ChatID:    telegoutil.ID(chatID),
		Text:      escapedMsg,
		ParseMode: telego.ModeMarkdownV2,
	}
	_, sendErr := bot.SendMessage(ctx, params)
	if sendErr != nil {
		log.Printf("Error sending show caption message to chat %d: %v", chatID, sendErr)
		// Log error, but maybe still return the logging error?
		// For consistency, returning nil after logging.
		return nil
	}

	// Return the original logging error if it occurred, otherwise nil
	return err // Return the logging error
}

// HandleClearCaption handles the /clearcaption command.
// It removes the currently set caption for the chat and confirms the action to the user.
func (h *MessageHandler) HandleClearCaption(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
	chatID := message.Chat.ID
	userID := message.From.ID
	localizer := h.getLocalizer(message.From) // Use helper

	// --- Admin Check ---
	isAdmin, err := h.adminChecker.IsAdmin(ctx, userID) // Use checker method
	if err != nil {
		// Handle the error from IsAdmin appropriately
		if errors.Is(err, errors.New("failed to get chat member info")) {
			log.Printf("[Cmd:clearcaption User:%d] Error checking admin status: %v", userID, err)
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			// Consider returning a more specific error?
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
		}
		// Other unexpected errors, assume non-admin
		log.Printf("[Cmd:clearcaption User:%d] Unexpected error during admin check: %v. Assuming non-admin.", userID, err)
		isAdmin = false
	}

	if !isAdmin {
		log.Printf("[Cmd:clearcaption User:%d] Non-admin attempted to use /clearcaption.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}
	// --- End Admin Check ---

	_, exists := h.activeCaptions.LoadAndDelete(chatID)
	h.waitingForCaption.Delete(chatID) // Also ensure waiting state is cleared

	var responseMsg string
	if exists {
		responseMsg = locales.GetMessage(localizer, "MsgCaptionCleared", nil, nil)
	} else {
		responseMsg = locales.GetMessage(localizer, "MsgCaptionNoneToClear", nil, nil)
	}

	// Record activity
	h.RecordUserActivity(ctx, message.From, ActionCommandClearCaption, isAdmin, map[string]interface{}{
		"chat_id":     chatID,
		"was_cleared": exists,
	})

	// Escape text for MarkdownV2 and send confirmation message
	escapedResponseMsg := utils.EscapeMarkdownV2(responseMsg)
	params := &telego.SendMessageParams{
		ChatID:    telegoutil.ID(chatID),
		Text:      escapedResponseMsg,
		ParseMode: telego.ModeMarkdownV2,
	}
	_, err = bot.SendMessage(ctx, params)
	if err != nil {
		log.Printf("Error sending clear caption confirmation to chat %d: %v", chatID, err)
		return nil // Logged error
	}
	return nil
}

// --- GetActiveCaption Removed (defined in helpers.go) ---

// setActiveCaption is unused because caption setting now happens directly via h.activeCaptions.Store

// clearActiveCaption is unused because caption clearing now happens directly via h.activeCaptions.Delete

// StoreMediaGroupCaption stores a caption associated with a specific media group ID.
// This is likely used to apply captions to media groups received shortly after text messages.
// It uses a sync.Map for thread-safe storage.
func (h *MessageHandler) StoreMediaGroupCaption(groupID, caption string) {
	if groupID == "" {
		log.Println("WARN: StoreMediaGroupCaption called with empty groupID")
		return
	}
	if caption == "" {
		log.Println("WARN: StoreMediaGroupCaption called with empty caption")
		// Optionally delete if an empty caption means removal: h.mediaGroupCaptions.Delete(groupID)
		return
	}
	h.mediaGroupCaptions.Store(groupID, caption)
	log.Printf("Caption stored for media group %s", groupID)
}
