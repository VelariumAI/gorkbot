package observability

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestObservabilityHub_ProviderLatencyMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record latencies
	hub.RecordProviderLatency("xai", "grok-3", 100*time.Millisecond)
	hub.RecordProviderLatency("xai", "grok-3", 150*time.Millisecond)
	hub.RecordProviderLatency("xai", "grok-3", 120*time.Millisecond)

	// Verify metrics
	metrics := hub.GetProviderLatencyMetrics("xai", "grok-3")
	if metrics == nil {
		t.Fatal("expected metrics for xai/grok-3")
	}
	if metrics.P50() == 0 {
		t.Error("expected non-zero P50")
	}
	if metrics.Mean() == 0 {
		t.Error("expected non-zero mean")
	}
}

func TestObservabilityHub_ToolExecutionMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record tool executions
	hub.RecordToolExecution("bash", true, 100*time.Millisecond)
	hub.RecordToolExecution("bash", true, 110*time.Millisecond)
	hub.RecordToolExecution("bash", false, 50*time.Millisecond)

	// Verify metrics
	metrics := hub.GetToolExecutionMetrics("bash")
	if metrics == nil {
		t.Fatal("expected metrics for bash")
	}
	if metrics.Invocations != 3 {
		t.Errorf("expected 3 invocations, got %d", metrics.Invocations)
	}
	if metrics.Successes != 2 {
		t.Errorf("expected 2 successes, got %d", metrics.Successes)
	}
	if metrics.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", metrics.Failures)
	}
	if sr := metrics.SuccessRate(); sr <= 0 || sr > 1 {
		t.Errorf("expected valid success rate, got %.2f", sr)
	}
}

func TestObservabilityHub_FailureMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record failures
	hub.RecordFailure("tool_error", "bash failed")
	hub.RecordFailure("tool_error", "bash timeout")
	hub.RecordFailure("provider_timeout", "xai timeout")

	// Verify metrics
	toolErr := hub.GetFailureClassMetrics("tool_error")
	if toolErr == nil || toolErr.Count != 2 {
		t.Errorf("expected 2 tool_error, got %v", toolErr)
	}

	provErr := hub.GetFailureClassMetrics("provider_timeout")
	if provErr == nil || provErr.Count != 1 {
		t.Errorf("expected 1 provider_timeout, got %v", provErr)
	}
}

func TestObservabilityHub_ApprovalOutcomes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record approval outcomes
	hub.RecordApprovalOutcome("low", "approved")
	hub.RecordApprovalOutcome("low", "approved")
	hub.RecordApprovalOutcome("high", "denied")
	hub.RecordApprovalOutcome("high", "approved")

	// Verify metrics
	low := hub.GetApprovalOutcomeMetrics("low")
	if low == nil || low.Approved != 2 {
		t.Errorf("expected 2 approved low-risk, got %v", low)
	}

	high := hub.GetApprovalOutcomeMetrics("high")
	if high == nil || high.Denied != 1 || high.Approved != 1 {
		t.Errorf("expected 1 denied + 1 approved high-risk, got %v", high)
	}
}

func TestObservabilityHub_CostMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record costs
	hub.RecordCost("xai", "grok-3", 1000, 500, 0.015)
	hub.RecordCost("xai", "grok-3", 1200, 600, 0.018)

	// Verify metrics
	metrics := hub.GetCostMetrics("xai", "grok-3")
	if metrics == nil {
		t.Fatal("expected cost metrics for xai/grok-3")
	}
	if metrics.TotalCost != 0.033 {
		t.Errorf("expected total cost 0.033, got %f", metrics.TotalCost)
	}
	if metrics.InputTokens != 2200 {
		t.Errorf("expected 2200 input tokens, got %d", metrics.InputTokens)
	}
	if metrics.TurnCount != 2 {
		t.Errorf("expected 2 turns, got %d", metrics.TurnCount)
	}
}

func TestObservabilityHub_MemoryQualityMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record memory quality
	hub.RecordMemoryQuality(0.95, 0.75, 0.88)
	hub.RecordMemoryQuality(0.92, 0.80, 0.85)

	// Verify metrics - should have averages
	metrics := hub.GetMemoryQualityMetrics()
	if metrics == nil || metrics.AverageCacheHit == 0 {
		t.Errorf("expected memory quality metrics, got %v", metrics)
	}
}

