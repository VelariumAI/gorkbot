package selfimprove

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// Mock implementations for testing.

type mockSPARK struct {
	state      *SPARKStateSnapshot
	afterExec  *SPARKStateSnapshot
	calls      int
	switchAfterCall int  // start returning afterExec after this call (0 = disabled)
}

func (m *mockSPARK) GetLastState() *SPARKStateSnapshot {
	m.calls++
	// Return afterExec if calls >= switchAfterCall (and switchAfterCall > 0)
	if m.switchAfterCall > 0 && m.calls >= m.switchAfterCall && m.afterExec != nil {
		return m.afterExec
	}
	return m.state
}

type mockFreeWill struct {
	observations []FreeWillObsInput
	proposals    []FreeWillProposalSummary
}

func (m *mockFreeWill) AddObservation(ctx context.Context, input FreeWillObsInput) error {
	m.observations = append(m.observations, input)
	return nil
}

func (m *mockFreeWill) GetPendingProposals() []FreeWillProposalSummary {
	return m.proposals
}

type mockHarness struct {
	failing int
	total   int
	active  string
}

func (m *mockHarness) FailingCount() int { return m.failing }
func (m *mockHarness) TotalCount() int { return m.total }
func (m *mockHarness) ActiveFeatureID() string { return m.active }

type mockResearch struct {
	buffered int
}

func (m *mockResearch) BufferedCount() int { return m.buffered }

type mockRegistry struct {
	executions []string
	execErr    error
	delay      time.Duration
}

func (m *mockRegistry) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	m.executions = append(m.executions, name)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.execErr != nil {
		return "", m.execErr
	}
	return "success", nil
}

type mockNotify struct {
	messages []string
}

func (m *mockNotify) Notify(msg string) {
	m.messages = append(m.messages, msg)
}

// Tests

func TestMotivatorEWMA(t *testing.T) {
	m := NewMotivator(0.15)

	// Feed increasing quality signals - need sustained high signals to change modes
	// due to EWMA smoothing with alpha=0.15 (slow-moving)

	// Initial low signal
	signals1 := &SignalSnapshot{SPARKDriveScore: 0.3}
	mode1 := m.Update(signals1)
	score1 := m.GetScore()

	if score1 < 0.0 || score1 > 1.0 {
		t.Errorf("score out of range: %f", score1)
	}
	if mode1 != ModeCalm {
		t.Errorf("expected ModeCalm, got %v", mode1)
	}

	// Feed moderately high signals (multiple iterations to overcome EWMA lag)
	for i := 0; i < 5; i++ {
		signals2 := &SignalSnapshot{
			SPARKDriveScore:       0.7,
			SPARKActiveDirectives: 10,
		}
		m.Update(signals2)
	}
	score2 := m.GetScore()

	// Score should increase but smoothed by EWMA
	if score2 <= score1 {
		t.Errorf("expected score to increase: %f -> %f", score1, score2)
	}

	// Feed very high signals (sustained for many iterations to reach Urgent)
	for i := 0; i < 10; i++ {
		signals3 := &SignalSnapshot{
			SPARKDriveScore:       0.95,
			SPARKActiveDirectives: 20,
			HarnessFailing:        8,
			HarnessTotal:          10,
			FreeWillProposalsPending: 5,
		}
		m.Update(signals3)
	}
	score3 := m.GetScore()

	if score3 <= score2 {
		t.Errorf("expected further score increase: %f -> %f", score2, score3)
	}

	// Should now be in higher mode (at least Focused, possibly Urgent)
	mode3 := m.Update(&SignalSnapshot{SPARKDriveScore: 0.95})
	if mode3 == ModeCalm || mode3 == ModeCurious {
		t.Errorf("expected Focused or Urgent mode for high sustained signal, got %v", mode3)
	}
}

func TestHeartbeatAdapt(t *testing.T) {
	hb := NewAdaptiveHeartbeat()
	defer hb.Stop()

	// Initially in Calm mode (8 minutes)
	first := hb.NextTickTime()

	// Adapt to Urgent (30 seconds)
	hb.Adapt(ModeUrgent)
	second := hb.NextTickTime()

	// Second should be much sooner than first would have been
	if second.Sub(first) > 1*time.Minute {
		t.Errorf("heartbeat didn't adapt properly")
	}

	// Adapt back to Calm
	hb.Adapt(ModeCalm)
	third := hb.NextTickTime()

	if third.Sub(second) < 1*time.Minute {
		t.Errorf("heartbeat didn't re-adapt to calm")
	}
}

func TestPlannerSelection(t *testing.T) {
	planner := NewImprovementPlanner()

	// Test with SPARK directives
	directives := []string{"optimize_cache"}
	proposals := []FreeWillProposalSummary{
		{Target: "refactor_parser", Confidence: 75, Risk: 0.3},
	}
	harnessInfo := &HarnessFailureInfo{FailingCount: 2, TotalCount: 10}

	cand := planner.Select(ModeFocused, directives, proposals, harnessInfo, 3)
	if cand == nil {
		t.Errorf("expected candidate, got nil")
	}
	if cand.Source != SourceSPARK && cand.Source != SourceHarness && cand.Source != SourceFreeWill {
		t.Errorf("unexpected source: %v", cand.Source)
	}

	// Test loop guard: same selection should not repeat immediately
	cand2 := planner.Select(ModeFocused, directives, proposals, harnessInfo, 3)
	if cand2 != nil && cand != nil && cand.Target == cand2.Target && cand.Source == cand2.Source {
		t.Errorf("loop guard failed: same action selected twice")
	}
}

