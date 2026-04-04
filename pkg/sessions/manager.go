package sessions

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionContextManager orchestrates multiple isolated user sessions
type SessionContextManager struct {
	sessions map[string]*SessionContext
	byUser   map[string][]string // userID → sessionIDs
	mu       sync.RWMutex

	// Configuration
	defaultBudget      float64
	sessionTimeout     time.Duration
	defaultProviders   []string
	maxSessionsPerUser int

	// Cleanup
	cleanupTicker *time.Ticker
	stopCh        chan struct{}

	logger *slog.Logger
}

// NewSessionContextManager creates a new session manager
func NewSessionContextManager(config SessionManagerConfig) *SessionContextManager {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.DefaultBudget <= 0 {
		config.DefaultBudget = 10.0 // $10 default
	}

	if config.SessionTimeout == 0 {
		config.SessionTimeout = 24 * time.Hour
	}

	if config.MaxSessionsPerUser == 0 {
		config.MaxSessionsPerUser = 5
	}

	scm := &SessionContextManager{
		sessions:           make(map[string]*SessionContext),
		byUser:             make(map[string][]string),
		defaultBudget:      config.DefaultBudget,
		sessionTimeout:     config.SessionTimeout,
		defaultProviders:   config.DefaultProviders,
		maxSessionsPerUser: config.MaxSessionsPerUser,
		logger:             config.Logger,
		stopCh:             make(chan struct{}),
	}

	// Start cleanup goroutine (remove expired sessions every minute)
	scm.cleanupTicker = time.NewTicker(1 * time.Minute)
	go scm.cleanupLoop()

	return scm
}

// CreateSession creates a new isolated session for a user
func (scm *SessionContextManager) CreateSession(
	ctx context.Context,
	userID, namespace string,
) (*SessionContext, error) {
	scm.mu.Lock()
	defer scm.mu.Unlock()

	// Check max sessions per user
	userSessions := scm.byUser[userID]
	if len(userSessions) >= scm.maxSessionsPerUser {
		scm.logger.Warn("max sessions per user reached",
			slog.String("user", userID),
			slog.Int("max", scm.maxSessionsPerUser),
		)
		return nil, ErrMaxSessionsExceeded
	}

	sessionID := uuid.New().String()

	session := NewSessionContext(
		sessionID,
		userID,
		namespace,
		scm.defaultBudget,
		scm.defaultProviders,
		scm.logger,
	)

	session.ExpiresAt = time.Now().Add(scm.sessionTimeout)

	scm.sessions[sessionID] = session
	scm.byUser[userID] = append(scm.byUser[userID], sessionID)

	scm.logger.Info("session created",
		slog.String("session_id", sessionID),
		slog.String("user_id", userID),
		slog.String("namespace", namespace),
	)

	return session, nil
}

// GetSession retrieves a session by ID
func (scm *SessionContextManager) GetSession(sessionID string) (*SessionContext, error) {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	session, ok := scm.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	return session, nil
}

// GetUserSessions returns all active sessions for a user
func (scm *SessionContextManager) GetUserSessions(userID string) []*SessionContext {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	sessionIDs := scm.byUser[userID]
	var sessions []*SessionContext

	for _, sid := range sessionIDs {
		if session, ok := scm.sessions[sid]; ok && !session.IsExpired() {
			sessions = append(sessions, session)
		}
	}

	return sessions
}

// ListSessions returns metadata for all active sessions
func (scm *SessionContextManager) ListSessions() []SessionMetadata {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	var metadata []SessionMetadata
	now := time.Now()

	for _, session := range scm.sessions {
		if !session.IsExpired() {
			metadata = append(metadata, SessionMetadata{
				ID:              session.ID,
				UserID:          session.UserID,
				Namespace:       session.Namespace,
				CreatedAt:       session.CreatedAt,
				UpdatedAt:       session.UpdatedAt,
				ExpiresAt:       session.ExpiresAt,
				TimeRemaining:   session.ExpiresAt.Sub(now),
				BudgetRemaining: session.GetBudgetRemaining(),
				BudgetSpent:     session.BudgetSpent,
				MessageCount:    len(session.GetHistory().GetMessages()),
			})
		}
	}

	return metadata
}

