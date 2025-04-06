package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	sentry "github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// --- Media Group Helpers ---

// createInputMedia converts a slice of telego.Message into telego.InputMedia.
// It applies the caption to the first media item.
func createInputMedia(msgs []telego.Message, caption string) []telego.InputMedia {
	inputMedia := make([]telego.InputMedia, 0, len(msgs))
	for i, msg := range msgs {
		if msg.Photo != nil && len(msg.Photo) > 0 {
			// Find the largest photo
			photo := msg.Photo[0]
			for _, p := range msg.Photo {
				if p.FileSize > photo.FileSize {
					photo = p
				}
			}
			mediaPhoto := &telego.InputMediaPhoto{
				Type:  telego.MediaTypePhoto,
				Media: telego.InputFile{FileID: photo.FileID},
			}
			if i == 0 && caption != "" {
				mediaPhoto.Caption = caption
				// TODO: Add ParseMode if needed (e.g., telego.ModeHTML)
			}
			inputMedia = append(inputMedia, mediaPhoto)
		} else if msg.Video != nil {
			mediaVideo := &telego.InputMediaVideo{
				Type:  telego.MediaTypeVideo,
				Media: telego.InputFile{FileID: msg.Video.FileID},
			}
			if i == 0 && caption != "" {
				mediaVideo.Caption = caption
				// TODO: Add ParseMode if needed
			}
			inputMedia = append(inputMedia, mediaVideo)
		} else {
			log.Printf("[createInputMedia] Unsupported message type in media group: MsgID=%d", msg.MessageID)
		}
	}
	return inputMedia
}

// sendMediaGroupWithRetry attempts to send a media group with retries.
// It requires the bot instance, channel ID, and debug flag.
func sendMediaGroupWithRetry(ctx context.Context, bot *telego.Bot, channelID int64, debug bool, groupID string, inputMedia []telego.InputMedia, maxRetries int) ([]telego.Message, error) {
	var lastErr error
	var retryCount int
	const defaultRetryWait = 2 * time.Second
	logPrefix := fmt.Sprintf("[SendRetry Group:%s]", groupID)

	for retryCount < maxRetries {
		sentMessages, err := bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(channelID),
			Media:  inputMedia,
		})

		if err == nil {
			if debug || retryCount > 0 {
				log.Printf("%s Successfully sent after %d attempt(s)", logPrefix, retryCount+1)
			}
			return sentMessages, nil
		}

		lastErr = err
		errStr := err.Error()

		if strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "429") {
			retryAfterSeconds, ok := parseRetryAfter(errStr) // Use helper
			waitDuration := defaultRetryWait
			if ok {
				log.Printf("%s Rate limit hit (attempt %d/%d), waiting %d seconds", logPrefix, retryCount+1, maxRetries, retryAfterSeconds)
				waitDuration = time.Duration(retryAfterSeconds) * time.Second
			} else {
				// Warning: Parsing the error string might be brittle.
				log.Printf("%s Rate limit hit (attempt %d/%d), couldn't parse retry time, waiting %v. Error: %s", logPrefix, retryCount+1, maxRetries, defaultRetryWait, errStr)
			}

			select {
			case <-ctx.Done():
				finalErr := fmt.Errorf("%s context cancelled during rate limit wait (attempt %d/%d): %w", logPrefix, retryCount+1, maxRetries, ctx.Err())
				sentry.CaptureException(finalErr)
				return nil, finalErr
			case <-time.After(waitDuration):
				retryCount++
				continue
			}
		}

		// Non-rate limit error
		finalErr := fmt.Errorf("%s failed to send media group (attempt %d/%d): %w", logPrefix, retryCount+1, maxRetries, err)
		sentry.CaptureException(finalErr)
		return nil, finalErr
	}

	// Max retries exceeded
	finalErr := fmt.Errorf("%s max retries (%d) exceeded for sending media group: %w", logPrefix, maxRetries, lastErr)
	sentry.CaptureException(finalErr)
	return nil, finalErr
}

// parseRetryAfter extracts retry duration from error string.
func parseRetryAfter(errorString string) (int, bool) {
	var retryAfter int
	fields := strings.Fields(errorString)
	if len(fields) >= 3 && fields[len(fields)-2] == "after" {
		_, err := fmt.Sscan(fields[len(fields)-1], &retryAfter)
		if err == nil && retryAfter > 0 {
			return retryAfter, true
		}
	}
	// Fallback parsing attempt for specific format
	if _, err := fmt.Sscan(errorString, "telego: sendMediaGroup: api: 429 Too Many Requests: retry after %d", &retryAfter); err == nil && retryAfter > 0 {
		return retryAfter, true
	}
	return 0, false
}
