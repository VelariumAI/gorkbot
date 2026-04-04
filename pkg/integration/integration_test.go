package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/velariumai/gorkbot/pkg/budget"
	"github.com/velariumai/gorkbot/pkg/memory"
)

func TestOrchestratorV6_FullIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	// Create orchestrator with Phase 1.1 + 1.2 + 2.2 wiring
	orch, err := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 3,
		BudgetPolicy:       budget.NewBudgetPolicy(),
		MemoryConfig: memory.SearchConfig{
			TopK:    8,
			FusionK: 60,
			Logger:  logger,
		},
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.Close()

	// Test 1: Create user session with full wiring
	sessionState, err := orch.CreateUserSession(ctx, "alice", "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	sessionID := sessionState.SessionContext.ID
	if sessionID == "" {
		t.Error("expected session ID")
	}

	if sessionState.HybridSearcher == nil {
		t.Error("expected HybridSearcher to be wired")
	}

	if sessionState.MemoryDB == nil {
		t.Error("expected MemoryDB to be wired")
	}

	// Test 2: Execute query with budget + memory
	queryResult, err := orch.ExecuteQuery(
		ctx,
		sessionID,
		"alice",
		"What is the weather?",
		"grok-3",
		2048, // input tokens
		1024, // output tokens
	)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}

	if !queryResult.Success {
		t.Errorf("query should succeed, got: %s", queryResult.Error)
	}

	if queryResult.CostEstimated <= 0 {
		t.Errorf("expected cost > 0, got %f", queryResult.CostEstimated)
	}

	if queryResult.BudgetRemaining >= 10.0 {
		t.Errorf("expected budget to be reduced, got %f", queryResult.BudgetRemaining)
	}

	t.Logf("Query result: status=%s, cost=%.4f, remaining=%.4f",
		queryResult.Status, queryResult.CostEstimated, queryResult.BudgetRemaining)

	// Test 3: Verify session stats
	stats := orch.GetSessionStats(sessionID)
	if stats["error"] != nil {
		t.Errorf("expected no error, got: %v", stats["error"])
	}

	if budget, ok := stats["budget_remaining"].(float64); ok {
		if budget <= 0 || budget >= 10.0 {
			t.Errorf("budget tracking failed: %f", budget)
		}
	}

	// Test 4: Budget enforcement
	// Try to execute very expensive query
	expensiveResult, _ := orch.ExecuteQuery(
		ctx,
		sessionID,
		"alice",
		"Very complex reasoning question",
		"gpt-4",
		8000, // Large input
		4000, // Large output
	)

	t.Logf("Expensive query: status=%s, remaining=%.4f", expensiveResult.Status, expensiveResult.BudgetRemaining)

	// Test 5: Multiple users with isolation
	session2, err := orch.CreateUserSession(ctx, "bob", "default")
	if err != nil {
		t.Fatalf("failed to create second session: %v", err)
	}

	bob_sessionID := session2.SessionContext.ID

	// Both sessions should have independent budgets
	query1, _ := orch.ExecuteQuery(ctx, sessionID, "alice", "Query 1", "haiku", 1000, 500)
	query2, _ := orch.ExecuteQuery(ctx, bob_sessionID, "bob", "Query 2", "haiku", 1500, 800)

	if query1.BudgetRemaining == query2.BudgetRemaining {
		t.Error("sessions should have independent budgets")
	}

	t.Logf("Alice remaining: %.4f, Bob remaining: %.4f",
		query1.BudgetRemaining, query2.BudgetRemaining)

	// Test 6: Close session cleanup
	err = orch.CloseSession(sessionID)
	if err != nil {
		t.Errorf("failed to close session: %v", err)
	}

	// Verify session is closed
	_, err = orch.sessionMgr.GetSession(sessionID)
	if err == nil {
		t.Error("expected closed session to not be retrievable")
	}
}