func TestObservabilityHub_SelfImprovementMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record SI events
	hub.RecordSICycleStart()
	hub.RecordSICycleStart()
	hub.RecordSIProposal()
	hub.RecordSIProposal()
	hub.RecordSIProposal()
	hub.RecordSIAccepted()
	hub.RecordSIRolledBack()
	hub.RecordSIFailed()

	// Verify metrics
	metrics := hub.GetSelfImprovementMetrics()
	if metrics == nil {
		t.Fatal("expected SI metrics")
	}
	if metrics.CyclesStarted != 2 {
		t.Errorf("expected 2 cycles, got %d", metrics.CyclesStarted)
	}
	if metrics.ProposalsTotal != 3 {
		t.Errorf("expected 3 proposals, got %d", metrics.ProposalsTotal)
	}
	if metrics.Accepted != 1 || metrics.Rolled != 1 || metrics.Failed != 1 {
		t.Errorf("unexpected SI outcomes")
	}
}

func TestObservabilityHub_PrometheusExport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record some data
	hub.RecordProviderLatency("xai", "grok-3", 100*time.Millisecond)
	hub.RecordToolExecution("bash", true, 50*time.Millisecond)
	hub.RecordFailure("tool_error", "test error")
	hub.RecordCost("xai", "grok-3", 1000, 500, 0.015)

	// Export metrics
	output := hub.ExportMetrics()

	// Verify Prometheus format
	if len(output) == 0 {
		t.Fatal("expected non-empty metrics output")
	}
	if !contains(output, "# HELP gorkbot_provider_latency_ms") {
		t.Error("missing provider latency HELP comment")
	}
	if !contains(output, "# HELP gorkbot_tool_invocations_total") {
		t.Error("missing tool invocations HELP comment")
	}
	if !contains(output, "# HELP gorkbot_failures_total") {
		t.Error("missing failures HELP comment")
	}
	if !contains(output, "# HELP gorkbot_cost_usd") {
		t.Error("missing cost HELP comment")
	}
	if !contains(output, "gorkbot_cost_total_usd") {
		t.Error("missing total cost metric")
	}
}

func TestObservabilityHub_CorrelationID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Test correlation ID support - use background context
	ctx := context.Background()
	ctxWithID := hub.WithCorrelationID(ctx, "request-123")
	if ctxWithID == nil {
		t.Error("expected non-nil context with correlation ID")
	}

	// Verify ID is stored
	id := hub.GetCorrelationID("request-123")
	if id != "request-123" {
		t.Errorf("expected correlation ID request-123, got %s", id)
	}
}

func TestProviderLatencyMetrics_Percentiles(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record varied latencies
	for i := 0; i < 100; i++ {
		hub.RecordProviderLatency("xai", "grok-3", time.Duration(i+1)*time.Millisecond)
	}

	metrics := hub.GetProviderLatencyMetrics("xai", "grok-3")
	if metrics == nil {
		t.Fatal("expected metrics")
	}

	p50 := metrics.P50()
	p95 := metrics.P95()
	p99 := metrics.P99()
	mean := metrics.Mean()

	if p50 == 0 {
		t.Error("P50 should be non-zero")
	}
	if p95 <= p50 {
		t.Errorf("P95 (%v) should be >= P50 (%v)", p95, p50)
	}
	if p99 <= p95 {
		t.Errorf("P99 (%v) should be >= P95 (%v)", p99, p95)
	}
	if mean == 0 {
		t.Error("mean should be non-zero")
	}
}

func TestToolExecutionMetrics_SuccessRate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record 8 successes and 2 failures (80% success rate)
	for i := 0; i < 8; i++ {
		hub.RecordToolExecution("bash", true, 50*time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		hub.RecordToolExecution("bash", false, 25*time.Millisecond)
	}

	metrics := hub.GetToolExecutionMetrics("bash")
	sr := metrics.SuccessRate()

	if sr < 0.79 || sr > 0.81 {
		t.Errorf("expected ~0.8 success rate, got %.2f", sr)
	}
}

func TestToolExecutionMetrics_AvgLatency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	hub.RecordToolExecution("bash", true, 100*time.Millisecond)
	hub.RecordToolExecution("bash", true, 200*time.Millisecond)
	hub.RecordToolExecution("bash", false, 50*time.Millisecond)

	metrics := hub.GetToolExecutionMetrics("bash")
	avg := metrics.AvgLatency()

	// Average should be around 116ms (350 / 3)
	if avg < 100*time.Millisecond || avg > 150*time.Millisecond {
		t.Errorf("average latency out of expected range: %v", avg)
	}
}

func TestFailureMetrics_EdgeCases(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record many failures of different types
	for i := 0; i < 5; i++ {
		hub.RecordFailure("timeout", "query timeout")
	}
	for i := 0; i < 3; i++ {
		hub.RecordFailure("rate_limit", "429 response")
	}

	timeout := hub.GetFailureClassMetrics("timeout")
	if timeout == nil || timeout.Count != 5 {
		t.Errorf("expected 5 timeouts, got %v", timeout)
	}

	rateLimit := hub.GetFailureClassMetrics("rate_limit")
	if rateLimit == nil || rateLimit.Count != 3 {
		t.Errorf("expected 3 rate limits, got %v", rateLimit)
	}

	// Non-existent class should return nil
	missing := hub.GetFailureClassMetrics("missing")
	if missing != nil {
		t.Error("expected nil for non-existent failure class")
	}
}

