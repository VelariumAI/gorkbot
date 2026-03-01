package telegram

import (
	"context"
	"log/slog"
	"sync"
)

// Manager wraps a Bot with lifecycle management and config persistence
type Manager struct {
	mu        sync.Mutex
	configDir string
	bot       *Bot
	cancel    context.CancelFunc
	logger    *slog.Logger
}

// NewManager creates a Manager. Call Start() to enable the bot.
func NewManager(configDir string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{configDir: configDir, logger: logger}
}

// IsEnabled returns true if Telegram integration is configured and enabled
func (m *Manager) IsEnabled() bool {
	cfg, err := LoadConfig(m.configDir)
	if err != nil {
		return false
	}
	return cfg.Enabled && cfg.Token != ""
}

// Start initializes and starts the Telegram bot using the stored config.
// handler is called with each incoming message text.
func (m *Manager) Start(handler MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := LoadConfig(m.configDir)
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.Token == "" {
		return nil // silently skip if not configured
	}

	bot, err := NewBot(Config{
		Token:         cfg.Token,
		AllowedUsers:  cfg.AllowedUsers,
		SessionShared: true,
	}, handler, m.logger)
	if err != nil {
		return err
	}
	m.bot = bot

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		if err := bot.Start(ctx); err != nil {
			m.logger.Warn("telegram bot exited", "err", err)
		}
	}()
	m.logger.Info("Telegram bot started")
	return nil
}

// Stop shuts down the running bot
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.bot != nil {
		m.bot.Stop()
		m.bot = nil
		m.logger.Info("Telegram bot stopped")
	}
}

// Status returns a human-readable status string
func (m *Manager) Status() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bot == nil {
		if m.IsEnabled() {
			return "configured but not running"
		}
		return "not configured"
	}
	if m.bot.IsRunning() {
		return "running"
	}
	return "stopped"
}

// SendMessage sends a message via the bot (no-op if not running)
func (m *Manager) SendMessage(chatID int64, text string) error {
	m.mu.Lock()
	bot := m.bot
	m.mu.Unlock()
	if bot == nil {
		return nil
	}
	return bot.SendMessage(chatID, text)
}
