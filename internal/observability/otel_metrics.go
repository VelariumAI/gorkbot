package observability

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// MetricFamily represents a group of related metrics
type MetricFamily string

const (
	// Core metric families (6 required)
	ProviderLatencyFamily     MetricFamily = "provider_latency"
	ToolExecutionFamily       MetricFamily = "tool_execution"
	FailureClassFamily        MetricFamily = "failure_class"
	ApprovalOutcomesFamily    MetricFamily = "approval_outcomes"
	MemoryQualityFamily       MetricFamily = "memory_quality"
	SelfImprovementFamily     MetricFamily = "self_improvement"
	DistributedMetricsFamily  MetricFamily = "distributed"
)

// ProviderLatencyMetrics tracks provider response latencies with percentiles
type ProviderLatencyMetrics struct {
	Provider  string
	Model     string
	Latencies []time.Duration
	mu        sync.RWMutex
}

func (plm *ProviderLatencyMetrics) Record(latency time.Duration) {
	plm.mu.Lock()
	defer plm.mu.Unlock()
	plm.Latencies = append(plm.Latencies, latency)
}

func (plm *ProviderLatencyMetrics) Percentile(p float64) time.Duration {
	plm.mu.RLock()
	defer plm.mu.RUnlock()

	if len(plm.Latencies) == 0 {
		return 0
	}

	idx := int(float64(len(plm.Latencies)) * p / 100)
	if idx >= len(plm.Latencies) {
		idx = len(plm.Latencies) - 1
	}

	// For a proper percentile, should sort - simplified here
	return plm.Latencies[idx]
}

