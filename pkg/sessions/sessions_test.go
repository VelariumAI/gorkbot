package sessions

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestSessionContextCreation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	sc := NewSessionContext("sess-1", "alice", "default", 10.0, []string{"grok", "claude"}, logger)

	if sc.ID != "sess-1" {
		t.Errorf("expected ID sess-1, got %s", sc.ID)
	}

	if sc.UserID != "alice" {
		t.Errorf("expected UserID alice, got %s", sc.UserID)
	}

	if sc.BudgetRemaining != 10.0 {
		t.Errorf("expected budget 10.0, got %f", sc.BudgetRemaining)
	}

	if len(sc.AllowedProviders) != 2 {
		t.Errorf("expected 2 allowed providers, got %d", len(sc.AllowedProviders))
	}
}

func TestSessionIsolation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	sc1 := NewSessionContext("sess-1", "alice", "default", 10.0, nil, logger)
	sc2 := NewSessionContext("sess-2", "bob", "default", 5.0, nil, logger)

	// Add messages to session 1
	sc1.AddMessage(ctx, "user", "Hello from Alice")
	sc1.AddMessage(ctx, "assistant", "Hi Alice")

	// Add messages to session 2
	sc2.AddMessage(ctx, "user", "Hello from Bob")
	sc2.AddMessage(ctx, "assistant", "Hi Bob")

	// Verify isolation
	// Verify histories are tracked
	hist1 := sc1.GetHistory()
	hist2 := sc2.GetHistory()

	if hist1 == nil || hist2 == nil {
		t.Error("both sessions should have conversation histories")
	}

	// Verify different budgets
	if sc1.GetBudgetRemaining() != 10.0 {
		t.Errorf("alice should have 10.0 budget, got %f", sc1.GetBudgetRemaining())
	}

	if sc2.GetBudgetRemaining() != 5.0 {
		t.Errorf("bob should have 5.0 budget, got %f", sc2.GetBudgetRemaining())
	}
}

func TestBudgetEnforcement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	sc := NewSessionContext("sess-1", "alice", "default", 5.0, nil, logger)

	// Deduct within budget
	err := sc.DeductBudget(ctx, 2.0, "grok")
	if err != nil {
		t.Errorf("deduction should succeed, got error: %v", err)
	}

	remaining := sc.GetBudgetRemaining()
	if remaining != 3.0 {
		t.Errorf("expected remaining 3.0, got %f", remaining)
	}

	// Deduct again within budget
	err = sc.DeductBudget(ctx, 1.5, "claude")
	if err != nil {
		t.Errorf("deduction should succeed, got error: %v", err)
	}

	remaining = sc.GetBudgetRemaining()
	if remaining != 1.5 {
		t.Errorf("expected remaining 1.5, got %f", remaining)
	}

	// Try to deduct more than remaining (should fail)
	err = sc.DeductBudget(ctx, 2.0, "gemini")
	if err != ErrBudgetExhausted {
		t.Errorf("expected budget exhausted error, got: %v", err)
	}

	// Budget should not have changed
	remaining = sc.GetBudgetRemaining()
	if remaining != 1.5 {
		t.Errorf("budget should stay at 1.5 after failed deduction, got %f", remaining)
	}
}

func TestProviderWhitelist(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	sc := NewSessionContext("sess-1", "alice", "default", 10.0, []string{"grok", "claude"}, logger)

	// Test allowed providers
	if !sc.CanUseProvider("grok") {
		t.Error("grok should be allowed")
	}

	if !sc.CanUseProvider("claude") {
		t.Error("claude should be allowed")
	}

	// Test disallowed provider
	if sc.CanUseProvider("gemini") {
		t.Error("gemini should not be allowed")
	}

	// Update whitelist
	sc.UpdateProviderWhitelist([]string{"gemini"})

	// Test updated whitelist
	if sc.CanUseProvider("grok") {
		t.Error("grok should not be allowed after update")
	}

	if !sc.CanUseProvider("gemini") {
		t.Error("gemini should be allowed after update")
	}
}

func TestAuditLog(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	sc := NewSessionContext("sess-1", "alice", "default", 10.0, nil, logger)

	// Perform actions
	sc.DeductBudget(ctx, 2.0, "grok")
	sc.UpdateProviderWhitelist([]string{"grok"})

	// Check audit log
	log := sc.GetAuditLog()

	// Should have: session.created, budget.deducted, whitelist.updated
	if len(log) < 3 {
		t.Errorf("expected at least 3 audit entries, got %d", len(log))
	}

	// Verify first entry is creation
	if log[0].Action != "session.created" {
		t.Errorf("first entry should be session.created, got %s", log[0].Action)
	}
}

func TestSessionExpiry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	sc := NewSessionContext("sess-1", "alice", "default", 10.0, nil, logger)

	// Session should not be expired initially
	if sc.IsExpired() {
		t.Error("new session should not be expired")
	}

	// Set expiry to past
	sc.ExpiresAt = time.Now().Add(-1 * time.Hour)

	if !sc.IsExpired() {
		t.Error("session should be expired when ExpiresAt is in past")
	}

	// Refresh expiry
	sc.RefreshExpiry(24 * time.Hour)

	if sc.IsExpired() {
		t.Error("refreshed session should not be expired")
	}
}

func TestSessionManagerCreateSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 3,
		Logger:             logger,
	})
	defer mgr.Close()

	// Create session
	sc, err := mgr.CreateSession(ctx, "alice", "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if sc.UserID != "alice" {
		t.Errorf("expected alice, got %s", sc.UserID)
	}

	if sc.BudgetRemaining != 10.0 {
		t.Errorf("expected 10.0 budget, got %f", sc.BudgetRemaining)
	}
}

func TestSessionManagerMultiUser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 5,
		Logger:             logger,
	})
	defer mgr.Close()

	// Create sessions for Alice
	s1, _ := mgr.CreateSession(ctx, "alice", "default")
	s2, _ := mgr.CreateSession(ctx, "alice", "default")

	// Create session for Bob
	s3, _ := mgr.CreateSession(ctx, "bob", "default")

	// Verify counts
	aliceSessions := mgr.GetUserSessions("alice")
	if len(aliceSessions) != 2 {
		t.Errorf("alice should have 2 sessions, got %d", len(aliceSessions))
	}

	bobSessions := mgr.GetUserSessions("bob")
	if len(bobSessions) != 1 {
		t.Errorf("bob should have 1 session, got %d", len(bobSessions))
	}

	totalSessions := mgr.GetSessionCount()
	if totalSessions != 3 {
		t.Errorf("total should be 3 sessions, got %d", totalSessions)
	}

	// Verify isolation: change budget in alice's first session
	s1.DeductBudget(ctx, 3.0, "test")

	// Bob's session should not be affected
	if s3.GetBudgetRemaining() != 10.0 {
		t.Errorf("bob should still have 10.0, got %f", s3.GetBudgetRemaining())
	}

	// Alice's second session should not be affected
	if s2.GetBudgetRemaining() != 10.0 {
		t.Errorf("alice's second session should still have 10.0, got %f", s2.GetBudgetRemaining())
	}
}

func TestSessionManagerMaxSessions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		MaxSessionsPerUser: 2, // Max 2 sessions per user
		Logger:             logger,
	})
	defer mgr.Close()

	// Create 2 sessions (should work)
	_, _ = mgr.CreateSession(ctx, "alice", "default")
	_, _ = mgr.CreateSession(ctx, "alice", "default")

	// Try to create 3rd (should fail)
	_, err := mgr.CreateSession(ctx, "alice", "default")
	if err != ErrMaxSessionsExceeded {
		t.Errorf("expected MaxSessionsExceeded, got: %v", err)
	}
}

func TestSessionManagerGetSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		Logger:             logger,
	})
	defer mgr.Close()

	sc, _ := mgr.CreateSession(ctx, "alice", "default")
	sessionID := sc.ID

	// Get session
	retrieved, err := mgr.GetSession(sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrieved.ID != sessionID {
		t.Errorf("expected %s, got %s", sessionID, retrieved.ID)
	}

	// Try to get non-existent session
	_, err = mgr.GetSession("non-existent")
	if err != ErrSessionNotFound {
		t.Errorf("expected SessionNotFound, got: %v", err)
	}
}

func TestSessionManagerCloseSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		Logger:             logger,
	})
	defer mgr.Close()

	sc, _ := mgr.CreateSession(ctx, "alice", "default")
	sessionID := sc.ID

	// Close session
	err := mgr.CloseSession(sessionID)
	if err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	// Try to get closed session (should fail)
	_, err = mgr.GetSession(sessionID)
	if err != ErrSessionNotFound {
		t.Errorf("expected SessionNotFound, got: %v", err)
	}

	// Session count should be 0
	if mgr.GetSessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", mgr.GetSessionCount())
	}
}

func TestSessionManagerListSessions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		Logger:             logger,
	})
	defer mgr.Close()

	mgr.CreateSession(ctx, "alice", "default")
	mgr.CreateSession(ctx, "bob", "default")

	metadata := mgr.ListSessions()

	if len(metadata) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(metadata))
	}

	// Check metadata contains expected fields
	for _, m := range metadata {
		if m.BudgetRemaining != 10.0 {
			t.Errorf("expected budget 10.0, got %f", m.BudgetRemaining)
		}
	}
}

func TestSessionManagerStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:      10.0,
		SessionTimeout:     1 * time.Hour,
		Logger:             logger,
	})
	defer mgr.Close()

	mgr.CreateSession(ctx, "alice", "default")
	mgr.CreateSession(ctx, "bob", "default")

	stats := mgr.GetStats()

	if stats["total_sessions"] != 2 {
		t.Errorf("expected 2 sessions, got %d", stats["total_sessions"])
	}

	if stats["unique_users"] != 2 {
		t.Errorf("expected 2 users, got %d", stats["unique_users"])
	}
}

func BenchmarkSessionCreation(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	mgr := NewSessionContextManager(SessionManagerConfig{
		DefaultBudget:  10.0,
		SessionTimeout: 1 * time.Hour,
		Logger:         logger,
	})
	defer mgr.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mgr.CreateSession(ctx, "user", "default")
	}
}

func BenchmarkBudgetDeduction(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	sc := NewSessionContext("sess", "user", "default", 1000000.0, nil, logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sc.DeductBudget(ctx, 0.001, "test")
	}
}
