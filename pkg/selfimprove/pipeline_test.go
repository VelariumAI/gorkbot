package selfimprove

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/internal/events"
)

// ─── Mock Implementations ────────────────────────────────────────────────────

// mockObs tracks observability calls.
type mockObs struct {
	// Core metrics (Task 4.3A)
	cycleStarts int
	proposals   int
	accepted    int
	rolledBack  int
	failed      int

	// Extended metrics (Task 4.3B)
	executionLatencies []time.Duration
	scoreDeltas        []float64
	rejectReasons      []string
	toolErrors         []string
	rollbackLatencies  []time.Duration
}

// Core metrics
func (m *mockObs) RecordSICycleStart() { m.cycleStarts++ }
func (m *mockObs) RecordSIProposal()   { m.proposals++ }
func (m *mockObs) RecordSIAccepted()   { m.accepted++ }
func (m *mockObs) RecordSIRolledBack() { m.rolledBack++ }
func (m *mockObs) RecordSIFailed()     { m.failed++ }

// Extended metrics (Task 4.3B)
func (m *mockObs) RecordSIExecutionLatency(latency time.Duration) {
	m.executionLatencies = append(m.executionLatencies, latency)
}
func (m *mockObs) RecordSIScoreDelta(delta float64) {
	m.scoreDeltas = append(m.scoreDeltas, delta)
}
func (m *mockObs) RecordSIGateRejectionReason(reason string) {
	m.rejectReasons = append(m.rejectReasons, reason)
}
func (m *mockObs) RecordSIToolError(toolName string) {
	m.toolErrors = append(m.toolErrors, toolName)
}
func (m *mockObs) RecordSIRollbackLatency(latency time.Duration) {
	m.rollbackLatencies = append(m.rollbackLatencies, latency)
}

// mockRollback tracks rollback store calls.
type mockRollback struct {
	snapshots []string
	discards  []string
	rollbacks []string
}

func (m *mockRollback) Snapshot(id string, _ []string) error {
	m.snapshots = append(m.snapshots, id)
	return nil
}

func (m *mockRollback) Discard(id string) error {
	m.discards = append(m.discards, id)
	return nil
}

func (m *mockRollback) Rollback(ctx context.Context, id string) error {
	m.rollbacks = append(m.rollbacks, id)
	return nil
}

// ─── Tests ──────────────────────────────────────────────────────────────────

// TestPipeline_FullSuccess verifies all 7 stages complete; Discard called; Accepted=true.
func TestPipeline_FullSuccess(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 2,
			IDLDebt:          1,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "improve_latency", Confidence: 75, Risk: 0.2},
		},
	}

	harness := &mockHarness{failing: 0, total: 5}
	research := &mockResearch{buffered: 2}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !accepted {
		t.Errorf("expected accepted=true, got false")
	}

	if obs.cycleStarts != 1 {
		t.Errorf("expected cycleStarts=1, got %d", obs.cycleStarts)
	}

	if obs.proposals != 1 {
		t.Errorf("expected proposals=1, got %d", obs.proposals)
	}

	if obs.accepted != 1 {
		t.Errorf("expected accepted=1, got %d", obs.accepted)
	}

	if len(rollback.discards) != 1 {
		t.Errorf("expected 1 discard call, got %d", len(rollback.discards))
	}

	if obs.rolledBack != 0 {
		t.Errorf("expected rolledBack=0, got %d", obs.rolledBack)
	}
}

// TestPipeline_NoSPARKState verifies nil SPARK → abort at Detect.
func TestPipeline_NoSPARKState(t *testing.T) {
	spark := &mockSPARK{state: nil}
	fw := &mockFreeWill{}
	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run should not return error on graceful abort, got: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false, got true")
	}

	if obs.cycleStarts != 0 {
		t.Errorf("expected cycleStarts=0 on abort, got %d", obs.cycleStarts)
	}

	if len(rollback.snapshots) != 0 {
		t.Errorf("expected no snapshots on early abort, got %d", len(rollback.snapshots))
	}
}