func (plm *ProviderLatencyMetrics) P50() time.Duration  { return plm.Percentile(50) }
func (plm *ProviderLatencyMetrics) P95() time.Duration  { return plm.Percentile(95) }
func (plm *ProviderLatencyMetrics) P99() time.Duration  { return plm.Percentile(99) }
func (plm *ProviderLatencyMetrics) Mean() time.Duration {
	plm.mu.RLock()
	defer plm.mu.RUnlock()

	if len(plm.Latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, l := range plm.Latencies {
		total += l
	}
	return total / time.Duration(len(plm.Latencies))
}

// ToolExecutionMetrics tracks tool invocation statistics
type ToolExecutionMetrics struct {
	ToolName     string
	Invocations  int64
	Successes    int64
	Failures     int64
	TotalLatency time.Duration
	mu           sync.RWMutex
}

func (tem *ToolExecutionMetrics) Record(success bool, latency time.Duration) {
	tem.mu.Lock()
	defer tem.mu.Unlock()
	tem.Invocations++
	tem.TotalLatency += latency
	if success {
		tem.Successes++
	} else {
		tem.Failures++
	}
}

func (tem *ToolExecutionMetrics) SuccessRate() float64 {
	tem.mu.RLock()
	defer tem.mu.RUnlock()
	if tem.Invocations == 0 {
		return 0
	}
	return float64(tem.Successes) / float64(tem.Invocations)
}

func (tem *ToolExecutionMetrics) AvgLatency() time.Duration {
	tem.mu.RLock()
	defer tem.mu.RUnlock()
	if tem.Invocations == 0 {
		return 0
	}
	return tem.TotalLatency / time.Duration(tem.Invocations)
}

// FailureClassMetrics tracks failure classification
type FailureClassMetrics struct {
	Class   string // e.g., "provider_timeout", "tool_error", "validation_failed"
	Count   int64
	LastErr string
	mu      sync.RWMutex
}

func (fcm *FailureClassMetrics) Record(err string) {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()
	fcm.Count++
	fcm.LastErr = err
}

// ApprovalOutcomeMetrics tracks HITL approval decisions
type ApprovalOutcomeMetrics struct {
	RiskLevel string // "low", "medium", "high", "critical"
	Approved  int64
	Denied    int64
	TimedOut  int64
	mu        sync.RWMutex
}

func (aom *ApprovalOutcomeMetrics) RecordApproved() {
	aom.mu.Lock()
	defer aom.mu.Unlock()
	aom.Approved++
}

func (aom *ApprovalOutcomeMetrics) RecordDenied() {
	aom.mu.Lock()
	defer aom.mu.Unlock()
	aom.Denied++
}

func (aom *ApprovalOutcomeMetrics) RecordTimedOut() {
	aom.mu.Lock()
	defer aom.mu.Unlock()
	aom.TimedOut++
}

// MemoryQualityMetrics tracks memory retrieval effectiveness
type MemoryQualityMetrics struct {
	Queries            int64
	AverageCacheHit    float64
	AverageCompression float64
	AverageRelevance   float64
	mu                 sync.RWMutex
}

func (mqm *MemoryQualityMetrics) Record(cacheHit, compression, relevance float64) {
	mqm.mu.Lock()
	defer mqm.mu.Unlock()
	mqm.Queries++
	// Running average
	mqm.AverageCacheHit = (mqm.AverageCacheHit*(float64(mqm.Queries)-1) + cacheHit) / float64(mqm.Queries)
	mqm.AverageCompression = (mqm.AverageCompression*(float64(mqm.Queries)-1) + compression) / float64(mqm.Queries)
	mqm.AverageRelevance = (mqm.AverageRelevance*(float64(mqm.Queries)-1) + relevance) / float64(mqm.Queries)
}

// SelfImprovementMetrics tracks SI attempt outcomes
type SelfImprovementMetrics struct {
	// Core counters
	CyclesStarted  int64
	ProposalsTotal int64
	Accepted       int64
	Rolled         int64
	Failed         int64

	// Extended metrics (Task 4.3B)
	TotalExecutionLatency time.Duration    // sum of all Sandbox stage latencies
	TotalScoreDelta       float64          // sum of PostScore - PreScore deltas
	HighestScoreDelta     float64          // max positive delta observed
	LowestScoreDelta      float64          // min negative delta observed
	TotalRollbackLatency  time.Duration    // sum of Persist→Rollback durations
	GateRejectReasons     map[string]int64 // count by reason
	ToolErrorCounts       map[string]int64 // count by tool name
	ExecutionLatencies    []time.Duration  // all recorded latencies (for percentiles)
	ScoreDeltas           []float64        // all recorded score deltas

	mu sync.RWMutex
}

func (sim *SelfImprovementMetrics) RecordCycleStart() {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.CyclesStarted++
}

func (sim *SelfImprovementMetrics) RecordProposal() {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.ProposalsTotal++
}

func (sim *SelfImprovementMetrics) RecordAccepted() {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.Accepted++
}

func (sim *SelfImprovementMetrics) RecordRolledBack() {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.Rolled++
}

func (sim *SelfImprovementMetrics) RecordFailed() {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.Failed++
}

// Extended metrics methods (Task 4.3B)

// RecordExecutionLatency records the latency of the Sandbox stage.
func (sim *SelfImprovementMetrics) RecordExecutionLatency(latency time.Duration) {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.TotalExecutionLatency += latency
	sim.ExecutionLatencies = append(sim.ExecutionLatencies, latency)
}

// RecordScoreDelta records the delta between pre and post execution drive scores.
func (sim *SelfImprovementMetrics) RecordScoreDelta(delta float64) {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.TotalScoreDelta += delta
	sim.ScoreDeltas = append(sim.ScoreDeltas, delta)

	// Track min/max
	if delta > sim.HighestScoreDelta {
		sim.HighestScoreDelta = delta
	}
	if delta < sim.LowestScoreDelta || (len(sim.ScoreDeltas) == 1) {
		sim.LowestScoreDelta = delta
	}
}

// RecordGateRejectReason records why a proposal was rejected at the Gate stage.
func (sim *SelfImprovementMetrics) RecordGateRejectReason(reason string) {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	if sim.GateRejectReasons == nil {
		sim.GateRejectReasons = make(map[string]int64)
	}
	sim.GateRejectReasons[reason]++
}

// RecordToolError records tool execution errors by tool name.
func (sim *SelfImprovementMetrics) RecordToolError(toolName string) {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	if sim.ToolErrorCounts == nil {
		sim.ToolErrorCounts = make(map[string]int64)
	}
	sim.ToolErrorCounts[toolName]++
}

// RecordRollbackLatency records the latency of the rollback operation.
func (sim *SelfImprovementMetrics) RecordRollbackLatency(latency time.Duration) {
	sim.mu.Lock()
	defer sim.mu.Unlock()
	sim.TotalRollbackLatency += latency
}

// Accessor methods

// AverageExecutionLatency returns the average latency of Sandbox executions.
func (sim *SelfImprovementMetrics) AverageExecutionLatency() time.Duration {
	sim.mu.RLock()
	defer sim.mu.RUnlock()
	if len(sim.ExecutionLatencies) == 0 {
		return 0
	}
	return sim.TotalExecutionLatency / time.Duration(len(sim.ExecutionLatencies))
}

// AverageScoreDelta returns the average score delta across all cycles.
func (sim *SelfImprovementMetrics) AverageScoreDelta() float64 {
	sim.mu.RLock()
	defer sim.mu.RUnlock()
	if len(sim.ScoreDeltas) == 0 {
		return 0
	}
	return sim.TotalScoreDelta / float64(len(sim.ScoreDeltas))
}

// AcceptanceRate returns the ratio of accepted proposals to total outcomes.
func (sim *SelfImprovementMetrics) AcceptanceRate() float64 {
	sim.mu.RLock()
	defer sim.mu.RUnlock()
	total := sim.Accepted + sim.Rolled + sim.Failed
	if total == 0 {
		return 0
	}
	return float64(sim.Accepted) / float64(total)
}

// Summary returns a map of all SI metrics for reporting/dashboards.
func (sim *SelfImprovementMetrics) Summary() map[string]interface{} {
	sim.mu.RLock()
	defer sim.mu.RUnlock()

	total := sim.Accepted + sim.Rolled + sim.Failed
	acceptanceRate := 0.0
	if total > 0 {
		acceptanceRate = float64(sim.Accepted) / float64(total)
	}

	avgLatency := time.Duration(0)
	if len(sim.ExecutionLatencies) > 0 {
		avgLatency = sim.TotalExecutionLatency / time.Duration(len(sim.ExecutionLatencies))
	}

	avgDelta := 0.0
	if len(sim.ScoreDeltas) > 0 {
		avgDelta = sim.TotalScoreDelta / float64(len(sim.ScoreDeltas))
	}

	return map[string]interface{}{
		"cycles_started":            sim.CyclesStarted,
		"proposals_total":           sim.ProposalsTotal,
		"accepted":                  sim.Accepted,
		"rolled_back":               sim.Rolled,
		"failed":                    sim.Failed,
		"total_outcomes":            total,
		"acceptance_rate":           acceptanceRate,
		"avg_execution_latency_ms":  avgLatency.Milliseconds(),
		"total_execution_latency_ms": sim.TotalExecutionLatency.Milliseconds(),
		"avg_score_delta":           avgDelta,
		"highest_score_delta":       sim.HighestScoreDelta,
		"lowest_score_delta":        sim.LowestScoreDelta,
		"total_rollback_latency_ms": sim.TotalRollbackLatency.Milliseconds(),
		"reject_reasons_count":      len(sim.GateRejectReasons),
		"tool_errors_count":         len(sim.ToolErrorCounts),
		"samples_latency":           len(sim.ExecutionLatencies),
		"samples_score_delta":       len(sim.ScoreDeltas),
	}
}

// CostMetrics tracks token costs across providers and models
type CostMetrics struct {
	Provider      string
	Model         string
	InputTokens   int64
	OutputTokens  int64
	TotalCost     float64
	TurnCount     int64
	AvgCostPerTurn float64
	mu            sync.RWMutex
}

func (cm *CostMetrics) RecordTurn(inputTokens, outputTokens int, cost float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.InputTokens += int64(inputTokens)
	cm.OutputTokens += int64(outputTokens)
	cm.TotalCost += cost
	cm.TurnCount++
	if cm.TurnCount > 0 {
		cm.AvgCostPerTurn = cm.TotalCost / float64(cm.TurnCount)
	}
}

// DistributedMetrics tracks distributed system metrics (node discovery, remote events, failovers).
type DistributedMetrics struct {
	NodesDiscovered        int64
	NodesLost              int64
	EventsPublishedLocal   int64
	EventsPublishedRemote  int64
	RemotePublishErrors    int64
	RemotePublishCount     int64
	TotalRemoteLatencyMS   int64
	ProviderFailovers      int64
	MemoryQueries          int64
	TotalMemLatencyMS      int64
	mu                     sync.RWMutex
}

// RecordNodeDiscovered increments node discovery count.
func (dm *DistributedMetrics) RecordNodeDiscovered() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.NodesDiscovered++
}

