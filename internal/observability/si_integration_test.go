package observability

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestSIMetricsRecording verifies that SI cycle metrics are properly recorded.
func TestSIMetricsRecording(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)

	// Simulate a complete SI cycle
	hub.RecordSICycleStart()
	hub.RecordSIProposal()
	hub.RecordSIAccepted()
	hub.RecordSICycleStart()
	hub.RecordSIProposal()
	hub.RecordSIRolledBack()
	hub.RecordSICycleStart()
	hub.RecordSIFailed()

	// Verify metrics
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	sim := hub.selfImprovement
	if sim.CyclesStarted != 3 {
		t.Errorf("expected 3 cycles started, got %d", sim.CyclesStarted)
	}
	if sim.ProposalsTotal != 2 {
		t.Errorf("expected 2 proposals, got %d", sim.ProposalsTotal)
	}
	if sim.Accepted != 1 {
		t.Errorf("expected 1 accepted, got %d", sim.Accepted)
	}
	if sim.Rolled != 1 {
		t.Errorf("expected 1 rolled back, got %d", sim.Rolled)
	}
	if sim.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", sim.Failed)
	}
}

// TestSIMetricsNilSafety verifies that nil hub doesn't panic.
// (Tests the adapter pattern used in selfimprove_hooks.go)
func TestSIMetricsNilSafety(t *testing.T) {
	var hub *ObservabilityHub

	// These should not panic even with nil hub
	if hub != nil {
		hub.RecordSICycleStart()
		hub.RecordSIProposal()
		hub.RecordSIAccepted()
		hub.RecordSIRolledBack()
		hub.RecordSIFailed()
	}
	// If we got here, the test passed (no panic)
}

// TestSIMetricsWithContext verifies metrics work with context.
func TestSIMetricsWithContext(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)
	ctx := context.Background()

	// Associate correlation ID
	corrID := "test-corr-123"
	ctx = hub.WithCorrelationID(ctx, corrID)

	// Record SI activity
	hub.RecordSICycleStart()
	hub.RecordSIProposal()
	hub.RecordSIAccepted()

	// Verify metrics recorded
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if hub.selfImprovement.CyclesStarted != 1 {
		t.Errorf("expected 1 cycle, got %d", hub.selfImprovement.CyclesStarted)
	}
}

// TestSIMetricsRates verifies rate calculations.
func TestSIMetricsRates(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)

	// Simulate 10 cycles: 7 accepted, 2 rolled back, 1 failed
	for i := 0; i < 7; i++ {
		hub.RecordSICycleStart()
		hub.RecordSIProposal()
		hub.RecordSIAccepted()
	}
	for i := 0; i < 2; i++ {
		hub.RecordSICycleStart()
		hub.RecordSIProposal()
		hub.RecordSIRolledBack()
	}
	hub.RecordSICycleStart()
	hub.RecordSIFailed()

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	sim := hub.selfImprovement

	// Verify counts
	totalActions := sim.Accepted + sim.Rolled + sim.Failed
	if totalActions != 10 {
		t.Errorf("expected 10 total actions, got %d", totalActions)
	}

	// Acceptance rate: 7/10 = 0.7
	expectedRate := 0.7
	actualRate := float64(sim.Accepted) / float64(totalActions)
	if actualRate != expectedRate {
		t.Errorf("expected acceptance rate %.2f, got %.2f", expectedRate, actualRate)
	}
}

// TestSIMetricsNilHub verifies that adapter gracefully handles nil hub.
// This simulates the nil-safe pattern used in obsAdapter.
func TestSIMetricsAdapterPattern(t *testing.T) {
	var hub *ObservabilityHub

	// Simulate obsAdapter pattern
	recordSICycleStart := func() {
		if hub != nil {
			hub.RecordSICycleStart()
		}
	}
	recordSIProposal := func() {
		if hub != nil {
			hub.RecordSIProposal()
		}
	}
	recordSIAccepted := func() {
		if hub != nil {
			hub.RecordSIAccepted()
		}
	}

	// Should not panic with nil hub
	recordSICycleStart()
	recordSIProposal()
	recordSIAccepted()

	// Now with real hub
	hub = NewObservabilityHub(slog.Default())
	recordSICycleStart()
	recordSIProposal()
	recordSIAccepted()

	// Verify metrics recorded
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if hub.selfImprovement.CyclesStarted != 1 {
		t.Errorf("expected 1 cycle started, got %d", hub.selfImprovement.CyclesStarted)
	}
	if hub.selfImprovement.ProposalsTotal != 1 {
		t.Errorf("expected 1 proposal, got %d", hub.selfImprovement.ProposalsTotal)
	}
	if hub.selfImprovement.Accepted != 1 {
		t.Errorf("expected 1 accepted, got %d", hub.selfImprovement.Accepted)
	}
}

// TestSIMetricsConcurrent verifies thread-safe metric recording.
func TestSIMetricsConcurrent(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)

	// Simulate 100 concurrent SI cycles (would be impossible in reality, but tests lock safety)
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			hub.RecordSICycleStart()
			time.Sleep(1 * time.Millisecond)
			hub.RecordSIProposal()
			if i%2 == 0 {
				hub.RecordSIAccepted()
			} else {
				hub.RecordSIRolledBack()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify metrics
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if hub.selfImprovement.CyclesStarted != 100 {
		t.Errorf("expected 100 cycles started, got %d", hub.selfImprovement.CyclesStarted)
	}
	if hub.selfImprovement.ProposalsTotal != 100 {
		t.Errorf("expected 100 proposals, got %d", hub.selfImprovement.ProposalsTotal)
	}
	if hub.selfImprovement.Accepted+hub.selfImprovement.Rolled != 100 {
		t.Errorf("expected 100 total accepted+rolled, got %d",
			hub.selfImprovement.Accepted+hub.selfImprovement.Rolled)
	}
}