// TestPipeline_NoCandidate verifies planner nil → abort at Hypothesise.
func TestPipeline_NoCandidate(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.5,
			ActiveDirectives: 0,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{proposals: []FreeWillProposalSummary{}} // empty proposals
	harness := &mockHarness{failing: 0, total: 5}
	research := &mockResearch{buffered: 0}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run should not return error on graceful abort, got: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false, got true")
	}

	if obs.proposals != 0 {
		t.Errorf("expected proposals=0 on no candidate, got %d", obs.proposals)
	}
}

// TestPipeline_ToolError verifies Registry error → Gate rejects → rollback called.
func TestPipeline_ToolError(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "fix_bug", Confidence: 90, Risk: 0.1},
		},
	}

	harness := &mockHarness{failing: 1, total: 10}
	research := &mockResearch{buffered: 0}
	registry := &mockRegistry{execErr: errors.New("tool execution failed")}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false due to tool error, got true")
	}

	if len(rollback.rollbacks) != 1 {
		t.Errorf("expected 1 rollback call, got %d", len(rollback.rollbacks))
	}

	if obs.rolledBack != 1 {
		t.Errorf("expected rolledBack=1, got %d", obs.rolledBack)
	}
}

// TestPipeline_ScoreDropTriggersRollback verifies score drop > threshold → rollback.
func TestPipeline_ScoreDropTriggersRollback(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.7,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
		switchAfterCall: 4, // return afterExec starting at Score stage (4th call)
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "risky_change", Confidence: 70, Risk: 0.7},
		},
	}

	harness := &mockHarness{failing: 0, total: 5}
	research := &mockResearch{buffered: 0}

	// Simulate score drop: initial 0.7 → final 0.6 (delta = -0.1, exceeds threshold 0.05)
	registry := &mockRegistry{}
	spark.afterExec = &SPARKStateSnapshot{
		DriveScore:       0.6, // dropped by 0.1
		ActiveDirectives: 1,
		IDLDebt:          0,
	}

	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false due to score drop, got true")
	}

	if len(rollback.rollbacks) != 1 {
		t.Errorf("expected 1 rollback call on score drop, got %d", len(rollback.rollbacks))
	}
}

// TestPipeline_BusEventsEmitted verifies ProposalEvent and RollbackEvent published.
func TestPipeline_BusEventsEmitted(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
		switchAfterCall: 4, // return afterExec starting at Score stage
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "test_change", Confidence: 80, Risk: 0.4},
		},
	}

	harness := &mockHarness{failing: 0, total: 5}
	research := &mockResearch{buffered: 0}

	// Simulate rejection by score drop
	registry := &mockRegistry{}
	spark.afterExec = &SPARKStateSnapshot{
		DriveScore:       0.6, // score drops
		ActiveDirectives: 1,
		IDLDebt:          0,
	}

	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	proposalEventSeen := false
	rollbackEventSeen := false

	bus.Register("ImprovementProposalEvent", func(ctx context.Context, evt events.BusEvent) events.BusEvent {
		proposalEventSeen = true
		return nil
	})

	bus.Register("RollbackEvent", func(ctx context.Context, evt events.BusEvent) events.BusEvent {
		rollbackEventSeen = true
		return nil
	})

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	_, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !proposalEventSeen {
		t.Errorf("expected ProposalEvent to be published")
	}

	if !rollbackEventSeen {
		t.Errorf("expected RollbackEvent to be published")
	}
}

// TestPipeline_ContextCancellation verifies ctx cancelled → Run returns error.
func TestPipeline_ContextCancellation(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "test", Confidence: 70, Risk: 0.2},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}

	// Registry that waits (to test cancellation during sandbox)
	registry := &mockRegistry{delay: 100 * time.Millisecond}

	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	accepted, err := pipeline.Run(ctx)

	if err == nil {
		t.Errorf("expected error on cancelled context, got nil")
	}

	if accepted {
		t.Errorf("expected accepted=false on cancellation, got true")
	}
}

// TestPipeline_DriverDelegatesToPipeline verifies Driver with pipeline wired.
func TestPipeline_DriverDelegatesToPipeline(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "improve", Confidence: 80, Risk: 0.2},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	// Create driver
	driver := NewDriver(spark, fw, harness, research, registry, notify, nil)

	// Create and wire pipeline
	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)
	driver.SetPipeline(pipeline)

	// Execute cycle
	ctx := context.Background()
	driver.executeCycle(ctx)

	// Verify cycle count incremented (acceptance recorded)
	snap := driver.Snapshot()
	if snap.CycleCount == 0 {
		t.Errorf("expected cycleCount > 0 after successful pipeline execution, got %d", snap.CycleCount)
	}
}