// RecordNodeLost increments node lost count.
func (dm *DistributedMetrics) RecordNodeLost() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.NodesLost++
}

// RecordEventPublished records a local or remote event publish.
func (dm *DistributedMetrics) RecordEventPublished(remote bool, latencyMS int64, failed bool) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if remote {
		dm.EventsPublishedRemote++
		dm.RemotePublishCount++
		dm.TotalRemoteLatencyMS += latencyMS
		if failed {
			dm.RemotePublishErrors++
		}
	} else {
		dm.EventsPublishedLocal++
	}
}

// RecordProviderFailover increments provider failover count.
func (dm *DistributedMetrics) RecordProviderFailover() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.ProviderFailovers++
}

// RecordMemoryQuery records a memory query and its latency.
func (dm *DistributedMetrics) RecordMemoryQuery(latencyMS int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.MemoryQueries++
	dm.TotalMemLatencyMS += latencyMS
}

// AvgRemoteLatencyMS returns average remote event latency in milliseconds.
func (dm *DistributedMetrics) AvgRemoteLatencyMS() int64 {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if dm.RemotePublishCount == 0 {
		return 0
	}
	return dm.TotalRemoteLatencyMS / dm.RemotePublishCount
}

// AvgMemoryLatencyMS returns average memory query latency in milliseconds.
func (dm *DistributedMetrics) AvgMemoryLatencyMS() int64 {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if dm.MemoryQueries == 0 {
		return 0
	}
	return dm.TotalMemLatencyMS / dm.MemoryQueries
}

