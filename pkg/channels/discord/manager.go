package discord

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Manager wraps a Bot with lifecycle management and optional config persistence.
//
// Security design: The Discord bot token is read exclusively from the
// DISCORD_BOT_TOKEN environment variable and is never written to disk.
// The on-disk discord.json only stores allow-lists and the enabled flag,
// making it safe to commit the config directory to version control.
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

// IsEnabled returns true when:
//   - DISCORD_BOT_TOKEN is present in the environment, AND
//   - either no discord.json config file exists (default = enabled), OR
//     the file exists with "enabled": true.
func (m *Manager) IsEnabled() bool {
	if os.Getenv("DISCORD_BOT_TOKEN") == "" {
		return false
	}
	// No config file → default to enabled when a token is present.
	cfgPath := filepath.Join(m.configDir, configFilename)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return true
	}
	cfg, err := LoadConfig(m.configDir)
	if err != nil {
		return false
	}
	return cfg.Enabled
}

// Start initialises and starts the Discord bot using DISCORD_BOT_TOKEN and the
// optional allow-list configuration in discord.json.
//
// When DISCORD_BOT_TOKEN is absent Start is a graceful no-op — it logs a
// warning and returns nil so the main application continues unaffected.
// handler is called for every incoming message and should return the response.
func (m *Manager) Start(handler MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		m.logger.Warn("discord: DISCORD_BOT_TOKEN not set — Discord bot disabled")
		return nil
	}

	cfg, _ := LoadConfig(m.configDir)

	bot, err := NewBot(Config{
		Token:         token,
		AllowedUsers:  cfg.AllowedUsers,
		AllowedGuilds: cfg.AllowedGuilds,
	}, handler, m.logger)
	if err != nil {
		return err
	}
	m.bot = bot

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	go func() {
		if err := bot.Start(ctx); err != nil {
			m.logger.Warn("discord bot exited", "err", err)
		}
	}()

	m.logger.Info("Discord bot started")
	return nil
}

// Stop gracefully shuts down the running bot and cancels its context.
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
		m.logger.Info("Discord bot stopped")
	}
}

// Status returns a human-readable status string suitable for display in /status.
func (m *Manager) Status() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bot == nil {
		if os.Getenv("DISCORD_BOT_TOKEN") == "" {
			return "not configured (DISCORD_BOT_TOKEN not set)"
		}
		return "configured but not running"
	}
	if m.bot.IsRunning() {
		return "running"
	}
	return "stopped"
}

// SendToChannel sends a message to a Discord channel on behalf of the agent.
//
// This is the primary outbound hook used by the discord_send tool so the agent
// can proactively push messages to Discord without waiting for a user prompt.
// It is a safe no-op (returns nil) when the bot is not running.
func (m *Manager) SendToChannel(channelID, text string) error {
	m.mu.Lock()
	bot := m.bot
	m.mu.Unlock()
	if bot == nil {
		return nil
	}
	return bot.SendToChannel(channelID, text)
}

// SetStreamHandler wires a streaming handler onto the running bot.
// Safe to call before or after Start; no-op when the bot is nil.
func (m *Manager) SetStreamHandler(sh StreamHandler) {
	m.mu.Lock()
	bot := m.bot
	m.mu.Unlock()
	if bot != nil {
		bot.SetStreamHandler(sh)
	}
}

// SetBridgeRegistry wires the cross-channel identity registry onto the bot.
func (m *Manager) SetBridgeRegistry(r BridgeRegistry) {
	m.mu.Lock()
	bot := m.bot
	m.mu.Unlock()
	if bot != nil {
		bot.SetBridgeRegistry(r)
	}
}
