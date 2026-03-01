package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Config holds runtime configuration for the Telegram bot.
type Config struct {
	Token         string  // Telegram bot token from @BotFather
	AllowedUsers  []int64 // User IDs allowed to interact (empty = all users)
	SessionShared bool    // If true, share conversation history with TUI session
}

// MessageHandler is called when a message arrives.
// Returns the response string to send back to the user.
type MessageHandler func(ctx context.Context, userID int64, username, text string) (string, error)

// Bot wraps the Telegram bot API and routes messages to a MessageHandler.
type Bot struct {
	cfg     Config
	api     *tgbotapi.BotAPI
	handler MessageHandler
	logger  *slog.Logger
	stop    chan struct{}
	mu      sync.Mutex
	running bool
}

// NewBot creates a new Bot but does not start polling.
func NewBot(cfg Config, handler MessageHandler, logger *slog.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("telegram: init bot API: %w", err)
	}
	api.Debug = false

	return &Bot{
		cfg:     cfg,
		api:     api,
		handler: handler,
		logger:  logger,
		stop:    make(chan struct{}),
	}, nil
}

// Start begins polling for updates. Blocks until ctx is cancelled or Stop is called.
func (b *Bot) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return fmt.Errorf("telegram: bot already running")
	}
	b.running = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
	}()

	updateCfg := tgbotapi.NewUpdate(0)
	updateCfg.Timeout = 60

	updates := b.api.GetUpdatesChan(updateCfg)

	b.logger.Info("telegram bot started", "username", b.api.Self.UserName)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return nil
		case <-b.stop:
			b.api.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil {
				continue
			}
			msg := update.Message
			if strings.TrimSpace(msg.Text) == "" {
				continue
			}

			chatID := msg.Chat.ID
			userID := msg.From.ID
			username := msg.From.UserName

			if !b.isAllowed(userID) {
				_ = b.SendMessage(chatID, "Unauthorized.")
				continue
			}

			text := strings.TrimSpace(msg.Text)

			switch text {
			case "/start":
				_ = b.SendMessage(chatID, "Gorkbot connected. Send any message to chat.")
				continue
			case "/clear":
				_ = b.SendMessage(chatID, "Session cleared.")
				continue
			}

			response, err := b.handler(ctx, userID, username, text)
			if err != nil {
				b.logger.Error("telegram handler error", "err", err, "userID", userID)
				_ = b.SendMessage(chatID, fmt.Sprintf("Error: %s", err.Error()))
				continue
			}

			if err := b.SendMessage(chatID, response); err != nil {
				b.logger.Error("telegram send error", "err", err, "chatID", chatID)
			}
		}
	}
}

// Stop signals the polling loop to exit.
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		close(b.stop)
	}
}

// IsRunning reports whether the bot is currently polling.
func (b *Bot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// SendMessage sends text to the given chat, splitting on 4000-char boundaries.
func (b *Bot) SendMessage(chatID int64, text string) error {
	const maxLen = 4000
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			idx := strings.LastIndex(text[:maxLen], "\n")
			if idx < 100 {
				idx = maxLen
			}
			chunk = text[:idx]
			text = text[idx:]
		} else {
			text = ""
		}

		outMsg := tgbotapi.NewMessage(chatID, chunk)
		outMsg.ParseMode = "Markdown"
		if _, err := b.api.Send(outMsg); err != nil {
			// Retry without Markdown — common when AI output has unmatched backticks
			outMsg.ParseMode = ""
			if _, err2 := b.api.Send(outMsg); err2 != nil {
				return err2
			}
		}
	}
	return nil
}

// isAllowed reports whether userID may interact with the bot.
func (b *Bot) isAllowed(userID int64) bool {
	if len(b.cfg.AllowedUsers) == 0 {
		return true
	}
	for _, id := range b.cfg.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}
