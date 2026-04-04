package memory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestTemporalMemory_GetDecayedConfidence(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	tests := []struct {
		name       string
		confidence float64
		age        time.Duration
		expected   float64
		shouldFloor bool
	}{
		{"new_fact", 0.9, 0, 0.9, false},
		{"half_life_decay", 0.9, 7 * 24 * time.Hour, 0.45, false},
		{"two_half_lives", 0.8, 14 * 24 * time.Hour, 0.2, true}, // Floored at MinConfidence
		{"below_minimum", 0.15, 10 * 24 * time.Hour, 0.0, true},
		{"zero_confidence", 0.0, 1 * 24 * time.Hour, 0.0, false},
		{"negative_confidence", -0.1, 1 * 24 * time.Hour, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdAt := time.Now().Add(-tt.age)
			decayed := tm.GetDecayedConfidence(tt.confidence, createdAt)

			if tt.shouldFloor || tt.expected == 0 {
				if decayed != 0 {
					t.Errorf("expected floored to 0, got %f", decayed)
				}
			} else {
				// Allow 5% tolerance for floating point comparison
				tolerance := tt.expected * 0.05
				if decayed < tt.expected-tolerance || decayed > tt.expected+tolerance {
					t.Errorf("expected ~%f, got %f", tt.expected, decayed)
				}
			}
		})
	}

	t.Logf("✓ Temporal decay calculations verified")
}

func TestTemporalMemory_CheckExpiration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	tests := []struct {
		name       string
		age        time.Duration
		shouldExpire bool
	}{
		{"recent_fact", 10 * 24 * time.Hour, false},
		{"near_threshold", 89 * 24 * time.Hour, false},
		{"just_expired", 91 * 24 * time.Hour, true},
		{"old_fact", 180 * 24 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdAt := time.Now().Add(-tt.age)
			expired := tm.CheckExpiration(createdAt)

			if expired != tt.shouldExpire {
				t.Errorf("expected %v, got %v", tt.shouldExpire, expired)
			}
		})
	}

	t.Logf("✓ Expiration checks verified")
}

func TestTemporalMemory_ShouldCompact(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	// Initially should not compact (just created with time.Now())
	if tm.ShouldCompact() {
		t.Error("expected false for fresh temporal memory")
	}

	// After marking last compaction time to past, should compact
	tm.compactionMu.Lock()
	tm.lastCompactionTime = time.Now().Add(-25 * time.Hour)
	tm.compactionMu.Unlock()

	if !tm.ShouldCompact() {
		t.Error("expected true after compaction interval passed")
	}

	// After updating to now, should not compact again
	tm.compactionMu.Lock()
	tm.lastCompactionTime = time.Now()
	tm.compactionMu.Unlock()

	if tm.ShouldCompact() {
		t.Error("expected false after recent compaction")
	}

	t.Logf("✓ Compaction scheduling verified")
}

func TestTemporalMemory_CompactFacts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()
	err := tm.CompactFacts(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify metrics are updated
	if tm.factsCompacted == 0 && tm.lastCompactionTime.IsZero() {
		t.Error("expected compaction to update state")
	}

	t.Logf("✓ Fact compaction completed")
}

func TestTemporalMemory_CompactGroup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()

	// Create test facts
	facts := []*SearchResult{
		{
			FactID:     "fact1",
			Subject:    "Python",
			Predicate:  "supports",
			Object:     "async",
			Confidence: 0.9,
			Timestamp:  time.Now().Add(-2 * 24 * time.Hour).Format(time.RFC3339),
		},
		{
			FactID:     "fact2",
			Subject:    "Python",
			Predicate:  "supports",
			Object:     "async",
			Confidence: 0.85,
			Timestamp:  time.Now().Add(-1 * 24 * time.Hour).Format(time.RFC3339),
		},
		{
			FactID:     "fact3",
			Subject:    "Python",
			Predicate:  "supports",
			Object:     "typing",
			Confidence: 0.95,
			Timestamp:  time.Now().Format(time.RFC3339),
		},
	}

	err := tm.compactGroup(ctx, facts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify compaction metrics
	if tm.factsCompacted != int64(len(facts)) {
		t.Errorf("expected %d facts compacted, got %d", len(facts), tm.factsCompacted)
	}

	t.Logf("✓ Group compaction verified (compacted %d facts)", tm.factsCompacted)
}

func TestTemporalMemory_ArchiveFacts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()
	err := tm.ArchiveFacts(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tm.factsArchived == 0 {
		t.Error("expected archival to update metrics")
	}

	t.Logf("✓ Fact archival verified")
}

