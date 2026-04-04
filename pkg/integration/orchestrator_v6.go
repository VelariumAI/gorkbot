package integration

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/budget"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/sessions"
)

// OrchestratorV6 wires together SessionContext, BudgetEnforcer, and HybridSearcher
// This is the integrated core of Phase 1 (Foundation) and Phase 2.2 (Memory MVP)
type OrchestratorV6 struct {
	// Session Management (Phase 1.1)
	sessionMgr *sessions.SessionContextManager

	// Budget Enforcement (Phase 1.2)
	budgetEnforcer *budget.BudgetEnforcer

	// Memory System (Phase 2.2)
	memoryFactory MemoryFactory // Creates per-session HybridSearcher

	// Per-session databases
	sessionDbs map[string]*sql.DB
	sessionSearchers map[string]*memory.HybridSearcher
	dbMu       sync.RWMutex

	// Configuration
	config OrchestratorConfig

	// Lifecycle
	logger *slog.Logger
}

// OrchestratorConfig holds all configuration for v6.0
type OrchestratorConfig struct {
	// Session config
	DefaultBudget      float64
	SessionTimeout     time.Duration
	MaxSessionsPerUser int

	// Budget config
	BudgetPolicy *budget.BudgetPolicy

	// Memory config
	MemoryConfig memory.SearchConfig

	// Database path for per-session memory
	MemoryDBPath string // e.g., "/tmp/gorkbot_memory"

	// Logger
	Logger *slog.Logger
}

// MemoryFactory creates memory systems for sessions
type MemoryFactory interface {
	CreateMemorySystem(sessionID string) (*memory.HybridSearcher, *sql.DB, error)
	CloseMemorySystem(sessionID string) error
}

// DefaultMemoryFactory creates in-memory SQLite databases
type DefaultMemoryFactory struct {
	logger *slog.Logger
}

// CreateMemorySystem creates a new in-memory HybridSearcher with SQLite backend
func (dmf *DefaultMemoryFactory) CreateMemorySystem(sessionID string) (*memory.HybridSearcher, *sql.DB, error) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create memory db for session %s: %w", sessionID, err)
	}

	// Create lexical searcher
	lexical := memory.NewFTS5LexicalSearcher(db, dmf.logger)

	// Create fact searcher
	facts := memory.NewSQLiteFactSearcher(db, dmf.logger)

	// Create hybrid searcher (will degrade gracefully)
	config := memory.SearchConfig{
		Logger:         dmf.logger,
		TopK:           8,
		FusionK:        60,
		LexicalWeight:  1.0,
		FactWeight:     1.0,
		SemanticWeight: 1.5,
		RerankerWeight: 1.2,
	}

	hs := memory.NewHybridSearcher(db, lexical, facts, config)

	return hs, db, nil
}

// CloseMemorySystem closes database resources
func (dmf *DefaultMemoryFactory) CloseMemorySystem(sessionID string) error {
	return nil // In-memory databases auto-cleanup
}