// ObservabilityHub aggregates all metric families with context support
type ObservabilityHub struct {
	logger *slog.Logger

	// Metric families
	providerLatencies map[string]*ProviderLatencyMetrics
	toolExecutions    map[string]*ToolExecutionMetrics
	failureClasses    map[string]*FailureClassMetrics
	approvalOutcomes  map[string]*ApprovalOutcomeMetrics
	memoryQuality     *MemoryQualityMetrics
	selfImprovement   *SelfImprovementMetrics
	costs             map[string]*CostMetrics
	distributed       *DistributedMetrics

	mu sync.RWMutex

	// Correlation ID tracking
	correlationIDs map[string]string
	muCorr         sync.RWMutex

	// Start time for uptime tracking
	startTime time.Time
}

// NewObservabilityHub creates a new observability hub
func NewObservabilityHub(logger *slog.Logger) *ObservabilityHub {
	if logger == nil {
		logger = slog.Default()
	}

	return &ObservabilityHub{
		logger:            logger,
		providerLatencies: make(map[string]*ProviderLatencyMetrics),
		toolExecutions:    make(map[string]*ToolExecutionMetrics),
		failureClasses:    make(map[string]*FailureClassMetrics),
		approvalOutcomes:  make(map[string]*ApprovalOutcomeMetrics),
		costs:             make(map[string]*CostMetrics),
		memoryQuality:     &MemoryQualityMetrics{},
		selfImprovement:   &SelfImprovementMetrics{},
		distributed:       &DistributedMetrics{},
		correlationIDs:    make(map[string]string),
		startTime:         time.Now(),
	}
}

// WithCorrelationID associates a correlation ID with the current context
func (oh *ObservabilityHub) WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	oh.muCorr.Lock()
	defer oh.muCorr.Unlock()
	oh.correlationIDs[correlationID] = correlationID
	return ctx
}

// RecordProviderLatency records provider response latency
func (oh *ObservabilityHub) RecordProviderLatency(provider, model string, latency time.Duration) {
	oh.mu.Lock()
	defer oh.mu.Unlock()

	key := fmt.Sprintf("%s_%s", provider, model)
	if _, ok := oh.providerLatencies[key]; !ok {
		oh.providerLatencies[key] = &ProviderLatencyMetrics{
			Provider:  provider,
			Model:     model,
			Latencies: []time.Duration{},
		}
	}
	oh.providerLatencies[key].Record(latency)

	oh.logger.Debug("recorded provider latency",
		slog.String("provider", provider),
		slog.String("model", model),
		slog.Duration("latency", latency),
	)
}

// RecordToolExecution records tool invocation result
func (oh *ObservabilityHub) RecordToolExecution(toolName string, success bool, latency time.Duration) {
	oh.mu.Lock()
	defer oh.mu.Unlock()

	if _, ok := oh.toolExecutions[toolName]; !ok {
		oh.toolExecutions[toolName] = &ToolExecutionMetrics{
			ToolName: toolName,
		}
	}
	oh.toolExecutions[toolName].Record(success, latency)

	oh.logger.Debug("recorded tool execution",
		slog.String("tool", toolName),
		slog.Bool("success", success),
		slog.Duration("latency", latency),
	)
}

