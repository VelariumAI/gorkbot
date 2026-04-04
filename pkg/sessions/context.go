package sessions

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/memory"
)

// AuditEntry logs actions within a session (budget, credentials, etc)
type AuditEntry struct {
	Timestamp   time.Time
	Action      string
	Details     string
	CostImpact  float64
	Success     bool
}

// SessionContext represents an isolated user session with its own:
// - Conversation history
// - Memory system
// - Budget tracking
// - Credential whitelist
// - Audit log
type SessionContext struct {
	// Identity
	ID        string // Unique session ID (UUID)
	UserID    string // User who owns this session
	Namespace string // Tenant/organization namespace

	// Conversation & Memory
	ConversationHistory *ai.ConversationHistory
	MemoryDB            *sql.DB // Per-session SQLite database
	MemoryManager       *memory.MemoryManager

	// Budget Tracking
	BudgetRemaining float64   // Remaining budget for this session
	BudgetSpent     float64   // Total spent in this session
	AllowedProviders []string // Whitelist of providers user can access

	// Audit Trail
	AuditLog []AuditEntry
	auditMu  sync.Mutex

	// Lifecycle
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time // Session timeout

	// Thread safety
	mu sync.RWMutex

	// Logger
	logger *slog.Logger
}

// NewSessionContext creates an isolated session for a user
func NewSessionContext(
	sessionID, userID, namespace string,
	budget float64,
	allowedProviders []string,
	logger *slog.Logger,
) *SessionContext {
	if logger == nil {
		logger = slog.Default()
	}

	now := time.Now()

	sc := &SessionContext{
		ID:               sessionID,
		UserID:           userID,
		Namespace:        namespace,
		ConversationHistory: ai.NewConversationHistory(),
		BudgetRemaining:  budget,
		BudgetSpent:      0,
		AllowedProviders: allowedProviders,
		AuditLog:         []AuditEntry{},
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        now.Add(24 * time.Hour), // 24h default session timeout
		logger:           logger,
	}

	sc.logAudit(AuditEntry{
		Timestamp:  now,
		Action:     "session.created",
		Details:    "Session initialized for user " + userID,
		CostImpact: 0,
		Success:    true,
	})

	return sc
}

// AddMessage appends a message to conversation history (thread-safe)
func (sc *SessionContext) AddMessage(ctx context.Context, role, content string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.ConversationHistory.AddMessage(role, content)
	sc.UpdatedAt = time.Now()

	return nil
}

// GetHistory returns a copy of conversation history (thread-safe)
func (sc *SessionContext) GetHistory() *ai.ConversationHistory {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.ConversationHistory
}

// GetBudgetRemaining returns remaining budget (thread-safe)
func (sc *SessionContext) GetBudgetRemaining() float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.BudgetRemaining
}

// DeductBudget subtracts cost from session budget, returns error if insufficient
func (sc *SessionContext) DeductBudget(ctx context.Context, amount float64, provider string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if amount > sc.BudgetRemaining {
		sc.logAudit(AuditEntry{
			Timestamp:  time.Now(),
			Action:     "budget.deduction_denied",
			Details:    "Insufficient budget: " + provider,
			CostImpact: amount,
			Success:    false,
		})
		return ErrBudgetExhausted
	}

	sc.BudgetRemaining -= amount
	sc.BudgetSpent += amount
	sc.UpdatedAt = time.Now()

	sc.logAudit(AuditEntry{
		Timestamp:  time.Now(),
		Action:     "budget.deducted",
		Details:    "Provider: " + provider,
		CostImpact: amount,
		Success:    true,
	})

	return nil
}

// CanUseProvider checks if user has whitelist access to provider
func (sc *SessionContext) CanUseProvider(provider string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if len(sc.AllowedProviders) == 0 {
		return true // No whitelist = all allowed
	}

	for _, allowed := range sc.AllowedProviders {
		if allowed == provider {
			return true
		}
	}

	return false
}

// UpdateProviderWhitelist updates the allowed providers for this session
func (sc *SessionContext) UpdateProviderWhitelist(providers []string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.AllowedProviders = providers
	sc.UpdatedAt = time.Now()

	sc.logAudit(AuditEntry{
		Timestamp: time.Now(),
		Action:    "whitelist.updated",
		Details:   "Provider whitelist changed",
		Success:   true,
	})

	return nil
}

// IsExpired checks if session has exceeded timeout
func (sc *SessionContext) IsExpired() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return time.Now().After(sc.ExpiresAt)
}

// RefreshExpiry extends session timeout
func (sc *SessionContext) RefreshExpiry(duration time.Duration) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.ExpiresAt = time.Now().Add(duration)
}

// GetAuditLog returns a copy of the audit log (thread-safe)
func (sc *SessionContext) GetAuditLog() []AuditEntry {
	sc.auditMu.Lock()
	defer sc.auditMu.Unlock()

	logCopy := make([]AuditEntry, len(sc.AuditLog))
	copy(logCopy, sc.AuditLog)
	return logCopy
}

// logAudit appends entry to audit log (internal use)
func (sc *SessionContext) logAudit(entry AuditEntry) {
	sc.auditMu.Lock()
	defer sc.auditMu.Unlock()

	sc.AuditLog = append(sc.AuditLog, entry)

	// Keep audit log bounded (last 10K entries)
	if len(sc.AuditLog) > 10000 {
		sc.AuditLog = sc.AuditLog[len(sc.AuditLog)-10000:]
	}

	sc.logger.Debug("Audit log entry", slog.String("action", entry.Action))
}

// GetStats returns session statistics
func (sc *SessionContext) GetStats() map[string]interface{} {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return map[string]interface{}{
		"session_id":       sc.ID,
		"user_id":          sc.UserID,
		"namespace":        sc.Namespace,
		"created_at":       sc.CreatedAt,
		"updated_at":       sc.UpdatedAt,
		"expires_at":       sc.ExpiresAt,
		"expired":          time.Now().After(sc.ExpiresAt),
		"budget_remaining": sc.BudgetRemaining,
		"budget_spent":     sc.BudgetSpent,
		"message_count":    len(sc.ConversationHistory.GetMessages()),
		"audit_log_size":   len(sc.AuditLog),
	}
}

// Close cleans up session resources
func (sc *SessionContext) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.MemoryDB != nil {
		return sc.MemoryDB.Close()
	}

	return nil
}
