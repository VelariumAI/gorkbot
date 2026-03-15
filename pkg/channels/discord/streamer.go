package discord

import (
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StreamCallback is a function called for each streamed token.
type StreamCallback func(token string)

// StreamingResponder sends an initial placeholder message to a Discord channel
// and then progressively edits it as tokens arrive. A 600ms ticker flushes the
// buffer to Discord (well within the ~5 edits/sec/channel rate limit).
type StreamingResponder struct {
	session   *discordgo.Session
	channelID string
	messageID string

	mu     sync.Mutex
	buf    strings.Builder
	done   chan struct{}
	ticker *time.Ticker
}

// NewStreamingResponder sends a placeholder "▌" message and starts the flush
// ticker. Call Finalize() after the stream is complete.
func NewStreamingResponder(session *discordgo.Session, channelID string) (*StreamingResponder, error) {
	msg, err := session.ChannelMessageSend(channelID, "▌")
	if err != nil {
		return nil, err
	}
	sr := &StreamingResponder{
		session:   session,
		channelID: channelID,
		messageID: msg.ID,
		done:      make(chan struct{}),
		ticker:    time.NewTicker(600 * time.Millisecond),
	}
	go sr.flushLoop()
	return sr, nil
}

// Push appends a token to the buffer. It satisfies the StreamCallback signature.
func (sr *StreamingResponder) Push(token string) {
	sr.mu.Lock()
	sr.buf.WriteString(token)
	sr.mu.Unlock()
}

// Finalize stops the ticker, performs a final edit without the cursor, and
// closes the done channel.
func (sr *StreamingResponder) Finalize() {
	sr.ticker.Stop()
	close(sr.done)

	sr.mu.Lock()
	text := sr.buf.String()
	sr.mu.Unlock()

	if text == "" {
		text = "_(no response)_"
	}
	// Final edit: strip the typing cursor.
	_ = sr.editMessage(text)
}

// flushLoop edits the Discord message every 600ms with the current buffer.
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
	// Discord message limit is 2000 characters; trim if necessary.
	const maxLen = 1990
	runes := []rune(text)
	if len(runes) > maxLen {
		text = string(runes[len(runes)-maxLen:]) // keep the tail (latest content)
	}
	edit := discordgo.NewMessageEdit(sr.channelID, sr.messageID)
	edit.Content = &text
	_, err := sr.session.ChannelMessageEditComplex(edit)
	return err
}
