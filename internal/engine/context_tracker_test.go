package engine

import (
	"testing"
)

func TestContextTracker_BasicTracking(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)

	// Update with initial breakdown
	ct.UpdateBreakdown(5000, 20000, 10000, 0)

	if ct.UsagePercent() < 35 || ct.UsagePercent() > 36 {
		t.Errorf("expected ~35%% usage, got %.1f%%", ct.UsagePercent())
	}
}

func TestContextTracker_PredictiveCompaction(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)

	// Simulate growing usage
	for i := 0; i < 10; i++ {
		ct.UpdateBreakdown(5000, int64(20000+i*5000), 10000, 0)
	}

	// Should predict turns until full
	turnsRemaining := ct.turnsUntilFull
	if turnsRemaining <= 0 {
		t.Error("expected positive turns until full")
	}
	if turnsRemaining > 100 {
		t.Errorf("unrealistic prediction: %d turns", turnsRemaining)
	}
}

func TestContextTracker_CompactionTriggers(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)

	// Below threshold
	ct.UpdateBreakdown(10000, 20000, 5000, 0)
	shouldCompact, _ := ct.ShouldCompactNow()
	if shouldCompact {
		t.Error("should not compact at ~35% usage")
	}

	// At threshold - just verify compaction is triggered (could be threshold or predictive)
	ct.UpdateBreakdown(85000, 0, 0, 0)
	shouldCompact, trigger := ct.ShouldCompactNow()
	if !shouldCompact {
		t.Errorf("should trigger compaction at 85%%, got shouldCompact=%v, trigger=%v", shouldCompact, trigger)
	}
	if trigger != TriggerThreshold && trigger != TriggerPredictive {
		t.Errorf("expected TriggerThreshold or TriggerPredictive, got %v", trigger)
	}

	// At critical
	ct.UpdateBreakdown(96000, 0, 0, 0)
	shouldCompact, trigger = ct.ShouldCompactNow()
	if !shouldCompact || trigger != TriggerCritical {
		t.Error("should trigger critical compaction at 96%")
	}
}

func TestContextTracker_CompactionEvent(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)
	ct.UpdateBreakdown(50000, 30000, 10000, 0)

	// Record compaction
	ct.RecordCompaction(TriggerThreshold, 25000)

	history := ct.CompactionHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 event, got %d", len(history))
	}

	event := history[0]
	if event.TokensReclaimed != 25000 {
		t.Errorf("expected 25000 tokens reclaimed, got %d", event.TokensReclaimed)
	}
	if event.Trigger != TriggerThreshold {
		t.Errorf("expected TriggerThreshold, got %v", event.Trigger)
	}
}

func TestContextTracker_BreakdownReport(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)
	ct.UpdateBreakdown(10000, 40000, 20000, 5000)

	report := ct.BreakdownReport()
	if report == "" {
		t.Error("expected non-empty report")
	}
	if !contains(report, "System") || !contains(report, "Conversation") {
		t.Error("report missing component breakdown")
	}
}

func TestContextTracker_Concurrent(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			ct.RecordTurn(int64((idx + 1) * 100))
			ct.UpdateBreakdown(5000, int64(20000+idx*1000), 10000, 0)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if ct.UsagePercent() <= 0 {
		t.Error("expected non-zero usage after concurrent updates")
	}
}

func TestContextTracker_HookFiring(t *testing.T) {
	ct := NewContextTracker(100000, 0.85, 0.95)

	hookCalled := false
	ct.SetCompactionHook(func(event CompactionEvent) {
		hookCalled = true
	})

	ct.UpdateBreakdown(50000, 30000, 10000, 0)
	ct.RecordCompaction(TriggerThreshold, 25000)

	if !hookCalled {
		t.Error("expected compaction hook to be called")
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