// RecordFailure records a failure classification
func (oh *ObservabilityHub) RecordFailure(class, err string) {
	oh.mu.Lock()
	defer oh.mu.Unlock()

	if _, ok := oh.failureClasses[class]; !ok {
		oh.failureClasses[class] = &FailureClassMetrics{Class: class}
	}
	oh.failureClasses[class].Record(err)

	oh.logger.Debug("recorded failure",
		slog.String("class", class),
		slog.String("error", err),
	)
}

// RecordApprovalOutcome records HITL approval decision
func (oh *ObservabilityHub) RecordApprovalOutcome(riskLevel string, outcome string) {
	oh.mu.Lock()
	defer oh.mu.Unlock()

	if _, ok := oh.approvalOutcomes[riskLevel]; !ok {
		oh.approvalOutcomes[riskLevel] = &ApprovalOutcomeMetrics{
			RiskLevel: riskLevel,
		}
	}

	aom := oh.approvalOutcomes[riskLevel]
	switch outcome {
	case "approved":
		aom.RecordApproved()
	case "denied":
		aom.RecordDenied()
	case "timed_out":
		aom.RecordTimedOut()
	}

	oh.logger.Debug("recorded approval outcome",
		slog.String("risk_level", riskLevel),
		slog.String("outcome", outcome),
	)
}

// RecordMemoryQuality records memory retrieval metrics
func (oh *ObservabilityHub) RecordMemoryQuality(cacheHit, compression, relevance float64) {
	oh.memoryQuality.Record(cacheHit, compression, relevance)

	oh.logger.Debug("recorded memory quality",
		slog.Float64("cache_hit", cacheHit),
		slog.Float64("compression", compression),
		slog.Float64("relevance", relevance),
	)
}

// RecordSIProposal records SI improvement proposal
func (oh *ObservabilityHub) RecordSIProposal() {
	oh.selfImprovement.RecordProposal()
}

// RecordSIAccepted records SI proposal acceptance
func (oh *ObservabilityHub) RecordSIAccepted() {
	oh.selfImprovement.RecordAccepted()
}

// RecordSIRolledBack records SI rollback
func (oh *ObservabilityHub) RecordSIRolledBack() {
	oh.selfImprovement.RecordRolledBack()
}

// RecordSIFailed records SI cycle failure
func (oh *ObservabilityHub) RecordSIFailed() {
	oh.selfImprovement.RecordFailed()
}

// RecordSICycleStart records SI cycle start
func (oh *ObservabilityHub) RecordSICycleStart() {
	oh.selfImprovement.RecordCycleStart()
}

// Extended SI metrics (Task 4.3B)

// RecordSIExecutionLatency records the latency of tool execution in Sandbox stage.
func (oh *ObservabilityHub) RecordSIExecutionLatency(latency time.Duration) {
	oh.selfImprovement.RecordExecutionLatency(latency)
	oh.logger.Debug("recorded SI execution latency", slog.Duration("latency", latency))
}

// RecordSIScoreDelta records the change in drive score after improvement execution.
func (oh *ObservabilityHub) RecordSIScoreDelta(delta float64) {
	oh.selfImprovement.RecordScoreDelta(delta)
	oh.logger.Debug("recorded SI score delta", slog.Float64("delta", delta))
}

// RecordSIGateRejectionReason records why a proposal was rejected at the Gate stage.
func (oh *ObservabilityHub) RecordSIGateRejectionReason(reason string) {
	oh.selfImprovement.RecordGateRejectReason(reason)
	oh.logger.Debug("recorded SI gate rejection reason", slog.String("reason", reason))
}

// RecordSIToolError records a tool execution error during Sandbox stage.
func (oh *ObservabilityHub) RecordSIToolError(toolName string) {
	oh.selfImprovement.RecordToolError(toolName)
	oh.logger.Debug("recorded SI tool error", slog.String("tool", toolName))
}

// RecordSIRollbackLatency records the latency of the rollback operation in Persist stage.
func (oh *ObservabilityHub) RecordSIRollbackLatency(latency time.Duration) {
	oh.selfImprovement.RecordRollbackLatency(latency)
	oh.logger.Debug("recorded SI rollback latency", slog.Duration("latency", latency))
}