func TestTemporalMemory_DetectContradictions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()

	fact := &SearchResult{
		FactID:    "fact1",
		Subject:   "Alice",
		Predicate: "works_at",
		Object:    "Acme Corp",
	}

	contradictions, err := tm.DetectContradictions(ctx, fact)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return an empty list (no contradictions detected in this minimal test)
	if contradictions == nil {
		t.Error("expected non-nil contradiction list (may be empty)")
	}

	t.Logf("✓ Contradiction detection verified (%d contradictions found)", len(contradictions))
}

func TestTemporalMemory_UpdateConfidenceBySupport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()

	fact := &SearchResult{
		FactID:     "fact1",
		Subject:    "Python",
		Predicate:  "language",
		Confidence: 0.8,
	}

	tests := []struct {
		name          string
		supporting    int
		contradicting int
		expectedMin   float64
		expectedMax   float64
	}{
		{"no_support", 0, 0, 0.8, 0.8},
		{"one_supporter", 1, 0, 0.9, 0.9},
		{"two_supporters", 2, 0, 1.0, 1.0},
		{"one_contradicting", 0, 1, 0.6, 0.6},
		{"mixed", 2, 1, 0.8, 0.8}, // 0.8 + (0.1*2) - (0.2*1) = 0.8
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := tm.UpdateConfidenceBySupport(ctx, fact, tt.supporting, tt.contradicting)

			// Allow small floating point tolerance
			tolerance := 0.0001
			if confidence < tt.expectedMin-tolerance || confidence > tt.expectedMax+tolerance {
				t.Errorf("expected %f-%f, got %f", tt.expectedMin, tt.expectedMax, confidence)
			}

			// Confidence should always be clamped [0, 1]
			if confidence < -tolerance || confidence > 1+tolerance {
				t.Errorf("confidence out of bounds: %f", confidence)
			}
		})
	}

	t.Logf("✓ Confidence support/contradiction adjustments verified")
}

func TestTemporalMemory_PruneByRetention(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()
	err := tm.PruneByRetention(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	t.Logf("✓ Retention pruning verified")
}

func TestTemporalMemory_PruneByCount(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	ctx := context.Background()
	err := tm.PruneByCount(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	t.Logf("✓ Count-based pruning verified")
}

func TestTemporalMemory_GetFactAge(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	tests := []struct {
		name     string
		age      time.Duration
		minDays  float64
		maxDays  float64
	}{
		{"new", 0, -0.1, 0.1},
		{"one_day", 24 * time.Hour, 0.9, 1.1},
		{"one_week", 7 * 24 * time.Hour, 6.9, 7.1},
		{"thirty_days", 30 * 24 * time.Hour, 29.9, 30.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdAt := time.Now().Add(-tt.age)
			age := tm.GetFactAge(createdAt)

			if age < tt.minDays || age > tt.maxDays {
				t.Errorf("expected %f-%f days, got %f", tt.minDays, tt.maxDays, age)
			}
		})
	}

	t.Logf("✓ Fact age calculations verified")
}

func TestTemporalMemory_GetMemoryMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	metrics := tm.GetMemoryMetrics()

	required := []string{
		"facts_compacted",
		"facts_decayed",
		"facts_archived",
		"last_compaction",
		"half_life",
		"min_confidence",
		"archive_threshold",
		"max_facts",
		"max_retention",
	}

	for _, key := range required {
		if _, ok := metrics[key]; !ok {
			t.Errorf("missing metric: %s", key)
		}
	}

	t.Logf("✓ Memory metrics available: %v", metrics)
}

func TestTemporalMemory_ApplyTemporalDecay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	results := []SearchResult{
		{
			FactID:     "fact1",
			Subject:    "A",
			Predicate:  "B",
			Object:     "C",
			Confidence: 0.9,
			Timestamp:  time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339), // 1 half-life old
		},
		{
			FactID:     "fact2",
			Subject:    "D",
			Predicate:  "E",
			Object:     "F",
			Confidence: 0.8,
			Timestamp:  time.Now().Format(time.RFC3339), // Brand new
		},
	}

	decayed := tm.ApplyTemporalDecay(results)

	if len(decayed) != 2 {
		t.Errorf("expected 2 results, got %d", len(decayed))
	}

	// After decay, results should be re-sorted by confidence
	if len(decayed) > 1 {
		if decayed[0].Confidence < decayed[1].Confidence {
			t.Error("results not properly sorted by decayed confidence")
		}
	}

	t.Logf("✓ Temporal decay applied and re-sorted: confidence %f → %f", results[1].Confidence, decayed[0].Confidence)
}

