package engine

import (
	"strings"
	"testing"
	"time"
)

func TestContextManager_UsageAndNearFullCallback(t *testing.T) {
	called := 0
	cm := NewContextManager(100, func() { called++ })

	cm.UpdateFromUsage(TokenUsage{
		InputTokens:  96,
		OutputTokens: 3,
		ProviderID:   "openai",
		ModelID:      "gpt-4o",
	})

	if called != 1 {
		t.Fatalf("expected near-full callback once, got %d", called)
	}
	if cm.TokensUsed() != 99 {
		t.Fatalf("unexpected tokens used: %d", cm.TokensUsed())
	}
	if cm.InputTokens() != 99 {
		t.Fatalf("unexpected input tokens: %d", cm.InputTokens())
	}
	if cm.UsedPct() <= 0.95 {
		t.Fatalf("expected >95%% used, got %.2f", cm.UsedPct())
	}

	in, out := cm.TotalUsage()
	if in != 96 || out != 3 {
		t.Fatalf("unexpected total usage: in=%d out=%d", in, out)
	}
	if cm.TotalCostUSD() <= 0 {
		t.Fatalf("expected tracked cost > 0")
	}
}

func TestContextManager_SetInputAndReports(t *testing.T) {
	cm := NewContextManager(2000, nil)
	cm.SetInputTokens(1400)
	if cm.TokensUsed() != 1400 {
		t.Fatalf("expected 1400 tokens used, got %d", cm.TokensUsed())
	}

	cm.UpdateFromUsage(TokenUsage{
		InputTokens:  1450,
		OutputTokens: 120,
		ProviderID:   "google",
		ModelID:      "gemini-2.0-flash",
	})

	status := cm.StatusBar()
	if !strings.Contains(status, "[") || !strings.Contains(status, "%") || !strings.Contains(status, "$") {
		t.Fatalf("unexpected status bar: %s", status)
	}

	breakdown := cm.ContextBreakdown(100, 900, 300, 120)
	if !strings.Contains(breakdown, "# Context Window") || !strings.Contains(breakdown, "Session Stats") {
		t.Fatalf("missing breakdown sections")
	}
	if !strings.Contains(breakdown, "Warning") && !strings.Contains(breakdown, "Tip") {
		t.Fatalf("expected warning or tip in breakdown")
	}

	cost := cm.CostReport("grok-3", "gemini-2.0-flash")
	if !strings.Contains(cost, "# Session Cost") || !strings.Contains(cost, "Total session cost") {
		t.Fatalf("unexpected cost report: %s", cost)
	}
	if cm.SessionDuration() < 0 {
		t.Fatalf("session duration should be non-negative")
	}
}

func TestContextManager_NilBillingFallbackAndHelpers(t *testing.T) {
	cm := NewContextManager(1000, nil)
	cm.Billing = nil
	cm.UpdateFromUsage(TokenUsage{InputTokens: 200, OutputTokens: 50})

	report := cm.CostReport("primary-model", "consultant-model")
	if !strings.Contains(report, "Provider") {
		t.Fatalf("expected fallback provider table, got: %s", report)
	}
	if !strings.Contains(report, "primary-model") {
		t.Fatalf("expected primary model in fallback report")
	}

	if cm.TokenLimit() != 1000 || cm.MaxTokens() != 1000 {
		t.Fatalf("unexpected token limit/max tokens")
	}
	if cm.TotalCostUSD() != 0 {
		t.Fatalf("expected zero total cost with nil billing")
	}
}

func TestContextHelpers_Formatting(t *testing.T) {
	if got := contextBar(0.5, 4); got != "[##--]" {
		t.Fatalf("unexpected context bar: %s", got)
	}
	if got := contextBar(3, 3); got != "[###]" {
		t.Fatalf("expected clamped full bar, got %s", got)
	}
	if got := contextBar(-1, 3); got != "[---]" {
		t.Fatalf("expected clamped empty bar, got %s", got)
	}
	if pct100(50, 0) != 0 {
		t.Fatalf("pct100 should handle zero total")
	}
	if formatTokens(500) != "500" || formatTokens(2000) != "2.0k" || formatTokens(2_000_000) != "2.0M" {
		t.Fatalf("formatTokens output mismatch")
	}

	_ = time.Second // keep time import explicit for readability in this helper-focused test file
}