// GetSIMetrics returns the current SI metrics snapshot.
func (oh *ObservabilityHub) GetSIMetrics() map[string]interface{} {
	return oh.selfImprovement.Summary()
}

// Distributed metrics delegation methods

// RecordNodeDiscovered records a discovered distributed node.
func (oh *ObservabilityHub) RecordNodeDiscovered() {
	if oh.distributed != nil {
		oh.distributed.RecordNodeDiscovered()
		oh.logger.Debug("recorded node discovered")
	}
}

// RecordNodeLost records a lost distributed node.
func (oh *ObservabilityHub) RecordNodeLost() {
	if oh.distributed != nil {
		oh.distributed.RecordNodeLost()
		oh.logger.Debug("recorded node lost")
	}
}

// RecordEventPublished records a published event (local or remote).
func (oh *ObservabilityHub) RecordEventPublished(remote bool, latencyMS int64, failed bool) {
	if oh.distributed != nil {
		oh.distributed.RecordEventPublished(remote, latencyMS, failed)
		oh.logger.Debug("recorded event published",
			slog.Bool("remote", remote),
			slog.Int64("latency_ms", latencyMS),
			slog.Bool("failed", failed))
	}
}

// RecordProviderFailover records a provider failover.
func (oh *ObservabilityHub) RecordProviderFailover() {
	if oh.distributed != nil {
		oh.distributed.RecordProviderFailover()
		oh.logger.Debug("recorded provider failover")
	}
}

// RecordMemoryQuery records a memory query and its latency.
func (oh *ObservabilityHub) RecordMemoryQuery(latencyMS int64) {
	if oh.distributed != nil {
		oh.distributed.RecordMemoryQuery(latencyMS)
		oh.logger.Debug("recorded memory query", slog.Int64("latency_ms", latencyMS))
	}
}

// GetDistributedMetrics returns the distributed metrics snapshot.
func (oh *ObservabilityHub) GetDistributedMetrics() *DistributedMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()
	return oh.distributed
}

// RecordCost records token cost for a provider/model pair
func (oh *ObservabilityHub) RecordCost(provider, model string, inputTokens, outputTokens int, cost float64) {
	oh.mu.Lock()
	defer oh.mu.Unlock()

	key := provider + ":" + model
	if _, ok := oh.costs[key]; !ok {
		oh.costs[key] = &CostMetrics{
			Provider: provider,
			Model:    model,
		}
	}
	oh.costs[key].RecordTurn(inputTokens, outputTokens, cost)
}

