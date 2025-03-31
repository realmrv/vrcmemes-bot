package bot

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"vrcmemes-bot/handlers"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Bot represents the Telegram bot
type Bot struct {
	bot         *telego.Bot
	handler     *handlers.MessageHandler
	mediaGroups sync.Map
}

// New creates a new bot instance
func New(token string, channelID int64) (*Bot, error) {
	handler := handlers.NewMessageHandler(channelID)
	bot, err := telego.NewBot(token, telego.WithDefaultDebugLogger())
	if err != nil {
		return nil, err
	}
	return &Bot{bot: bot, handler: handler}, nil
}

// storeMessageInGroup stores a message in the media group
func (b *Bot) storeMessageInGroup(message telego.Message) {
	if messages, exists := b.mediaGroups.Load(message.MediaGroupID); exists {
		msgs := messages.([]telego.Message)
		msgs = append(msgs, message)
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].MessageID < msgs[j].MessageID
		})
		b.mediaGroups.Store(message.MediaGroupID, msgs)
	} else {
		b.mediaGroups.Store(message.MediaGroupID, []telego.Message{message})
	}
}

// createInputMedia creates input media array from messages
func (b *Bot) createInputMedia(msgs []telego.Message, caption string) []telego.InputMedia {
	var inputMedia []telego.InputMedia
	for i, msg := range msgs {
		if msg.Photo != nil {
			photo := msg.Photo[len(msg.Photo)-1]
			mediaPhoto := &telego.InputMediaPhoto{
				Type:  "photo",
				Media: telego.InputFile{FileID: photo.FileID},
			}
			if i == 0 && caption != "" {
				mediaPhoto.Caption = caption
			}
			inputMedia = append(inputMedia, mediaPhoto)
		}
	}
	return inputMedia
}

// sendMediaGroup sends media group to channel
func (b *Bot) sendMediaGroup(inputMedia []telego.InputMedia) error {
	newCtx := context.Background()
	_, err := b.bot.SendMediaGroup(newCtx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(b.handler.GetChannelID()),
		Media:  inputMedia,
	})
	return err
}

// processMediaGroup processes a complete media group
func (b *Bot) processMediaGroup(message telego.Message, msgs []telego.Message) {
	log.Printf("Processing media group: ID=%s, Messages count=%d", message.MediaGroupID, len(msgs))

	var caption string
	if caption, _ = b.handler.GetActiveCaption(message.Chat.ID); caption != "" {
		b.handler.StoreMediaGroupCaption(message.MediaGroupID, caption)
	}

	inputMedia := b.createInputMedia(msgs, caption)
	log.Printf("Created input media: count=%d", len(inputMedia))

	if len(inputMedia) > 0 {
		if err := b.sendMediaGroup(inputMedia); err != nil {
			log.Printf("Error sending media group: %v", err)
		} else {
			log.Printf("Successfully sent media group")
		}
	} else {
		log.Printf("No valid media found in group")
	}

	b.mediaGroups.Delete(message.MediaGroupID)
	log.Printf("Removed media group from storage: ID=%s", message.MediaGroupID)
}

// handleMediaGroup processes a media group message
func (b *Bot) handleMediaGroup(message telego.Message) {
	log.Printf("Received media group message: ID=%d, MediaGroupID=%s", message.MessageID, message.MediaGroupID)
	b.storeMessageInGroup(message)

	go func() {
		time.Sleep(3 * time.Second)

		if messages, exists := b.mediaGroups.Load(message.MediaGroupID); exists {
			msgs := messages.([]telego.Message)
			if message.MessageID != msgs[0].MessageID {
				log.Printf("Skipping non-first message in group: ID=%d", message.MessageID)
				return
			}
			b.processMediaGroup(message, msgs)
		} else {
			log.Printf("Media group not found in storage: ID=%s", message.MediaGroupID)
		}
	}()
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) {
	updates, err := b.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	bh, err := th.NewBotHandler(b.bot, updates)
	if err != nil {
		log.Fatal(err)
	}

	defer func() { _ = bh.Stop() }()

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		if message.MediaGroupID != "" {
			b.handleMediaGroup(message)
			return nil
		}

		switch message.Text {
		case "/start":
			return b.handler.HandleStart(ctx, message)
		case "/help":
			return b.handler.HandleHelp(ctx, message)
		case "/status":
			return b.handler.HandleStatus(ctx, message)
		case "/version":
			return b.handler.HandleVersion(ctx, message)
		case "/caption":
			return b.handler.HandleCaption(ctx, message)
		case "/showcaption":
			return b.handler.HandleShowCaption(ctx, message)
		case "/clearcaption":
			return b.handler.HandleClearCaption(ctx, message)
		default:
			if message.Photo != nil {
				return b.handler.HandlePhoto(ctx, message)
			} else if message.Text != "" {
				return b.handler.HandleText(ctx, message)
			}
			return nil
		}
	})

	log.Println("Bot started")
	if err := bh.Start(); err != nil {
		log.Fatal(err)
	}
}