func TestPlannerRiskFilter(t *testing.T) {
	planner := NewImprovementPlanner()

	// High-risk proposal should be filtered
	proposals := []FreeWillProposalSummary{
		{Target: "dangerous_change", Confidence: 90, Risk: 0.9},
	}

	cand := planner.Select(ModeCurious, []string{}, proposals, nil, 0)
	if cand != nil && cand.Target == "dangerous_change" {
		t.Errorf("high-risk proposal should have been filtered")
	}
}

func TestPlannerUrgentSkipsResearch(t *testing.T) {
	planner := NewImprovementPlanner()

	// Research should be skipped in Urgent mode
	cand := planner.Select(ModeUrgent, []string{}, []FreeWillProposalSummary{}, nil, 10)
	if cand != nil && cand.Source == SourceResearch {
		t.Errorf("research should be skipped in Urgent mode")
	}
}

func TestDriverLifecycle(t *testing.T) {
	logger := slog.Default()

	mockSpark := &mockSPARK{
		state: &SPARKStateSnapshot{DriveScore: 0.3, ActiveDirectives: 2},
	}
	mockFW := &mockFreeWill{proposals: []FreeWillProposalSummary{}}
	mockHarn := &mockHarness{failing: 1, total: 5, active: "feat1"}
	mockRes := &mockResearch{buffered: 0}
	mockReg := &mockRegistry{}
	mockNot := &mockNotify{}

	driver := NewDriver(mockSpark, mockFW, mockHarn, mockRes, mockReg, mockNot, logger)

	// Initially disabled
	snap := driver.Snapshot()
	if snap.Enabled {
		t.Errorf("driver should start disabled")
	}

	// Start
	ctx := context.Background()
	err := driver.Start(ctx)
	if err != nil {
		t.Errorf("failed to start: %v", err)
	}

	snap = driver.Snapshot()
	if !snap.Enabled {
		t.Errorf("driver should be enabled after Start()")
	}

	// Wait briefly for a cycle
	time.Sleep(100 * time.Millisecond)

	// Stop
	driver.Stop()
	snap = driver.Snapshot()
	if snap.Enabled {
		t.Errorf("driver should be disabled after Stop()")
	}

	// Double-stop is safe
	driver.Stop()
}

func TestDriverToggle(t *testing.T) {
	logger := slog.Default()
	mockSpark := &mockSPARK{
		state: &SPARKStateSnapshot{DriveScore: 0.2, ActiveDirectives: 0},
	}

	driver := NewDriver(mockSpark, &mockFreeWill{}, &mockHarness{}, &mockResearch{},
		&mockRegistry{}, &mockNotify{}, logger)

	ctx := context.Background()

	// Toggle on
	enabled := driver.Toggle(ctx)
	if !enabled {
		t.Errorf("toggle should return true when enabling")
	}

	// Toggle off
	enabled = driver.Toggle(ctx)
	if enabled {
		t.Errorf("toggle should return false when disabling")
	}
}

func TestEmotionalModeString(t *testing.T) {
	tests := []struct {
		mode     EmotionalMode
		expected string
	}{
		{ModeCalm, "CALM"},
		{ModeCurious, "CURIOUS"},
		{ModeFocused, "FOCUSED"},
		{ModeUrgent, "URGENT"},
		{ModeRestrained, "RESTRAINED"},
	}

	for _, tc := range tests {
		if tc.mode.String() != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.mode.String())
		}
	}
}

func TestSignalSourceString(t *testing.T) {
	tests := []struct {
		src      SignalSource
		expected string
	}{
		{SourceSPARK, "SPARK"},
		{SourceFreeWill, "FreeWill"},
		{SourceHarness, "Harness"},
		{SourceResearch, "Research"},
	}

	for _, tc := range tests {
		if tc.src.String() != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.src.String())
		}
	}
}

func BenchmarkMotivatorUpdate(b *testing.B) {
	m := NewMotivator(0.15)
	signals := &SignalSnapshot{
		SPARKDriveScore:         0.5,
		SPARKActiveDirectives:   5,
		HarnessFailing:          2,
		HarnessTotal:            10,
		FreeWillProposalsPending: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Update(signals)
	}
}

func BenchmarkPlannerSelect(b *testing.B) {
	planner := NewImprovementPlanner()
	directives := []string{"optimize"}
	proposals := []FreeWillProposalSummary{{Target: "refactor", Confidence: 80, Risk: 0.2}}
	harnessInfo := &HarnessFailureInfo{FailingCount: 1, TotalCount: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = planner.Select(ModeFocused, directives, proposals, harnessInfo, 2)
	}
}
