package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// MessageHandler is called when a Discord message arrives.
//
// userID is the author's Discord snowflake (string), username is the display
// name, text is the full message content. The returned string is sent back to
// the originating channel.
type MessageHandler func(ctx context.Context, userID, username, text string) (string, error)

// Config holds runtime configuration for the Discord bot.
type Config struct {
	Token         string   // Discord bot token (from DISCORD_BOT_TOKEN env var)
	AllowedUsers  []string // Snowflake IDs allowed to interact; empty = all users
	AllowedGuilds []string // Guild IDs to restrict to; empty = all guilds
}

// StreamHandler is called for streaming responses. cb receives individual tokens.
type StreamHandler func(ctx context.Context, userID, username, text string, cb StreamCallback) error

// BridgeRegistry is the minimal interface the bot needs from the channel bridge.
type BridgeRegistry interface {
	GetOrCreate(platform, platformUserID, username string) (string, error)
	GenerateLinkCode(canonicalID string) (string, error)
	ConsumeLink(code, platform, platformUserID string) (string, error)
	LinkedPlatforms(canonicalID string) ([]string, error)
}

// Bot wraps a discordgo.Session and routes incoming Discord messages through a
// MessageHandler, enabling bidirectional communication between the agent and
// Discord users.
type Bot struct {
	cfg           Config
	session       *discordgo.Session
	handler       MessageHandler
	streamHandler StreamHandler
	linkRegistry  BridgeRegistry
	logger        *slog.Logger

	// selfID is populated from the Ready event; used to skip the bot's own messages.
	selfID string

	// inflight tracks users with an active in-progress request so we can
	// debounce duplicate sends without blocking the discordgo event loop.
	inflightMu sync.Mutex
	inflight   map[string]bool // key = Discord user snowflake

	mu      sync.Mutex
	running bool
}

// SetStreamHandler sets a streaming handler. When set, incoming messages use
// progressive Discord message editing instead of the blocking MessageHandler.
func (b *Bot) SetStreamHandler(sh StreamHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.streamHandler = sh
}

// SetBridgeRegistry wires the cross-channel identity registry.
func (b *Bot) SetBridgeRegistry(r BridgeRegistry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.linkRegistry = r
}

// getStreamHandler safely returns the stream handler (may be nil).
func (b *Bot) getStreamHandler() StreamHandler {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.streamHandler
}

// NewBot creates a Bot but does not open a connection.
func NewBot(cfg Config, handler MessageHandler, logger *slog.Logger) (*Bot, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("discord: DISCORD_BOT_TOKEN is required")
	}

	// discordgo requires the "Bot " prefix on the token.
	token := cfg.Token
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	dg, err := discordgo.New(token)
	if err != nil {
		return nil, fmt.Errorf("discord: create session: %w", err)
	}

	// Request the minimum gateway intents needed to receive and read messages:
	//
	//   IntentsGuildMessages  — messages posted in server channels
	//   IntentsDirectMessages — DMs sent directly to the bot
	//   IntentMessageContent  — PRIVILEGED: access to the body of messages
	//
	// IMPORTANT: IntentMessageContent must be toggled ON in the Discord Developer
	// Portal → Bot → Privileged Gateway Intents before the bot will receive text.
	// Without it, m.Content will always be empty and all messages are silently dropped.
	dg.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	b := &Bot{
		cfg:      cfg,
		session:  dg,
		handler:  handler,
		logger:   logger,
		inflight: make(map[string]bool),
	}

	// Register all gateway event handlers before Open() is called.
	dg.AddHandler(b.onReady)
	dg.AddHandler(b.onMessageCreate)
	dg.AddHandler(func(s *discordgo.Session, e *discordgo.Disconnect) {
		b.logger.Warn("discord: WebSocket disconnected — discordgo will reconnect automatically")
	})

	return b, nil
}

// Start opens the Discord WebSocket and blocks until ctx is cancelled.
//
// On initial connection failure it retries with exponential backoff (1 s → 2 min).
// Once the session is open, discordgo manages heartbeating and reconnects
// internally for transient network blips.
func (b *Bot) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return fmt.Errorf("discord: bot already running")
	}
	b.running = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
	}()

	backoff := time.Second
	const maxBackoff = 2 * time.Minute

	for {
		b.logger.Info("discord: opening WebSocket connection")
		if err := b.session.Open(); err != nil {
			b.logger.Error("discord: open failed", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			}
		}

		backoff = time.Second // reset on successful connect

		// Block until context is cancelled; discordgo handles internal reconnects.
		<-ctx.Done()
		b.logger.Info("discord: context cancelled — closing WebSocket")
		return b.session.Close()
	}
}

// Stop closes the session idempotently; safe to call from any goroutine.
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		_ = b.session.Close()
	}
}

// IsRunning reports whether the bot currently has an open session.
func (b *Bot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// SendToChannel pushes text to a Discord channel, splitting on Discord's
// 2000-character hard limit.
func (b *Bot) SendToChannel(channelID, text string) error {
	return b.splitAndSend(channelID, text)
}

// ─── Gateway Event Handlers ───────────────────────────────────────────────────

// onReady fires once the session is fully authenticated.
func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.selfID = r.User.ID
	b.logger.Info("discord: ready",
		"bot_id", r.User.ID,
		"username", r.User.Username,
		"guilds", len(r.Guilds))
}