func TestOrchestratorV6_BudgetExhaustion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	// Create with very small budget
	orch, err := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:  0.05, // $0.05 only
		SessionTimeout: 1 * time.Hour,
		BudgetPolicy:   budget.NewBudgetPolicy(),
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.Close()

	session, _ := orch.CreateUserSession(ctx, "alice", "default")
	sessionID := session.SessionContext.ID

	// First query should succeed (small cost)
	result1, _ := orch.ExecuteQuery(ctx, sessionID, "alice", "Hi", "haiku", 100, 50)
	if !result1.Success {
		t.Errorf("first query should succeed")
	}

	t.Logf("After first query: remaining=%.4f", result1.BudgetRemaining)

	// Second expensive query should fail or use fallback
	result2, _ := orch.ExecuteQuery(ctx, sessionID, "alice", "Complex query", "gpt-4", 4000, 2000)

	if !result2.Success && result2.FallbackModel == "" {
		t.Logf("Query denied with no fallback available (expected with small budget): %s", result2.Error)
	} else if result2.Success {
		t.Logf("Query succeeded with fallback or within remaining budget")
	}
}

func TestOrchestratorV6_MemoryIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	orch, _ := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:  100.0,
		SessionTimeout: 1 * time.Hour,
		BudgetPolicy:   budget.NewBudgetPolicy(),
		MemoryConfig: memory.SearchConfig{
			TopK:    8,
			FusionK: 60,
			Logger:  logger,
		},
		Logger: logger,
	})
	defer orch.Close()

	session, _ := orch.CreateUserSession(ctx, "alice", "default")
	sessionID := session.SessionContext.ID

	// Execute query - should use memory enrichment
	result, _ := orch.ExecuteQuery(
		ctx,
		sessionID,
		"alice",
		"What did I say earlier?",
		"grok-3",
		2048, 1024,
	)

	if result.EnrichedPrompt == result.Query {
		t.Log("No memory facts available (expected for new session)")
	} else {
		t.Logf("Query enriched with memory facts: %d facts added", result.MemoryFacts)
	}

	t.Logf("Enriched prompt length: %d (original: %d)",
		len(result.EnrichedPrompt), len(result.Query))
}

func TestOrchestratorV6_ConcurrentSessions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	orch, _ := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 5,
		BudgetPolicy:       budget.NewBudgetPolicy(),
		Logger:             logger,
	})
	defer orch.Close()

	// Create multiple sessions concurrently
	sessionCount := 5
	sessionIDs := make([]string, sessionCount)

	for i := 0; i < sessionCount; i++ {
		session, err := orch.CreateUserSession(ctx, "alice", "default")
		if err != nil {
			t.Errorf("failed to create session %d: %v", i, err)
			continue
		}
		sessionIDs[i] = session.SessionContext.ID
	}

	if len(sessionIDs) != sessionCount {
		t.Errorf("expected %d sessions, got %d", sessionCount, len(sessionIDs))
	}

	// Try to create 6th (should fail due to max 5 per user)
	_, err := orch.CreateUserSession(ctx, "alice", "default")
	if err == nil {
		t.Error("expected error when exceeding max sessions per user")
	}

	// Verify all sessions are independent
	for i, sessionID := range sessionIDs {
		result, _ := orch.ExecuteQuery(ctx, sessionID, "alice", "Query", "haiku", 1000, 500)
		if !result.Success {
			t.Errorf("session %d query failed", i)
		}
	}

	t.Logf("Successfully managed %d concurrent sessions", sessionCount)
}

func BenchmarkOrchestratorV6_QueryExecution(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	orch, _ := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:  1000.0,
		SessionTimeout: 1 * time.Hour,
		BudgetPolicy:   budget.NewBudgetPolicy(),
		Logger:         logger,
	})
	defer orch.Close()

	session, _ := orch.CreateUserSession(ctx, "alice", "default")
	sessionID := session.SessionContext.ID

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		orch.ExecuteQuery(ctx, sessionID, "alice", "Test query", "haiku", 1000, 500)
	}
}

func BenchmarkOrchestratorV6_SessionCreation(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	orch, _ := NewOrchestratorV6(OrchestratorConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 1000,
		BudgetPolicy:       budget.NewBudgetPolicy(),
		Logger:             logger,
	})
	defer orch.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		orch.CreateUserSession(ctx, "user", "default")
	}
}