// CloseSession closes a session and cleans up resources
func (scm *SessionContextManager) CloseSession(sessionID string) error {
	scm.mu.Lock()
	defer scm.mu.Unlock()

	session, ok := scm.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	// Remove from byUser index
	userSessions := scm.byUser[session.UserID]
	for i, sid := range userSessions {
		if sid == sessionID {
			scm.byUser[session.UserID] = append(userSessions[:i], userSessions[i+1:]...)
			break
		}
	}

	// Close session resources
	session.Close()

	// Remove from active sessions
	delete(scm.sessions, sessionID)

	scm.logger.Info("session closed",
		slog.String("session_id", sessionID),
		slog.String("user_id", session.UserID),
	)

	return nil
}

// CloseUserSessions closes all sessions for a user
func (scm *SessionContextManager) CloseUserSessions(userID string) error {
	scm.mu.Lock()
	sessionIDs := make([]string, len(scm.byUser[userID]))
	copy(sessionIDs, scm.byUser[userID])
	scm.mu.Unlock()

	for _, sessionID := range sessionIDs {
		scm.CloseSession(sessionID)
	}

	return nil
}

// RefreshSession extends session timeout
func (scm *SessionContextManager) RefreshSession(sessionID string) error {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	session, ok := scm.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	session.RefreshExpiry(scm.sessionTimeout)
	return nil
}

// GetSessionCount returns total active session count
func (scm *SessionContextManager) GetSessionCount() int {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	count := 0
	for _, session := range scm.sessions {
		if !session.IsExpired() {
			count++
		}
	}

	return count
}

// GetUserSessionCount returns active session count for a user
func (scm *SessionContextManager) GetUserSessionCount(userID string) int {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	sessionIDs := scm.byUser[userID]
	count := 0

	for _, sid := range sessionIDs {
		if session, ok := scm.sessions[sid]; ok && !session.IsExpired() {
			count++
		}
	}

	return count
}

// GetStats returns statistics about all sessions
func (scm *SessionContextManager) GetStats() map[string]interface{} {
	scm.mu.RLock()
	defer scm.mu.RUnlock()

	totalSessions := 0
	totalBudgetSpent := 0.0
	totalMessages := 0
	userCount := 0

	for _, session := range scm.sessions {
		if !session.IsExpired() {
			totalSessions++
			totalBudgetSpent += session.BudgetSpent
			// Messages are internal to ConversationHistory
		}
	}

	userCount = len(scm.byUser)

	return map[string]interface{}{
		"total_sessions":       totalSessions,
		"unique_users":         userCount,
		"total_budget_spent":   totalBudgetSpent,
		"total_messages":       totalMessages,
		"avg_budget_per_user":  0,
		"max_sessions_per_user": scm.maxSessionsPerUser,
	}
}

// cleanupLoop periodically removes expired sessions
func (scm *SessionContextManager) cleanupLoop() {
	for {
		select {
		case <-scm.cleanupTicker.C:
			scm.cleanupExpiredSessions()
		case <-scm.stopCh:
			scm.cleanupTicker.Stop()
			return
		}
	}
}

// cleanupExpiredSessions removes all expired sessions
func (scm *SessionContextManager) cleanupExpiredSessions() {
	scm.mu.Lock()
	defer scm.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for sessionID, session := range scm.sessions {
		if now.After(session.ExpiresAt) {
			// Remove from byUser index
			userSessions := scm.byUser[session.UserID]
			for i, sid := range userSessions {
				if sid == sessionID {
					scm.byUser[session.UserID] = append(userSessions[:i], userSessions[i+1:]...)
					break
				}
			}

			session.Close()
			delete(scm.sessions, sessionID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		scm.logger.Info("cleaned up expired sessions", slog.Int("count", expiredCount))
	}
}

// Close stops the manager and closes all sessions
func (scm *SessionContextManager) Close() error {
	close(scm.stopCh)

	scm.mu.Lock()
	defer scm.mu.Unlock()

	for _, session := range scm.sessions {
		session.Close()
	}

	scm.sessions = make(map[string]*SessionContext)
	scm.byUser = make(map[string][]string)

	scm.logger.Info("session manager closed")
	return nil
}

// SessionManagerConfig holds configuration for SessionContextManager
type SessionManagerConfig struct {
	DefaultBudget      float64
	SessionTimeout     time.Duration
	DefaultProviders   []string
	MaxSessionsPerUser int
	Logger             *slog.Logger
}

// SessionMetadata is a lightweight representation of a session
type SessionMetadata struct {
	ID              string
	UserID          string
	Namespace       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ExpiresAt       time.Time
	TimeRemaining   time.Duration
	BudgetRemaining float64
	BudgetSpent     float64
	MessageCount    int
}
