package bot

import (
	"context"
	"log"

	"vrcmemes-bot/handlers"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Bot represents the Telegram bot
type Bot struct {
	bot     *bot.Bot
	handler *handlers.MessageHandler
}

// New creates a new bot instance
func New(token string, channelID int64) (*Bot, error) {
	handler := handlers.NewMessageHandler(channelID)

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message == nil {
				return
			}

			// First check for media group
			if update.Message.MediaGroupID != "" {
				handler.HandleMediaGroup(ctx, b, update)
				return
			}

			// Then check for commands
			switch update.Message.Text {
			case "/start":
				handler.HandleStart(ctx, b, update)
			case "/help":
				handler.HandleHelp(ctx, b, update)
			case "/status":
				handler.HandleStatus(ctx, b, update)
			case "/version":
				handler.HandleVersion(ctx, b, update)
			case "/caption":
				handler.HandleCaption(ctx, b, update)
			case "/showcaption":
				handler.HandleShowCaption(ctx, b, update)
			case "/clearcaption":
				handler.HandleClearCaption(ctx, b, update)
			default:
				if update.Message.Photo != nil {
					handler.HandlePhoto(ctx, b, update)
				} else if update.Message.Text != "" {
					handler.HandleText(ctx, b, update)
				}
			}
		}),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, err
	}

	return &Bot{
		bot:     b,
		handler: handler,
	}, nil
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) {
	log.Println("Bot started")
	b.bot.Start(ctx)
}
