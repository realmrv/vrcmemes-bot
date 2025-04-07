package telegoapi

import (
	"context"

	"github.com/mymmrac/telego"
)

// BotAPI defines the interface for bot operations used by various packages.
// This allows using both the real telego.Bot and mocks.
type BotAPI interface {
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
	GetMe(ctx context.Context) (*telego.User, error) // Used by some constructors/checks
	CopyMessage(ctx context.Context, params *telego.CopyMessageParams) (*telego.MessageID, error)
	SetMyCommands(ctx context.Context, params *telego.SetMyCommandsParams) error
	AnswerCallbackQuery(ctx context.Context, params *telego.AnswerCallbackQueryParams) error
	SendMediaGroup(ctx context.Context, params *telego.SendMediaGroupParams) ([]telego.Message, error) // Added based on usage in bot/bot.go

	// Methods required by suggestions package
	GetChatMember(ctx context.Context, params *telego.GetChatMemberParams) (telego.ChatMember, error)
	SendPhoto(ctx context.Context, params *telego.SendPhotoParams) (*telego.Message, error)
	DeleteMessage(ctx context.Context, params *telego.DeleteMessageParams) error
	// Add EditMessageMedia, EditMessageReplyMarkup if needed by review UI
}