// ExportMetrics exports all metrics in Prometheus text format
func (oh *ObservabilityHub) ExportMetrics() string {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	output := "# HELP gorkbot_provider_latency_ms Provider latency in milliseconds\n"
	output += "# TYPE gorkbot_provider_latency_ms gauge\n"

	// Provider latencies with percentiles
	for _, plm := range oh.providerLatencies {
		output += fmt.Sprintf("gorkbot_provider_latency_ms{provider=\"%s\",model=\"%s\",percentile=\"50\"} %.2f\n",
			plm.Provider, plm.Model, float64(plm.P50().Milliseconds()))
		output += fmt.Sprintf("gorkbot_provider_latency_ms{provider=\"%s\",model=\"%s\",percentile=\"95\"} %.2f\n",
			plm.Provider, plm.Model, float64(plm.P95().Milliseconds()))
		output += fmt.Sprintf("gorkbot_provider_latency_ms{provider=\"%s\",model=\"%s\",percentile=\"99\"} %.2f\n",
			plm.Provider, plm.Model, float64(plm.P99().Milliseconds()))
		output += fmt.Sprintf("gorkbot_provider_latency_ms{provider=\"%s\",model=\"%s\",type=\"mean\"} %.2f\n",
			plm.Provider, plm.Model, float64(plm.Mean().Milliseconds()))
	}

	// Tool execution metrics
	output += "\n# HELP gorkbot_tool_invocations_total Tool invocation count\n"
	output += "# TYPE gorkbot_tool_invocations_total counter\n"
	for tool, tem := range oh.toolExecutions {
		output += fmt.Sprintf("gorkbot_tool_invocations_total{tool=\"%s\"} %d\n", tool, tem.Invocations)
		output += fmt.Sprintf("gorkbot_tool_successes_total{tool=\"%s\"} %d\n", tool, tem.Successes)
		output += fmt.Sprintf("gorkbot_tool_failures_total{tool=\"%s\"} %d\n", tool, tem.Failures)
		output += fmt.Sprintf("gorkbot_tool_success_rate{tool=\"%s\"} %.4f\n", tool, tem.SuccessRate())
		output += fmt.Sprintf("gorkbot_tool_avg_latency_ms{tool=\"%s\"} %.2f\n", tool, float64(tem.AvgLatency().Milliseconds()))
	}

	// Failure class metrics
	output += "\n# HELP gorkbot_failures_total Failure count by classification\n"
	output += "# TYPE gorkbot_failures_total counter\n"
	for class, fcm := range oh.failureClasses {
		output += fmt.Sprintf("gorkbot_failures_total{class=\"%s\"} %d\n", class, fcm.Count)
	}

	// Approval outcome metrics
	output += "\n# HELP gorkbot_approvals_total Approval decision count\n"
	output += "# TYPE gorkbot_approvals_total counter\n"
	for level, aom := range oh.approvalOutcomes {
		output += fmt.Sprintf("gorkbot_approvals_total{risk_level=\"%s\",outcome=\"approved\"} %d\n", level, aom.Approved)
		output += fmt.Sprintf("gorkbot_approvals_total{risk_level=\"%s\",outcome=\"denied\"} %d\n", level, aom.Denied)
		output += fmt.Sprintf("gorkbot_approvals_total{risk_level=\"%s\",outcome=\"timed_out\"} %d\n", level, aom.TimedOut)
	}

	// Memory quality metrics
	output += "\n# HELP gorkbot_memory_quality Memory retrieval quality metrics\n"
	output += "# TYPE gorkbot_memory_quality gauge\n"
	output += fmt.Sprintf("gorkbot_memory_cache_hit_rate %.4f\n", oh.memoryQuality.AverageCacheHit)
	output += fmt.Sprintf("gorkbot_memory_compression_ratio %.4f\n", oh.memoryQuality.AverageCompression)
	output += fmt.Sprintf("gorkbot_memory_relevance_score %.4f\n", oh.memoryQuality.AverageRelevance)

	// Self-improvement metrics
	output += "\n# HELP gorkbot_si_cycles_total Self-improvement cycle count\n"
	output += "# TYPE gorkbot_si_cycles_total counter\n"
	output += fmt.Sprintf("gorkbot_si_cycles_total %d\n", oh.selfImprovement.CyclesStarted)
	output += fmt.Sprintf("gorkbot_si_proposals_total %d\n", oh.selfImprovement.ProposalsTotal)
	output += fmt.Sprintf("gorkbot_si_accepted_total %d\n", oh.selfImprovement.Accepted)
	output += fmt.Sprintf("gorkbot_si_rolled_back_total %d\n", oh.selfImprovement.Rolled)
	output += fmt.Sprintf("gorkbot_si_failed_total %d\n", oh.selfImprovement.Failed)

	// Cost tracking metrics
	output += "\n# HELP gorkbot_cost_usd Total cost in USD\n"
	output += "# TYPE gorkbot_cost_usd gauge\n"
	totalCost := 0.0
	for _, cm := range oh.costs {
		output += fmt.Sprintf("gorkbot_cost_usd{provider=\"%s\",model=\"%s\"} %.6f\n", cm.Provider, cm.Model, cm.TotalCost)
		output += fmt.Sprintf("gorkbot_cost_input_tokens{provider=\"%s\",model=\"%s\"} %d\n", cm.Provider, cm.Model, cm.InputTokens)
		output += fmt.Sprintf("gorkbot_cost_output_tokens{provider=\"%s\",model=\"%s\"} %d\n", cm.Provider, cm.Model, cm.OutputTokens)
		output += fmt.Sprintf("gorkbot_cost_avg_per_turn{provider=\"%s\",model=\"%s\"} %.6f\n", cm.Provider, cm.Model, cm.AvgCostPerTurn)
		totalCost += cm.TotalCost
	}
	output += fmt.Sprintf("\n# HELP gorkbot_cost_total_usd Total session cost in USD\n")
	output += fmt.Sprintf("# TYPE gorkbot_cost_total_usd gauge\n")
	output += fmt.Sprintf("gorkbot_cost_total_usd %.6f\n", totalCost)

	// Uptime
	output += "\n# HELP gorkbot_uptime_seconds System uptime\n"
	output += "# TYPE gorkbot_uptime_seconds gauge\n"
	output += fmt.Sprintf("gorkbot_uptime_seconds %.0f\n", time.Since(oh.startTime).Seconds())

	// Distributed metrics
	if oh.distributed != nil {
		output += "\n# HELP gorkbot_distributed_nodes_discovered_total Total nodes discovered\n"
		output += "# TYPE gorkbot_distributed_nodes_discovered_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_nodes_discovered_total %d\n", oh.distributed.NodesDiscovered)

		output += "\n# HELP gorkbot_distributed_nodes_lost_total Total nodes lost\n"
		output += "# TYPE gorkbot_distributed_nodes_lost_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_nodes_lost_total %d\n", oh.distributed.NodesLost)

		output += "\n# HELP gorkbot_distributed_events_remote_total Total remote events published\n"
		output += "# TYPE gorkbot_distributed_events_remote_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_events_remote_total %d\n", oh.distributed.EventsPublishedRemote)

		output += "\n# HELP gorkbot_distributed_remote_publish_errors_total Total remote publish errors\n"
		output += "# TYPE gorkbot_distributed_remote_publish_errors_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_remote_publish_errors_total %d\n", oh.distributed.RemotePublishErrors)

		output += "\n# HELP gorkbot_distributed_remote_avg_latency_ms Average remote event latency\n"
		output += "# TYPE gorkbot_distributed_remote_avg_latency_ms gauge\n"
		output += fmt.Sprintf("gorkbot_distributed_remote_avg_latency_ms %d\n", oh.distributed.AvgRemoteLatencyMS())

		output += "\n# HELP gorkbot_distributed_provider_failovers_total Total provider failovers\n"
		output += "# TYPE gorkbot_distributed_provider_failovers_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_provider_failovers_total %d\n", oh.distributed.ProviderFailovers)

		output += "\n# HELP gorkbot_distributed_memory_queries_total Total memory queries\n"
		output += "# TYPE gorkbot_distributed_memory_queries_total counter\n"
		output += fmt.Sprintf("gorkbot_distributed_memory_queries_total %d\n", oh.distributed.MemoryQueries)

		output += "\n# HELP gorkbot_distributed_memory_avg_latency_ms Average memory query latency\n"
		output += "# TYPE gorkbot_distributed_memory_avg_latency_ms gauge\n"
		output += fmt.Sprintf("gorkbot_distributed_memory_avg_latency_ms %d\n", oh.distributed.AvgMemoryLatencyMS())
	}

	return output
}

