package telegram

import (
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StreamCallback is a function called for each streamed token.
type StreamCallback func(token string)

// StreamingResponder sends an initial placeholder Telegram message and then
// progressively edits it as tokens arrive. A 600ms ticker batches edits to
// stay within Telegram's ~20 messages/minute/chat rate limit.
type StreamingResponder struct {
	api       *tgbotapi.BotAPI
	chatID    int64
	messageID int

	mu     sync.Mutex
	buf    strings.Builder
	done   chan struct{}
	ticker *time.Ticker
}

// NewStreamingResponder sends a "▌" placeholder and starts the flush ticker.
// Call Finalize() after the stream is complete.
func NewStreamingResponder(api *tgbotapi.BotAPI, chatID int64) (*StreamingResponder, error) {
	msg := tgbotapi.NewMessage(chatID, "▌")
	sent, err := api.Send(msg)
	if err != nil {
		return nil, err
	}
	sr := &StreamingResponder{
		api:       api,
		chatID:    chatID,
		messageID: sent.MessageID,
		done:      make(chan struct{}),
		ticker:    time.NewTicker(600 * time.Millisecond),
	}
	go sr.flushLoop()
	return sr, nil
}

// Push appends a token to the internal buffer.
func (sr *StreamingResponder) Push(token string) {
	sr.mu.Lock()
	sr.buf.WriteString(token)
	sr.mu.Unlock()
}

// Finalize stops the ticker, does a final edit removing the cursor, and closes.
func (sr *StreamingResponder) Finalize() {
	sr.ticker.Stop()
	close(sr.done)

	sr.mu.Lock()
	text := sr.buf.String()
	sr.mu.Unlock()

	if text == "" {
		text = "_(no response)_"
	}
	_ = sr.editMessage(text)
}

// flushLoop edits the Telegram message every 600ms with the current buffer.
func (sr *StreamingResponder) flushLoop() {
	for {
		select {
		case <-sr.done:
			return
		case <-sr.ticker.C:
			sr.mu.Lock()
			text := sr.buf.String()
			sr.mu.Unlock()
			if text != "" {
				_ = sr.editMessage(text + "▌")
			}
		}
	}
}

func (sr *StreamingResponder) editMessage(text string) error {
	// Telegram message limit is 4096 characters; trim if needed.
	const maxLen = 4000
	if len(text) > maxLen {
		runes := []rune(text)
		if len(runes) > maxLen {
			text = string(runes[len(runes)-maxLen:])
		}
	}
	edit := tgbotapi.NewEditMessageText(sr.chatID, sr.messageID, text)
	_, err := sr.api.Send(edit)
	return err
}
