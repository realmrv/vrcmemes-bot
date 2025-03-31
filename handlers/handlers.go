package handlers

import (
	"fmt"
	"os"
	"sync"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	version = "1.0.0"
)

// Message types
const (
	msgStart               = "Привет! Я бот для публикации мемов в канал. Отправь мне фото или текст, и я опубликую его в канале."
	msgCaptionPrompt       = "Пожалуйста, введите текст подписи для следующих фотографий. Это заменит любую существующую подпись."
	msgCaptionSaved        = "Подпись сохранена! Все фотографии, которые вы отправите, будут использовать эту подпись. Используйте /caption снова, чтобы изменить её."
	msgCaptionOverwrite    = "Предыдущая подпись была заменена на новую."
	msgPostSuccess         = "Пост успешно опубликован в канале!"
	msgPhotoSuccess        = "Фото успешно опубликовано в канале!"
	msgPhotoWithCaption    = "Фото с подписью успешно опубликовано в канале!"
	msgHelpFooter          = "\nЧтобы создать пост, просто отправьте любое текстовое сообщение.\nЧтобы добавить подпись к фото, используйте команду /caption, а затем отправьте фото."
	msgNoCaptionSet        = "Нет активной подписи. Используйте /caption, чтобы установить её."
	msgCurrentCaption      = "Текущая активная подпись:\n%s"
	msgCaptionCleared      = "Активная подпись очищена."
	msgErrorSendingMessage = "Ошибка отправки сообщения: %s"
	msgErrorCopyingMessage = "Ошибка копирования сообщения: %s"
)

// Command represents a bot command
type Command struct {
	Command     string
	Description string
	Handler     func(*th.Context, telego.Message) error
}

// MessageHandler handles incoming messages
type MessageHandler struct {
	channelID int64
	// Map to store users waiting for captions
	waitingForCaption sync.Map
	// Map to store active captions for users
	activeCaptions sync.Map
	// Map to store captions for media groups
	mediaGroupCaptions sync.Map
	// Map to store processed message count for media groups
	mediaGroupProcessed sync.Map
	// Available commands
	commands []Command
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(channelID int64) *MessageHandler {
	h := &MessageHandler{
		channelID: channelID,
	}

	// Initialize commands
	h.commands = []Command{
		{Command: "start", Description: "Начать работу с ботом", Handler: h.HandleStart},
		{Command: "help", Description: "Показать справку", Handler: h.HandleHelp},
		{Command: "status", Description: "Проверить статус бота", Handler: h.HandleStatus},
		{Command: "version", Description: "Показать версию бота", Handler: h.HandleVersion},
		{Command: "caption", Description: "Установить подпись для следующих фото", Handler: h.HandleCaption},
		{Command: "showcaption", Description: "Показать текущую подпись", Handler: h.HandleShowCaption},
		{Command: "clearcaption", Description: "Очистить текущую подпись", Handler: h.HandleClearCaption},
	}

	return h
}

// sendError sends an error message to the user
func (h *MessageHandler) sendError(ctx *th.Context, chatID int64, err error) error {
	_, err = ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		fmt.Sprintf(msgErrorSendingMessage, err.Error()),
	))
	return err
}

// sendSuccess sends a success message to the user
func (h *MessageHandler) sendSuccess(ctx *th.Context, chatID int64, message string) error {
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		message,
	))
	return err
}

// setupCommands registers bot commands
func (h *MessageHandler) setupCommands(ctx *th.Context) error {
	commands := make([]telego.BotCommand, len(h.commands))
	for i, cmd := range h.commands {
		commands[i] = telego.BotCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		}
	}

	err := ctx.Bot().SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: commands,
	})
	return err
}

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx *th.Context, message telego.Message) error {
	if err := h.setupCommands(ctx); err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, msgStart)
}

// HandleHelp handles the /help command
func (h *MessageHandler) HandleHelp(ctx *th.Context, message telego.Message) error {
	var helpText string
	for _, cmd := range h.commands {
		helpText += fmt.Sprintf("/%s - %s\n", cmd.Command, cmd.Description)
	}
	helpText += msgHelpFooter

	return h.sendSuccess(ctx, message.Chat.ID, helpText)
}

// HandleCaption handles the /caption command
func (h *MessageHandler) HandleCaption(ctx *th.Context, message telego.Message) error {
	h.waitingForCaption.Store(message.Chat.ID, true)
	return h.sendSuccess(ctx, message.Chat.ID, msgCaptionPrompt)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx *th.Context, message telego.Message) error {
	caption, _ := h.getActiveCaption(message.Chat.ID)
	statusText := fmt.Sprintf("Бот работает\nID канала: %d\nПодпись: %s", h.channelID, caption)
	return h.sendSuccess(ctx, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx *th.Context, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}
	return h.sendSuccess(ctx, message.Chat.ID, "Версия бота: "+version)
}

