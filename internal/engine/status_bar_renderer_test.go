package engine

import (
	"testing"
)

func TestStatusBarRenderer_ContextUpdate(t *testing.T) {
	sbr := NewStatusBarRenderer(100000)
	sbr.UpdateContext(50000, 100000)

	if sbr.contextUsagePercent.Load() != 50 {
		t.Errorf("expected 50%%, got %d%%", sbr.contextUsagePercent.Load())
	}
}

func TestStatusBarRenderer_CostUpdate(t *testing.T) {
	sbr := NewStatusBarRenderer(100000)
	sbr.UpdateCost(0.25, 4.75, 0.05)

	cost := float64(sbr.sessionCost.Load()) / 1000000
	if cost < 0.24 || cost > 0.26 {
		t.Errorf("expected ~0.25, got %.4f", cost)
	}
}

func TestStatusBarRenderer_Compact(t *testing.T) {
	sbr := NewStatusBarRenderer(100000)
	sbr.UpdateContext(75000, 100000)
	sbr.UpdateCost(0.15, 9.85, 0.02)
	sbr.UpdatePerformance(45, 2, 250)

	compact := sbr.RenderCompact()
	if compact == "" {
		t.Error("expected non-empty compact display")
	}
	if !contains(compact, "$") {
		t.Error("expected cost in compact display")
	}
}

func TestStatusBarRenderer_HealthScore(t *testing.T) {
	sbr := NewStatusBarRenderer(100000)

	// Healthy state
	sbr.UpdateContext(30000, 100000)
	sbr.UpdateCost(0.10, 9.90, 0.01)
	sbr.UpdatePerformance(30, 0, 100)

	score := sbr.HealthScore()
	if score < 80 {
		t.Errorf("expected high health score, got %d", score)
	}

	// Degraded state
	sbr.UpdateContext(95000, 100000)
	sbr.UpdateCost(8.00, 2.00, 1.00)
	sbr.UpdatePerformance(1, 5, 6000)

	score = sbr.HealthScore()
	if score > 40 {
		t.Errorf("expected low health score, got %d", score)
	}
}

func TestStatusBarRenderer_Minimal(t *testing.T) {
	sbr := NewStatusBarRenderer(100000)

	sbr.UpdateContext(50000, 100000)
	minimal := sbr.RenderMinimal()
	if minimal == "" {
		t.Error("expected non-empty minimal display")
	}
}