// onMessageCreate handles every new message delivered by the gateway.
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// ── Diagnostic: log every raw event so we can see what the gateway delivers.
	b.logger.Debug("discord: message event",
		"author_id", func() string {
			if m.Author != nil {
				return m.Author.ID
			}
			return "<nil>"
		}(),
		"content_len", len(m.Content),
		"channel", m.ChannelID,
		"guild", m.GuildID,
	)

	// Skip messages with no author data.
	if m.Author == nil {
		b.logger.Debug("discord: dropped — nil author")
		return
	}

	// Skip the bot's own messages.
	if m.Author.ID == b.selfID {
		return
	}

	// ── Detect missing MessageContent intent ─────────────────────────────────
	// If m.Content is empty for a real user message, the most common cause is
	// that the "Message Content Intent" privileged toggle is OFF in the Discord
	// Developer Portal. Log a clear warning so it shows up in the Gorkbot log.
	if m.Content == "" {
		b.logger.Warn("discord: message content is empty — check that 'Message Content Intent' is enabled in Discord Developer Portal → Bot → Privileged Gateway Intents",
			"author", m.Author.Username,
			"channel", m.ChannelID)
		return
	}

	content := strings.TrimSpace(m.Content)

	// Strip the bot's own @mention that Discord prepends when a user @-tags the
	// bot in a server channel (e.g. "<@123456789> hello" → "hello").
	// Discord sends two formats: <@ID> and the legacy <@!ID>.
	if b.selfID != "" {
		content = strings.TrimSpace(strings.TrimPrefix(content, "<@!"+b.selfID+">"))
		content = strings.TrimSpace(strings.TrimPrefix(content, "<@"+b.selfID+">"))
	}

	if content == "" {
		b.logger.Debug("discord: dropped — content empty after stripping @mention")
		return
	}

	userID := m.Author.ID
	username := m.Author.Username

	b.logger.Info("discord: incoming message",
		"user", username,
		"content_preview", truncate(content, 60))

	// Enforce the optional user and guild allow-lists.
	if !b.isUserAllowed(userID) {
		b.logger.Info("discord: rejected — user not in allow-list", "userID", userID)
		_, _ = s.ChannelMessageSend(m.ChannelID, "Unauthorized.")
		return
	}
	if m.GuildID != "" && !b.isGuildAllowed(m.GuildID) {
		b.logger.Info("discord: ignored — guild not in allow-list", "guildID", m.GuildID)
		return
	}

	// Built-in lightweight commands that bypass the LLM.
	switch content {
	case "!ping":
		_, sendErr := s.ChannelMessageSend(m.ChannelID,
			"🤖 Gorkbot online. Routing active. Try any message.")
		b.logger.Info("discord: !ping", "send_err", sendErr)
		return

	case "!status":
		// Probe write permission: if this message reaches Discord the bot can write.
		msg := fmt.Sprintf("**Gorkbot status**\nBot ID: `%s`\nSession: active\nMessage Content Intent: ✅ working (you can read this)\nWrite permission: ✅ working (you can see this message)", b.selfID)
		if _, err := s.ChannelMessageSend(m.ChannelID, msg); err != nil {
			b.logger.Error("discord: !status send failed", "err", err)
		}
		return

	case "!help":
		_, _ = s.ChannelMessageSend(m.ChannelID,
			"**Gorkbot** — AI assistant\n"+
				"`!ping` — check connectivity\n"+
				"`!status` — show bot status + permissions\n"+
				"`!clear` — clear session context\n"+
				"`!link` — generate a cross-channel link code\n"+
				"`!whoami` — show your canonical identity\n"+
				"Any other message → routed to the AI agent.")
		return

	case "!clear":
		_, _ = s.ChannelMessageSend(m.ChannelID, "Session context cleared. Start fresh.")
		return

	case "!link":
		if b.linkRegistry != nil {
			canonicalID, _ := b.linkRegistry.GetOrCreate("discord", userID, username)
			code, err := b.linkRegistry.GenerateLinkCode(canonicalID)
			if err != nil {
				_, _ = s.ChannelMessageSend(m.ChannelID, "❌ Failed to generate link code.")
				return
			}
			_, _ = s.ChannelMessageSend(m.ChannelID,
				fmt.Sprintf("Your link code: **%s** (valid 10 min)\nIn Telegram: `!link %s`", code, code))
		} else {
			_, _ = s.ChannelMessageSend(m.ChannelID, "_Cross-channel bridge not configured._")
		}
		return

	case "!whoami":
		if b.linkRegistry != nil {
			canonicalID, _ := b.linkRegistry.GetOrCreate("discord", userID, username)
			platforms, _ := b.linkRegistry.LinkedPlatforms(canonicalID)
			_, _ = s.ChannelMessageSend(m.ChannelID,
				fmt.Sprintf("Canonical ID: `%s`\nLinked platforms: %v", canonicalID, platforms))
		} else {
			_, _ = s.ChannelMessageSend(m.ChannelID, "_Cross-channel bridge not configured._")
		}
		return
	}

	// Handle !link <code> for consuming a link code.
	if len(content) > 6 && content[:6] == "!link " {
		code := strings.TrimSpace(content[6:])
		if b.linkRegistry != nil {
			if _, err := b.linkRegistry.ConsumeLink(code, "discord", userID); err != nil {
				_, _ = s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Link failed: %v", err))
			} else {
				_, _ = s.ChannelMessageSend(m.ChannelID, "✅ Accounts linked! Your conversation history is now shared across channels.")
			}
		}
		return
	}

	// Per-user in-flight guard: debounce concurrent requests from the same user
	// without blocking the discordgo dispatcher goroutine.
	b.inflightMu.Lock()
	if b.inflight[userID] {
		b.inflightMu.Unlock()
		_, _ = s.ChannelMessageSend(m.ChannelID, "_Still thinking… please wait._")
		return
	}
	b.inflight[userID] = true
	b.inflightMu.Unlock()

	// Hand off to a goroutine so the discordgo event loop is never blocked.
	go func() {
		// ── Panic guard: catch any crash in handler/orchestrator and report it.
		defer func() {
			if r := recover(); r != nil {
				b.logger.Error("discord: handler goroutine panicked", "panic", r)
				_, _ = s.ChannelMessageSend(m.ChannelID,
					fmt.Sprintf("⚠ Internal error (panic): %v", r))
			}
		}()
		defer func() {
			b.inflightMu.Lock()
			delete(b.inflight, userID)
			b.inflightMu.Unlock()
		}()

		// Show a typing indicator while the agent is working.
		if err := s.ChannelTyping(m.ChannelID); err != nil {
			b.logger.Warn("discord: typing indicator failed", "err", err)
		}

		// ── LLM integration hook ──────────────────────────────────────────────
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		b.logger.Info("discord: calling handler", "user", username, "content_len", len(content))

		// ── Prefer streaming handler when available ────────────────────────────
		sh := b.getStreamHandler()
		if sh != nil {
			sr, srErr := NewStreamingResponder(s, m.ChannelID)
			if srErr != nil {
				b.logger.Warn("discord: streaming responder init failed, falling back", "err", srErr)
				sh = nil
			} else {
				err := sh(ctx, userID, username, content, sr.Push)
				sr.Finalize()
				if err != nil {
					b.logger.Error("discord: stream handler error", "err", err)
					_, _ = s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("⚠ Error: %s", err.Error()))
				}
				return
			}
		}

		// ── Fallback: blocking handler ────────────────────────────────────────
		response, err := b.handler(ctx, userID, username, content)
		if err != nil {
			b.logger.Error("discord: handler returned error", "err", err, "user", username)
			if _, sendErr := s.ChannelMessageSend(m.ChannelID,
				fmt.Sprintf("⚠ Error: %s", err.Error())); sendErr != nil {
				// Can't even send the error — log it so it's visible in Gorkbot logs.
				b.logger.Error("discord: ALSO failed to send error message",
					"send_err", sendErr,
					"original_err", err)
			}
			return
		}

		b.logger.Info("discord: handler succeeded", "response_len", len(response))

		// ── Response pipe-back hook ───────────────────────────────────────────
		if err := b.splitAndSend(m.ChannelID, response); err != nil {
			b.logger.Error("discord: splitAndSend failed", "err", err, "channelID", m.ChannelID)
			// The send failed — try a minimal fallback so the user knows something happened.
			_, _ = s.ChannelMessageSend(m.ChannelID,
				"⚠ Got a response from the AI but failed to deliver it to this channel. Check bot permissions (Send Messages).")
		}
	}()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// splitAndSend sends text to channelID in chunks ≤ 1900 chars (Discord's hard
// limit is 2000; we leave headroom for Discord's own overhead).
func (b *Bot) splitAndSend(channelID, text string) error {
	const maxLen = 1900
	if text == "" {
		return nil
	}
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			// Prefer splitting on a newline boundary to avoid cutting mid-word.
			idx := strings.LastIndex(text[:maxLen], "\n")
			if idx < 100 {
				idx = maxLen
			}
			chunk = text[:idx]
			text = text[idx:]
		} else {
			text = ""
		}
		if _, err := b.session.ChannelMessageSend(channelID, chunk); err != nil {
			return fmt.Errorf("discord send: %w", err)
		}
	}
	return nil
}

// isUserAllowed returns true when userID may interact with the bot.
func (b *Bot) isUserAllowed(userID string) bool {
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

// isGuildAllowed returns true when the guild is on the allow-list (or the list is empty).
func (b *Bot) isGuildAllowed(guildID string) bool {
	if len(b.cfg.AllowedGuilds) == 0 {
		return true
	}
	for _, id := range b.cfg.AllowedGuilds {
		if id == guildID {
			return true
		}
	}
	return false
}

// truncate clips s to maxLen runes for log previews.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