// NewOrchestratorV6 creates an integrated v6.0 orchestrator
func NewOrchestratorV6(config OrchestratorConfig) (*OrchestratorV6, error) {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.DefaultBudget <= 0 {
		config.DefaultBudget = 10.0
	}

	if config.SessionTimeout == 0 {
		config.SessionTimeout = 24 * time.Hour
	}

	if config.MaxSessionsPerUser == 0 {
		config.MaxSessionsPerUser = 5
	}

	if config.BudgetPolicy == nil {
		config.BudgetPolicy = budget.NewBudgetPolicy()
	}

	// Initialize session manager (Phase 1.1)
	sessionMgr := sessions.NewSessionContextManager(sessions.SessionManagerConfig{
		DefaultBudget:      config.DefaultBudget,
		SessionTimeout:     config.SessionTimeout,
		DefaultProviders:   []string{}, // Providers can be set per-session
		MaxSessionsPerUser: config.MaxSessionsPerUser,
		Logger:             config.Logger,
	})

	// Initialize budget enforcer (Phase 1.2)
	budgetEnforcer := budget.NewBudgetEnforcer(config.BudgetPolicy, config.Logger)

	// Initialize memory factory
	memoryFactory := &DefaultMemoryFactory{logger: config.Logger}

	orch := &OrchestratorV6{
		sessionMgr:     sessionMgr,
		budgetEnforcer: budgetEnforcer,
		memoryFactory:  memoryFactory,
		sessionDbs:     make(map[string]*sql.DB),
		sessionSearchers: make(map[string]*memory.HybridSearcher),
		config:         config,
		logger:         config.Logger,
	}

	config.Logger.Info("OrchestratorV6 initialized",
		slog.Float64("default_budget", config.DefaultBudget),
		slog.Int("max_sessions_per_user", config.MaxSessionsPerUser),
	)

	return orch, nil
}

// CreateUserSession creates a complete isolated session with budget + memory
func (ov6 *OrchestratorV6) CreateUserSession(ctx context.Context, userID string, namespace string) (*SessionState, error) {
	// 1. Create session context (Phase 1.1)
	sessionCtx, err := ov6.sessionMgr.CreateSession(ctx, userID, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	sessionID := sessionCtx.ID

	// 2. Initialize budget for this session (Phase 1.2)
	ov6.budgetEnforcer.InitializeSession(sessionID, ov6.config.DefaultBudget)

	// 3. Create memory system for this session (Phase 2.2)
	hybridSearcher, memDB, err := ov6.memoryFactory.CreateMemorySystem(sessionID)
	if err != nil {
		ov6.sessionMgr.CloseSession(sessionID)
		return nil, fmt.Errorf("failed to create memory system: %w", err)
	}

	// Store database reference
	ov6.dbMu.Lock()
	ov6.sessionDbs[sessionID] = memDB
	ov6.sessionSearchers[sessionID] = hybridSearcher
	ov6.dbMu.Unlock()

	// Wire memory system to session
	sessionCtx.MemoryDB = memDB

	ov6.logger.Info("user session created with full wiring",
		slog.String("session_id", sessionID),
		slog.String("user_id", userID),
		slog.Float64("budget", ov6.config.DefaultBudget),
	)

	return &SessionState{
		SessionContext: sessionCtx,
		HybridSearcher: hybridSearcher,
		MemoryDB:       memDB,
	}, nil
}

// ExecuteQuery orchestrates a complete query execution with budget + memory
func (ov6 *OrchestratorV6) ExecuteQuery(ctx context.Context, sessionID string, userID string, query string, model string, tokens ...int) (*QueryResult, error) {
	// Validate session
	sessionCtx, err := ov6.sessionMgr.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session: %w", err)
	}

	// Estimate cost (Phase 1.2)
	var inputTokens, outputTokens int
	if len(tokens) >= 2 {
		inputTokens, outputTokens = tokens[0], tokens[1]
	}

	estimate := ov6.budgetEnforcer.EstimateCost(ctx, model, inputTokens, outputTokens)

	// Check budget (Phase 1.2)
	decision := ov6.budgetEnforcer.CanUseModel(ctx, sessionID, userID, model, estimate.EstimatedCost)

	if decision.Status == budget.Denied {
		return &QueryResult{
			Success:       false,
			Error:         decision.DenialReason,
			FallbackModel: decision.FallbackModel,
			Status:        "budget_denied",
		}, nil
	}

	// Prepare enrichment from memory (Phase 2.2)
	ov6.dbMu.RLock()
	memDB, exists := ov6.sessionDbs[sessionID]
	hs := ov6.sessionSearchers[sessionID]
	ov6.dbMu.RUnlock()

	var memoryFacts []memory.SearchResult
	if exists && memDB != nil && hs != nil {
		memoryFacts, _ = hs.Search(ctx, query, 8)
	}

	// Build enriched prompt with memory facts
	enrichedPrompt := query
	if len(memoryFacts) > 0 {
		enrichedPrompt += "\n\n[Context from memory]"
		for i, fact := range memoryFacts {
			if i >= 3 { // Limit to 3 facts for token efficiency
				break
			}
			enrichedPrompt += fmt.Sprintf("\n- %s | %s | %s", fact.Subject, fact.Predicate, fact.Object)
		}
	}

	// Deduct budget (Phase 1.2)
	err = ov6.budgetEnforcer.DeductCost(ctx, sessionID, userID, estimate.EstimatedCost)
	if err != nil {
		return nil, fmt.Errorf("failed to deduct cost: %w", err)
	}

	// Record actual cost for future estimates
	// (In real implementation, would use actual response tokens)
	ov6.budgetEnforcer.RecordModelCost(model, estimate.EstimatedCost)

	// Add message to session history
	sessionCtx.AddMessage(ctx, "user", query)
	sessionCtx.AddMessage(ctx, "assistant", "[Response would be here]")

	result := &QueryResult{
		Success:          decision.Status == budget.Approved || decision.Status == budget.ApprovedWarn,
		SessionID:        sessionID,
		Query:            query,
		EnrichedPrompt:   enrichedPrompt,
		Model:            model,
		CostEstimated:    estimate.EstimatedCost,
		BudgetRemaining:  decision.RemainingBudget,
		MemoryFacts:      len(memoryFacts),
		BudgetWarning:    decision.WarningMessage,
		Status:           string(decision.Status),
	}

	return result, nil
}