func TestApprovalOutcomeMetrics_Distribution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record various outcomes for different risk levels
	hub.RecordApprovalOutcome("critical", "approved")
	hub.RecordApprovalOutcome("critical", "denied")
	hub.RecordApprovalOutcome("critical", "denied")
	hub.RecordApprovalOutcome("high", "approved")
	hub.RecordApprovalOutcome("high", "timed_out")

	critical := hub.GetApprovalOutcomeMetrics("critical")
	if critical.Approved != 1 || critical.Denied != 2 {
		t.Errorf("critical approval distribution wrong: approved=%d, denied=%d", critical.Approved, critical.Denied)
	}

	high := hub.GetApprovalOutcomeMetrics("high")
	if high.Approved != 1 || high.TimedOut != 1 {
		t.Errorf("high approval distribution wrong: approved=%d, timed_out=%d", high.Approved, high.TimedOut)
	}
}

func TestMemoryQualityMetrics_Averaging(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record with varying quality metrics
	hub.RecordMemoryQuality(0.90, 0.85, 0.95)  // High cache, high compression, very high relevance
	hub.RecordMemoryQuality(0.80, 0.75, 0.85)  // Lower values
	hub.RecordMemoryQuality(1.0, 1.0, 1.0)     // Perfect

	metrics := hub.GetMemoryQualityMetrics()

	// Averages should be around 0.90, 0.87, 0.93
	if metrics.AverageCacheHit < 0.88 || metrics.AverageCacheHit > 0.92 {
		t.Errorf("cache hit average out of range: %.2f", metrics.AverageCacheHit)
	}
	if metrics.AverageCompression < 0.85 || metrics.AverageCompression > 0.89 {
		t.Errorf("compression average out of range: %.2f", metrics.AverageCompression)
	}
	if metrics.AverageRelevance < 0.91 || metrics.AverageRelevance > 0.95 {
		t.Errorf("relevance average out of range: %.2f", metrics.AverageRelevance)
	}
}

func TestSelfImprovementMetrics_Lifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Simulate SI lifecycle
	hub.RecordSICycleStart()
	hub.RecordSICycleStart()

	hub.RecordSIProposal()
	hub.RecordSIProposal()
	hub.RecordSIProposal()
	hub.RecordSIProposal()

	hub.RecordSIAccepted()
	hub.RecordSIAccepted()

	hub.RecordSIRolledBack()

	hub.RecordSIFailed()

	metrics := hub.GetSelfImprovementMetrics()

	if metrics.CyclesStarted != 2 {
		t.Errorf("expected 2 cycles, got %d", metrics.CyclesStarted)
	}
	if metrics.ProposalsTotal != 4 {
		t.Errorf("expected 4 proposals, got %d", metrics.ProposalsTotal)
	}
	if metrics.Accepted != 2 {
		t.Errorf("expected 2 accepted, got %d", metrics.Accepted)
	}
	if metrics.Rolled != 1 {
		t.Errorf("expected 1 rolled back, got %d", metrics.Rolled)
	}
	if metrics.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", metrics.Failed)
	}
}

func TestCostMetrics_Accumulation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record multiple turns to same model
	for i := 0; i < 3; i++ {
		hub.RecordCost("anthropic", "claude-opus", 1000+i*100, 500+i*50, 0.01)
	}

	metrics := hub.GetCostMetrics("anthropic", "claude-opus")

	if metrics.TurnCount != 3 {
		t.Errorf("expected 3 turns, got %d", metrics.TurnCount)
	}
	if metrics.InputTokens != 3300 {  // 1000 + 1100 + 1200
		t.Errorf("expected 3300 input tokens, got %d", metrics.InputTokens)
	}
	if metrics.OutputTokens != 1650 {  // 500 + 550 + 600
		t.Errorf("expected 1650 output tokens, got %d", metrics.OutputTokens)
	}
	if metrics.TotalCost < 0.029 || metrics.TotalCost > 0.031 {  // 3 * 0.01 = 0.03
		t.Errorf("expected ~0.03 total cost, got %.4f", metrics.TotalCost)
	}
}