// getActiveCaption returns the active caption for a user
func (h *MessageHandler) getActiveCaption(chatID int64) (string, bool) {
	if caption, exists := h.activeCaptions.Load(chatID); exists {
		return caption.(string), true
	}
	return "", false
}

// setActiveCaption sets the active caption for a user
func (h *MessageHandler) setActiveCaption(chatID int64, caption string) {
	h.activeCaptions.Store(chatID, caption)
}

// clearActiveCaption removes the active caption for a user
func (h *MessageHandler) clearActiveCaption(chatID int64) {
	h.activeCaptions.Delete(chatID)
}

// HandleText handles text messages
func (h *MessageHandler) HandleText(ctx *th.Context, message telego.Message) error {
	if message.Text == "" || message.Text == "/start" {
		return nil
	}

	if _, waiting := h.waitingForCaption.Load(message.Chat.ID); waiting {
		// Check if there was a previous caption
		_, hadPreviousCaption := h.getActiveCaption(message.Chat.ID)

		// Store the new caption for future photos
		h.setActiveCaption(message.Chat.ID, message.Text)
		h.waitingForCaption.Delete(message.Chat.ID)

		// Send appropriate message
		if hadPreviousCaption {
			return h.sendSuccess(ctx, message.Chat.ID, msgCaptionOverwrite)
		}
		return h.sendSuccess(ctx, message.Chat.ID, msgCaptionSaved)
	}

	_, err := ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(h.channelID),
		message.Text,
	).WithParseMode(telego.ModeHTML),
	)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, msgPostSuccess)
}

// HandlePhoto handles photo messages
func (h *MessageHandler) HandlePhoto(ctx *th.Context, message telego.Message) error {
	if message.Photo == nil {
		return nil
	}

	// Get active caption if exists
	var caption string
	if caption, _ = h.getActiveCaption(message.Chat.ID); caption != "" {
		// Don't clear caption after use
	}

	// Copy message to channel
	_, err := ctx.Bot().CopyMessage(ctx, &telego.CopyMessageParams{
		ChatID:     tu.ID(h.channelID),
		FromChatID: tu.ID(message.Chat.ID),
		MessageID:  message.MessageID,
		Caption:    caption,
	})
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	successMsg := msgPhotoSuccess
	if caption != "" {
		successMsg = msgPhotoWithCaption
	}
	return h.sendSuccess(ctx, message.Chat.ID, successMsg)
}

// HandleMediaGroup handles media group messages
func (h *MessageHandler) HandleMediaGroup(ctx *th.Context, message telego.Message) error {
	if message.MediaGroupID == "" {
		return nil
	}

	// Get active caption if exists
	var caption string
	if caption, _ = h.getActiveCaption(message.Chat.ID); caption != "" {
		// Store caption for media group
		h.mediaGroupCaptions.Store(message.MediaGroupID, caption)
	}

	// Create input media array
	var inputMedia []telego.InputMedia
	if message.Photo != nil {
		photo := message.Photo[len(message.Photo)-1]
		mediaPhoto := &telego.InputMediaPhoto{
			Type:  "photo",
			Media: telego.InputFile{FileID: photo.FileID},
		}
		// Add caption only to the first photo
		if caption != "" {
			mediaPhoto.Caption = caption
		}
		inputMedia = append(inputMedia, mediaPhoto)
	}

	// Send media group to channel
	_, err := ctx.Bot().SendMediaGroup(ctx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(h.channelID),
		Media:  inputMedia,
	})
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	successMsg := "Медиа группа опубликована в канале"
	if caption != "" {
		successMsg = "Медиа группа с подписью опубликована в канале"
	}
	return h.sendSuccess(ctx, message.Chat.ID, successMsg)
}

// HandleShowCaption handles the /showcaption command
func (h *MessageHandler) HandleShowCaption(ctx *th.Context, message telego.Message) error {
	caption, exists := h.getActiveCaption(message.Chat.ID)
	if !exists {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoCaptionSet)
	}

	return h.sendSuccess(ctx, message.Chat.ID, fmt.Sprintf(msgCurrentCaption, caption))
}

// HandleClearCaption handles the /clearcaption command
func (h *MessageHandler) HandleClearCaption(ctx *th.Context, message telego.Message) error {
	h.clearActiveCaption(message.Chat.ID)
	return h.sendSuccess(ctx, message.Chat.ID, msgCaptionCleared)
}

// GetActiveCaption returns the active caption for a user
func (h *MessageHandler) GetActiveCaption(chatID int64) (string, bool) {
	if caption, exists := h.activeCaptions.Load(chatID); exists {
		return caption.(string), true
	}
	return "", false
}

// GetChannelID returns the channel ID
func (h *MessageHandler) GetChannelID() int64 {
	return h.channelID
}

// StoreMediaGroupCaption stores caption for a media group
func (h *MessageHandler) StoreMediaGroupCaption(mediaGroupID string, caption string) {
	h.mediaGroupCaptions.Store(mediaGroupID, caption)
}