// Extended test cases for Task 4.3B (Extended Metrics)

// TestPipeline_ExecutionLatencyTracking verifies execution latency is recorded.
func TestPipeline_ExecutionLatencyTracking(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "test_latency", Confidence: 75, Risk: 0.2},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{delay: 50 * time.Millisecond}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !accepted {
		t.Errorf("expected accepted=true, got false")
	}

	if len(obs.executionLatencies) != 1 {
		t.Errorf("expected 1 execution latency recorded, got %d", len(obs.executionLatencies))
	}

	if obs.executionLatencies[0] < 50*time.Millisecond {
		t.Errorf("expected latency >= 50ms, got %v", obs.executionLatencies[0])
	}
}

// TestPipeline_ScoreDeltaTracking verifies score delta is recorded.
func TestPipeline_ScoreDeltaTracking(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.7,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
		switchAfterCall: 4,
		afterExec: &SPARKStateSnapshot{
			DriveScore:       0.8, // positive delta
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "improve_score", Confidence: 75, Risk: 0.2},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	_, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(obs.scoreDeltas) != 1 {
		t.Errorf("expected 1 score delta recorded, got %d", len(obs.scoreDeltas))
	}

	expectedDelta := 0.1 // 0.8 - 0.7
	epsilon := 0.00001
	if math.Abs(obs.scoreDeltas[0]-expectedDelta) > epsilon {
		t.Errorf("expected score delta %.4f, got %.4f (diff: %.6f)", expectedDelta, obs.scoreDeltas[0], math.Abs(obs.scoreDeltas[0]-expectedDelta))
	}
}

// TestPipeline_GateRejectionReasonTracking verifies rejection reason is recorded.
func TestPipeline_GateRejectionReasonTracking(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
		switchAfterCall: 4,
		afterExec: &SPARKStateSnapshot{
			DriveScore:       0.6, // score drop exceeds threshold
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "risky_change", Confidence: 70, Risk: 0.5},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false due to score drop, got true")
	}

	if len(obs.rejectReasons) != 1 {
		t.Errorf("expected 1 reject reason recorded, got %d", len(obs.rejectReasons))
	}

	if !strings.Contains(obs.rejectReasons[0], "score_delta") {
		t.Errorf("expected reject reason to contain 'score_delta', got %s", obs.rejectReasons[0])
	}
}

// TestPipeline_ToolErrorTracking verifies tool errors are recorded.
func TestPipeline_ToolErrorTracking(t *testing.T) {
	spark := &mockSPARK{
		state: &SPARKStateSnapshot{
			DriveScore:       0.8,
			ActiveDirectives: 1,
			IDLDebt:          0,
		},
	}

	fw := &mockFreeWill{
		proposals: []FreeWillProposalSummary{
			{Target: "failing_tool", Confidence: 75, Risk: 0.2},
		},
	}

	harness := &mockHarness{}
	research := &mockResearch{}
	registry := &mockRegistry{execErr: errors.New("tool failed")}
	notify := &mockNotify{}
	obs := &mockObs{}
	rollback := &mockRollback{}
	bus := events.NewBus()

	pipeline := NewSIPipeline(spark, fw, harness, research, registry, notify, obs, bus, rollback, nil)

	ctx := context.Background()
	accepted, err := pipeline.Run(ctx)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if accepted {
		t.Errorf("expected accepted=false due to tool error, got true")
	}

	if len(obs.toolErrors) != 1 {
		t.Errorf("expected 1 tool error recorded, got %d", len(obs.toolErrors))
	}
}

// Note: Mock structs are defined in selfimprove_test.go and shared across tests.
// These include mockSPARK, mockFW (mockFreeWill), mockHarness, mockResearch, mockRegistry, and mockNotify.
// Enhanced with additional fields (afterExec, execErr, delay) to support pipeline testing.
