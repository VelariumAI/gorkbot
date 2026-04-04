package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/billing"
)

func TestBudgetGuard_EstimateCostAndSessionCost(t *testing.T) {
	bm := billing.NewBillingManagerWithDir(t.TempDir())
	bg := NewBudgetGuard(bm, 1.0, 2.0)

	est := bg.EstimateCost("openai", "gpt-4o", 1_000_000, 1_000_000)
	if est <= 0 {
		t.Fatalf("expected positive estimate for priced model")
	}
	if bg.EstimateCost("unknown", "x", 100, 100) != 0 {
		t.Fatalf("expected zero estimate for unknown provider")
	}

	bm.TrackTurn("openai", "gpt-4o", 500_000, 250_000)
	if bg.SessionCost() <= 0 {
		t.Fatalf("expected positive session cost after tracked turn")
	}

	var nilBG *BudgetGuard
	if got := nilBG.SessionCost(); got != 0 {
		t.Fatalf("expected zero session cost for nil guard, got %f", got)
	}
}

func TestBudgetGuard_CheckAndTrack_AllowWarnBlock(t *testing.T) {
	bm := billing.NewBillingManagerWithDir(t.TempDir())
	bg := NewBudgetGuard(bm, 0.004, 0.02)

	allow := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if allow.Action != BudgetAllow {
		t.Fatalf("expected allow action, got %v", allow.Action)
	}

	// Simulate near-session-limit spend to force warn.
	bm.TrackTurn("google", "gemini-2.0-flash", 24_000, 0) // ~0.0024
	warn := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if warn.Action != BudgetWarn {
		t.Fatalf("expected warn action, got %v", warn.Action)
	}
	if !strings.Contains(strings.ToLower(warn.Message), "approaching") {
		t.Fatalf("unexpected warn message: %s", warn.Message)
	}

	// Push over session limit to force block.
	bm.TrackTurn("google", "gemini-2.0-flash", 20_000, 0) // now above session limit
	block := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if block.Action != BudgetBlock {
		t.Fatalf("expected block action, got %v", block.Action)
	}
	if !strings.Contains(strings.ToLower(block.Message), "budget exceeded") {
		t.Fatalf("unexpected block message: %s", block.Message)
	}
}

func TestBudgetGuard_DailyResetAndDailyChecks(t *testing.T) {
	bm := billing.NewBillingManagerWithDir(t.TempDir())
	bg := NewBudgetGuard(bm, 0, 0.003)
	bg.WarnAt = 0.5

	bg.dailySpend = 0.0018
	warn := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if warn.Action != BudgetWarn {
		t.Fatalf("expected warn on projected daily usage, got %v", warn.Action)
	}

	bg.dailySpend = 0.0023
	block := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if block.Action != BudgetBlock {
		t.Fatalf("expected block on projected daily usage, got %v", block.Action)
	}

	bg.dailySpend = 1.0
	bg.lastReset = time.Now().Add(-25 * time.Hour)
	allow := bg.CheckAndTrack("google", "gemini-2.0-flash", 100, 100)
	if allow.Action != BudgetAllow {
		t.Fatalf("expected allow after reset, got %v", allow.Action)
	}
	if bg.dailySpend >= 1.0 {
		t.Fatalf("expected daily spend reset and re-accrual, got %f", bg.dailySpend)
	}
}

func TestBudgetGuard_NilBehavior(t *testing.T) {
	var bg *BudgetGuard
	decision := bg.CheckAndTrack("openai", "gpt-4o", 1, 1)
	if decision.Action != BudgetAllow {
		t.Fatalf("expected allow for nil guard")
	}

	bg2 := NewBudgetGuard(nil, 1, 1)
	if got := bg2.EstimateCost("openai", "gpt-4o", 1, 1); got != 0 {
		t.Fatalf("expected zero estimate with nil billing")
	}
}