// GetSessionStats returns complete session statistics
func (ov6 *OrchestratorV6) GetSessionStats(sessionID string) map[string]interface{} {
	sessionCtx, err := ov6.sessionMgr.GetSession(sessionID)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	stats := sessionCtx.GetStats()
	stats["budget_remaining"] = ov6.budgetEnforcer.GetSessionBudget(sessionID)
	stats["audit_log_size"] = len(sessionCtx.GetAuditLog())

	return stats
}

// CloseSession closes a session and cleans up all resources
func (ov6 *OrchestratorV6) CloseSession(sessionID string) error {
	// Close budget tracking
	ov6.budgetEnforcer.CloseSession(sessionID)

	// Close memory database
	ov6.dbMu.Lock()
	if hs, exists := ov6.sessionSearchers[sessionID]; exists {
		_ = hs.Close()
		delete(ov6.sessionSearchers, sessionID)
	}
	if db, exists := ov6.sessionDbs[sessionID]; exists {
		db.Close()
		delete(ov6.sessionDbs, sessionID)
	}
	ov6.dbMu.Unlock()

	// Close session context
	return ov6.sessionMgr.CloseSession(sessionID)
}

// Close closes the entire orchestrator
func (ov6 *OrchestratorV6) Close() error {
	// Close all session databases
	ov6.dbMu.Lock()
	for sessionID, hs := range ov6.sessionSearchers {
		_ = hs.Close()
		delete(ov6.sessionSearchers, sessionID)
	}
	for sessionID, db := range ov6.sessionDbs {
		db.Close()
		delete(ov6.sessionDbs, sessionID)
	}
	ov6.dbMu.Unlock()

	// Close session manager
	return ov6.sessionMgr.Close()
}

// SessionState represents a complete user session with all systems wired
type SessionState struct {
	SessionContext *sessions.SessionContext
	HybridSearcher *memory.HybridSearcher
	MemoryDB       *sql.DB
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	Success          bool
	SessionID        string
	Query            string
	EnrichedPrompt   string
	Model            string
	CostEstimated    float64
	BudgetRemaining  float64
	MemoryFacts      int
	BudgetWarning    string
	FallbackModel    string
	Error            string
	Status           string // "approved", "approved_warn", "budget_denied"
}
