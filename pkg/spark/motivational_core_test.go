package spark

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/sense"
)

func TestObserveUpdatesEWMA(t *testing.T) {
	lie := sense.NewLIEEvaluator()
	mc := NewMotivationalCore(lie, 0.5) // high alpha for fast convergence in test

	initial := mc.DriveScore()
	// Observe a long, well-structured response to push drive score up.
	for i := 0; i < 5; i++ {
		mc.Observe("Step 1: reasoning here. Step 2: conclusion.\n1. point one\n2. point two\n```code block```")
	}
	final := mc.DriveScore()
	// With a good response the drive score should shift from 0.5.
	if final == initial {
		t.Error("DriveScore should change after observations")
	}
}

func TestCalibrateWeightsBoost(t *testing.T) {
	lie := sense.NewLIEEvaluator()
	mc := NewMotivationalCore(lie, 0.1)

	// Artificially inject low-reward history.
	mc.mu.Lock()
	for i := 0; i < 10; i++ {
		mc.history = append(mc.history, 0.1)
	}
	mc.mu.Unlock()

	origLength := lie.LengthWeight
	mc.CalibrateWeights()
	if lie.LengthWeight <= origLength {
		t.Errorf("LengthWeight should increase when mean reward < 0.3: before=%.3f after=%.3f", origLength, lie.LengthWeight)
	}
}

func TestCalibrateWeightsRelax(t *testing.T) {
	lie := sense.NewLIEEvaluator()
	mc := NewMotivationalCore(lie, 0.1)

	// Artificially inject high-reward history.
	mc.mu.Lock()
	for i := 0; i < 10; i++ {
		mc.history = append(mc.history, 0.9)
	}
	mc.mu.Unlock()

	origLength := lie.LengthWeight
	mc.CalibrateWeights()
	if lie.LengthWeight >= origLength {
		t.Errorf("LengthWeight should decrease when mean reward > 0.8: before=%.3f after=%.3f", origLength, lie.LengthWeight)
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.0, 0.0},
		{1.0, 1.0},
		{-1.0, 0.0},
		{2.0, 1.0},
		{0.5, 0.5},
	}
	for _, tc := range tests {
		got := clamp01(tc.in)
		if got != tc.want {
			t.Errorf("clamp01(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