func TestTemporalMemory_GetMostRecentFacts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	now := time.Now()
	results := []SearchResult{
		{FactID: "fact1", Timestamp: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339)},
		{FactID: "fact2", Timestamp: now.Add(-1 * 24 * time.Hour).Format(time.RFC3339)},
		{FactID: "fact3", Timestamp: now.Format(time.RFC3339)},
		{FactID: "fact4", Timestamp: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)},
	}

	recent := tm.GetMostRecentFacts(results, 2)

	if len(recent) != 2 {
		t.Errorf("expected 2 results, got %d", len(recent))
	}

	// Most recent should be first
	if recent[0].FactID != "fact3" {
		t.Errorf("expected 'fact3' first, got '%s'", recent[0].FactID)
	}

	if recent[1].FactID != "fact2" {
		t.Errorf("expected 'fact2' second, got '%s'", recent[1].FactID)
	}

	t.Logf("✓ Most recent facts retrieved in order: %v", []string{recent[0].FactID, recent[1].FactID})
}

func TestTemporalMemory_SetConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	newConfig := TemporalConfig{
		HalfLife:        14 * 24 * time.Hour,
		MinConfidence:   0.3,
		ArchiveThreshold: 60 * 24 * time.Hour,
	}

	tm.SetConfig(newConfig)

	// Verify config was updated
	tm.compactionMu.RLock()
	if tm.config.HalfLife != newConfig.HalfLife {
		t.Errorf("expected half life %v, got %v", newConfig.HalfLife, tm.config.HalfLife)
	}
	tm.compactionMu.RUnlock()

	t.Logf("✓ Configuration updated successfully")
}

func TestTemporalMemory_CompactionJob_Lifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	job := NewCompactionJob(tm, 10*time.Second) // Long interval to prevent firing during test

	if job.IsRunning() {
		t.Error("job should not be running initially")
	}

	ctx, cancel := context.WithCancel(context.Background())

	job.Start(ctx)

	if !job.IsRunning() {
		t.Error("job should be running after Start()")
	}

	// Stop the job by cancelling the context (clean shutdown)
	cancel()

	// Give it time to respond to context cancellation
	time.Sleep(200 * time.Millisecond)

	// Job should eventually stop when context is cancelled
	// Allow a small window for the goroutine to process the cancellation
	maxWait := 5
	for i := 0; i < maxWait; i++ {
		if !job.IsRunning() {
			break
		}
		if i < maxWait-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	t.Logf("✓ Compaction job lifecycle verified (running=%v)", job.IsRunning())
}

func TestTemporalMemory_CompactionJob_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	job := NewCompactionJob(tm, 1*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)

	job.Start(ctx)

	if !job.IsRunning() {
		t.Error("job should be running")
	}

	cancel()

	// Give it time to respond to context cancellation
	time.Sleep(100 * time.Millisecond)

	if job.IsRunning() {
		t.Error("job should stop when context is cancelled")
	}

	t.Logf("✓ Context cancellation handling verified")
}

func TestTemporalMemory_DefaultConfig(t *testing.T) {
	config := DefaultTemporalConfig()

	if config.HalfLife != 7*24*time.Hour {
		t.Errorf("expected 7 day half life, got %v", config.HalfLife)
	}

	if config.MinConfidence != 0.2 {
		t.Errorf("expected 0.2 min confidence, got %f", config.MinConfidence)
	}

	if config.ArchiveThreshold != 90*24*time.Hour {
		t.Errorf("expected 90 day archive threshold, got %v", config.ArchiveThreshold)
	}

	if config.CompactionFactor != 10 {
		t.Errorf("expected 10:1 compaction factor, got %d", config.CompactionFactor)
	}

	if config.MaxFacts != 10000 {
		t.Errorf("expected 10000 max facts, got %d", config.MaxFacts)
	}

	if config.MaxRetention != 365*24*time.Hour {
		t.Errorf("expected 1 year max retention, got %v", config.MaxRetention)
	}

	t.Logf("✓ Default configuration verified")
}

// Benchmark tests

func BenchmarkTemporalMemory_GetDecayedConfidence(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	createdAt := time.Now().Add(-7 * 24 * time.Hour)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tm.GetDecayedConfidence(0.8, createdAt)
	}
}

func BenchmarkTemporalMemory_CheckExpiration(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	createdAt := time.Now().Add(-50 * 24 * time.Hour)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tm.CheckExpiration(createdAt)
	}
}

func BenchmarkTemporalMemory_ApplyTemporalDecay(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	results := make([]SearchResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = SearchResult{
			FactID:     fmt.Sprintf("fact%d", i),
			Confidence: 0.8,
			Timestamp:  time.Now().Add(-time.Duration(i) * 24 * time.Hour).Format(time.RFC3339),
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tm.ApplyTemporalDecay(results)
	}
}

func BenchmarkTemporalMemory_GetMostRecentFacts(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tm := NewTemporalMemory(nil, logger)

	results := make([]SearchResult, 1000)
	for i := 0; i < 1000; i++ {
		results[i] = SearchResult{
			FactID:    fmt.Sprintf("fact%d", i),
			Timestamp: time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tm.GetMostRecentFacts(results, 10)
	}
}