func TestObservabilityHub_MultipleProviders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record latencies for multiple providers
	hub.RecordProviderLatency("xai", "grok-3", 100*time.Millisecond)
	hub.RecordProviderLatency("xai", "grok-3", 150*time.Millisecond)
	hub.RecordProviderLatency("google", "gemini", 80*time.Millisecond)
	hub.RecordProviderLatency("google", "gemini", 120*time.Millisecond)

	xaiMetrics := hub.GetProviderLatencyMetrics("xai", "grok-3")
	if xaiMetrics == nil || xaiMetrics.Mean() == 0 {
		t.Error("expected xai metrics")
	}

	googleMetrics := hub.GetProviderLatencyMetrics("google", "gemini")
	if googleMetrics == nil || googleMetrics.Mean() == 0 {
		t.Error("expected google metrics")
	}

	// Metrics should be separate
	if xaiMetrics.Mean() == googleMetrics.Mean() {
		t.Error("provider metrics should be independent")
	}
}

func TestExportMetrics_ValidFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Record comprehensive data
	hub.RecordProviderLatency("xai", "grok-3", 100*time.Millisecond)
	hub.RecordToolExecution("bash", true, 50*time.Millisecond)
	hub.RecordFailure("timeout", "test timeout")
	hub.RecordApprovalOutcome("medium", "approved")
	hub.RecordCost("xai", "grok-3", 1000, 500, 0.015)
	hub.RecordMemoryQuality(0.8, 0.75, 0.9)

	output := hub.ExportMetrics()

	// Verify Prometheus format elements
	if !contains(output, "# TYPE") {
		t.Error("missing prometheus TYPE comments")
	}
	if !contains(output, "# HELP") {
		t.Error("missing prometheus HELP comments")
	}
	if !contains(output, "gorkbot_") {
		t.Error("missing gorkbot metric prefix")
	}

	// Verify we can parse lines
	lines := findLines(output)
	if len(lines) < 5 {
		t.Errorf("expected at least 5 metric lines, got %d", len(lines))
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || (len(s) > len(substr) && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func findLines(s string) []string {
	var lines []string
	current := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func TestProviderLatencyMetrics_SingleRecord(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// Single record
	hub.RecordProviderLatency("xai", "grok-3", 100*time.Millisecond)

	metrics := hub.GetProviderLatencyMetrics("xai", "grok-3")
	mean := metrics.Mean()

	if mean != 100*time.Millisecond {
		t.Errorf("single record mean should be exactly 100ms, got %v", mean)
	}

	p50 := metrics.P50()
	if p50 != 100*time.Millisecond {
		t.Errorf("single record P50 should be 100ms, got %v", p50)
	}
}

func TestToolExecutionMetrics_AllSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// All successful
	for i := 0; i < 10; i++ {
		hub.RecordToolExecution("bash", true, 50*time.Millisecond)
	}

	metrics := hub.GetToolExecutionMetrics("bash")
	sr := metrics.SuccessRate()

	if sr != 1.0 {
		t.Errorf("100%% success rate should be 1.0, got %.2f", sr)
	}
}

func TestToolExecutionMetrics_AllFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	// All failures
	for i := 0; i < 5; i++ {
		hub.RecordToolExecution("read_file", false, 25*time.Millisecond)
	}

	metrics := hub.GetToolExecutionMetrics("read_file")
	sr := metrics.SuccessRate()

	if sr != 0.0 {
		t.Errorf("0%% success rate should be 0.0, got %.2f", sr)
	}
}

func TestSelfImprovementMetrics_NoActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	metrics := hub.GetSelfImprovementMetrics()
	if metrics.CyclesStarted != 0 {
		t.Error("should have no cycles with no activity")
	}
}

func TestCostMetrics_SingleTurn(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	hub.RecordCost("xai", "grok-3", 1000, 500, 0.015)

	metrics := hub.GetCostMetrics("xai", "grok-3")
	if metrics.TurnCount != 1 {
		t.Errorf("expected 1 turn, got %d", metrics.TurnCount)
	}
	if metrics.InputTokens != 1000 {
		t.Errorf("expected 1000 input tokens, got %d", metrics.InputTokens)
	}
	if metrics.OutputTokens != 500 {
		t.Errorf("expected 500 output tokens, got %d", metrics.OutputTokens)
	}
	if metrics.TotalCost != 0.015 {
		t.Errorf("expected 0.015 cost, got %f", metrics.TotalCost)
	}
}

func TestMemoryQualityMetrics_PerfectScores(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := NewObservabilityHub(logger)

	for i := 0; i < 5; i++ {
		hub.RecordMemoryQuality(1.0, 1.0, 1.0)
	}

	metrics := hub.GetMemoryQualityMetrics()
	if metrics.AverageCacheHit != 1.0 || metrics.AverageCompression != 1.0 || metrics.AverageRelevance != 1.0 {
		t.Error("perfect scores should average to 1.0")
	}
}