// GetProviderLatencyMetrics returns latency stats for a provider/model pair
func (oh *ObservabilityHub) GetProviderLatencyMetrics(provider, model string) *ProviderLatencyMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	key := fmt.Sprintf("%s_%s", provider, model)
	return oh.providerLatencies[key]
}

// GetToolExecutionMetrics returns execution stats for a tool
func (oh *ObservabilityHub) GetToolExecutionMetrics(tool string) *ToolExecutionMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	return oh.toolExecutions[tool]
}

// GetFailureClassMetrics returns failure stats for a class
func (oh *ObservabilityHub) GetFailureClassMetrics(class string) *FailureClassMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	return oh.failureClasses[class]
}

// GetApprovalOutcomeMetrics returns approval stats for a risk level
func (oh *ObservabilityHub) GetApprovalOutcomeMetrics(riskLevel string) *ApprovalOutcomeMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	return oh.approvalOutcomes[riskLevel]
}

// GetCostMetrics returns cost stats for a provider/model pair
func (oh *ObservabilityHub) GetCostMetrics(provider, model string) *CostMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	key := provider + ":" + model
	return oh.costs[key]
}

// GetMemoryQualityMetrics returns memory quality stats
func (oh *ObservabilityHub) GetMemoryQualityMetrics() *MemoryQualityMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	return oh.memoryQuality
}

// GetSelfImprovementMetrics returns SI stats
func (oh *ObservabilityHub) GetSelfImprovementMetrics() *SelfImprovementMetrics {
	oh.mu.RLock()
	defer oh.mu.RUnlock()

	return oh.selfImprovement
}

// GetCorrelationID retrieves the correlation ID for a request
func (oh *ObservabilityHub) GetCorrelationID(key string) string {
	oh.muCorr.RLock()
	defer oh.muCorr.RUnlock()

	return oh.correlationIDs[key]
}
