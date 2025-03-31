package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
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
	debug       bool
}

// New creates a new bot instance
func New(token string, channelID int64, debug bool) (*Bot, error) {
	handler := handlers.NewMessageHandler(channelID)

	var bot *telego.Bot
	var err error

	if debug {
		bot, err = telego.NewBot(token, telego.WithDefaultDebugLogger())
	} else {
		bot, err = telego.NewBot(token, telego.WithDefaultLogger(false, false))
	}

	if err != nil {
		return nil, err
	}

	return &Bot{
		bot:     bot,
		handler: handler,
		debug:   debug,
	}, nil
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

// sendMediaGroupWithRetry sends media group to channel with retry on rate limit
func (b *Bot) sendMediaGroupWithRetry(inputMedia []telego.InputMedia, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		newCtx := context.Background()
		_, err := b.bot.SendMediaGroup(newCtx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(b.handler.GetChannelID()),
			Media:  inputMedia,
		})

		if err == nil {
			return nil
		}

		lastErr = err
		errStr := err.Error()

		// Check if error is "Too Many Requests"
		if strings.Contains(errStr, "Too Many Requests") {
			// Extract retry after value
			var retryAfter int
			_, err := fmt.Sscanf(errStr, "telego: sendMediaGroup: api: 429 \"Too Many Requests: retry after %d\"", &retryAfter)
			if err == nil {
				log.Printf("Rate limit hit, waiting %d seconds before retry %d/%d", retryAfter, i+1, maxRetries)
				time.Sleep(time.Duration(retryAfter) * time.Second)
				continue
			}
		}

		// If it's not a rate limit error, return immediately
		return err
	}

	return fmt.Errorf("max retries exceeded: %v", lastErr)
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
		if err := b.sendMediaGroupWithRetry(inputMedia, 3); err != nil {
			log.Printf("Error sending media group after retries: %v", err)
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

// handleCommand processes a single command message
func (b *Bot) handleCommand(ctx *th.Context, message telego.Message) error {
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
		return nil
	}
}

// handleMessage processes a single message
func (b *Bot) handleMessage(ctx *th.Context, message telego.Message) error {
	if b.debug {
		log.Printf("Received message: %+v", message)
	}

	if message.MediaGroupID != "" {
		b.handleMediaGroup(message)
		return nil
	}

	if strings.HasPrefix(message.Text, "/") {
		return b.handleCommand(ctx, message)
	}

	if message.Photo != nil {
		return b.handler.HandlePhoto(ctx, message)
	}

	if message.Text != "" {
		return b.handler.HandleText(ctx, message)
	}

	return nil
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) {
	if b.debug {
		log.Println("Starting bot in debug mode")
	}

	updates, err := b.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	bh, err := th.NewBotHandler(b.bot, updates)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if b.debug {
			log.Println("Stopping updates")
		}
		_ = bh.Stop()
	}()

	bh.HandleMessage(b.handleMessage)

	log.Println("Bot started")
	if err := bh.Start(); err != nil {
		log.Fatal(err)
	}
}
