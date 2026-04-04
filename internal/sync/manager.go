package sync

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Session represents a cross-channel session
type Session struct {
	ID        string
	Channels  map[string]*ChannelState
	StartTime time.Time
	State     map[string]interface{}
}

// ChannelState represents state in a specific channel
type ChannelState struct {
	Channel    string
	Status     string // "active", "inactive", "error"
	LastUpdate time.Time
	Data       map[string]interface{}
}

// SyncManager coordinates sessions across channels
type SyncManager struct {
	logger   *slog.Logger
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSyncManager creates a new sync manager
func NewSyncManager(logger *slog.Logger) *SyncManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &SyncManager{
		logger:   logger,
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new cross-channel session
func (sm *SyncManager) CreateSession(channels []string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := &Session{
		ID:        fmt.Sprintf("session-%d", time.Now().UnixNano()),
		Channels:  make(map[string]*ChannelState),
		StartTime: time.Now(),
		State:     make(map[string]interface{}),
	}

	for _, channel := range channels {
		session.Channels[channel] = &ChannelState{
			Channel:    channel,
			Status:     "active",
			LastUpdate: time.Now(),
			Data:       make(map[string]interface{}),
		}
	}

	sm.sessions[session.ID] = session

	sm.logger.Debug("created cross-channel session",
		slog.String("id", session.ID),
		slog.Int("channels", len(channels)),
	)

	return session
}

// GetSession retrieves a session
func (sm *SyncManager) GetSession(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.sessions[id]
}

// SyncState synchronizes state across channels
func (sm *SyncManager) SyncState(sessionID string, channel string, data map[string]interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update global session state
	for k, v := range data {
		session.State[k] = v
	}

	// Propagate to other channels
	for channelName := range session.Channels {
		if channelName != channel {
			sm.propagateToChannel(session, channelName, data)
		}
	}

	sm.logger.Debug("synced state across channels",
		slog.String("session", sessionID),
		slog.String("from_channel", channel),
		slog.Int("channels", len(session.Channels)),
	)

	return nil
}

// propagateToChannel sends data to a specific channel
func (sm *SyncManager) propagateToChannel(session *Session, channelName string, data map[string]interface{}) {
	channelState := session.Channels[channelName]
	if channelState == nil {
		return
	}

	// Format data for channel
	formatted := sm.formatForChannel(channelName, data)

	// Update channel state
	channelState.Data = formatted
	channelState.LastUpdate = time.Now()

	sm.logger.Debug("propagated state to channel",
		slog.String("session", session.ID),
		slog.String("channel", channelName),
	)
}

// formatForChannel formats data for specific channel
func (sm *SyncManager) formatForChannel(channel string, data map[string]interface{}) map[string]interface{} {
	formatted := make(map[string]interface{})

	for k, v := range data {
		switch channel {
		case "discord":
			formatted[k] = sm.formatAsDiscordMessage(k, v)
		case "telegram":
			formatted[k] = sm.formatAsTelegramMessage(k, v)
		case "whatsapp":
			formatted[k] = sm.formatAsWhatsAppMessage(k, v)
		default:
			formatted[k] = v
		}
	}

	return formatted
}

// formatAsDiscordMessage formats message for Discord
func (sm *SyncManager) formatAsDiscordMessage(key string, value interface{}) string {
	return fmt.Sprintf("**%s**: %v", key, value)
}

// formatAsTelegramMessage formats message for Telegram
func (sm *SyncManager) formatAsTelegramMessage(key string, value interface{}) string {
	return fmt.Sprintf("`%s`: %v", key, value)
}

// formatAsWhatsAppMessage formats message for WhatsApp
func (sm *SyncManager) formatAsWhatsAppMessage(key string, value interface{}) string {
	return fmt.Sprintf("%s: %v", key, value)
}

// CloseSession closes a session
func (sm *SyncManager) CloseSession(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.sessions[id]; !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	delete(sm.sessions, id)

	sm.logger.Debug("closed session", slog.String("id", id))

	return nil
}

// GetStats returns sync statistics
func (sm *SyncManager) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"active_sessions": len(sm.sessions),
	}
}
