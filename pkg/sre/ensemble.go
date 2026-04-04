package sre

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/spark"
	"github.com/velariumai/gorkbot/pkg/subagents"
)

// TrajectoryResult holds the output and quality score of one parallel trace.
type TrajectoryResult struct {
	Label   string
	Output  string
	Score   float64 // quality score from SPARK.ObserveEnsemble; 0 = unscored
	Elapsed time.Duration
	Err     error
}

// TemperatureConfigurable is the optional ai.AIProvider extension for temperature variation.
type TemperatureConfigurable interface {
	WithTemperature(temp float32) ai.AIProvider
}

// EnsembleManager runs multi-trajectory reasoning and synthesizes results.
type EnsembleManager struct {
	provider  ai.AIProvider
	spark     *spark.SPARK  // nil-safe; for ObserveEnsemble scoring
	sense     SENSEProvider // nil-safe; for LogSREEnsemble
	consensus *ConsensusOrchestrator
	logger    *slog.Logger
	Enabled   bool
}

func newEnsembleManager(provider ai.AIProvider, sparkInst *spark.SPARK,
	sense SENSEProvider, logger *slog.Logger) *EnsembleManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnsembleManager{
		provider:  provider,
		spark:     sparkInst,
		sense:     sense,
		consensus: newConsensusOrchestrator(provider),
		logger:    logger,
		Enabled:   false,
	}
}

// ShouldRun returns true for WorkflowAnalytical/WorkflowAgentic route decisions,
// or if m.Enabled == true.
func (m *EnsembleManager) ShouldRun(route adaptive.RouteDecision) bool {
	if m.Enabled {
		return true
	}
	// Check if route is analytical or agentic
	routeStr := fmt.Sprintf("%v", route)
	return routeStr == "WorkflowAnalytical" || routeStr == "WorkflowAgentic"
}

// Run spawns 3 goroutines (temps: 0.3, 0.7, 0.9), collects and scores results,
// returns synthesized SynthesisResult. Returns nil on all-traces-failed.
func (m *EnsembleManager) Run(ctx context.Context, history *ai.ConversationHistory) (*subagents.SynthesisResult, error) {
	if m.provider == nil || history == nil {
		return nil, fmt.Errorf("ensemble: provider or history nil")
	}

	type traceWork struct {
		label string
		temp  float32
	}

	traces := []traceWork{
		{label: "cold", temp: 0.3},
		{label: "warm", temp: 0.7},
		{label: "hot", temp: 0.9},
	}

	resultsCh := make(chan TrajectoryResult, len(traces))
	var wg sync.WaitGroup

	for _, t := range traces {
		wg.Add(1)
		go func(tr traceWork) {
			defer wg.Done()
			result := m.runTrace(ctx, tr.label, tr.temp, history)
			resultsCh <- result
		}(t)
	}

	wg.Wait()
	close(resultsCh)

	var results []TrajectoryResult
	for r := range resultsCh {
		results = append(results, r)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("ensemble: all traces failed")
	}

	// Score all results via SPARK if available
	if m.spark != nil {
		outputs := make([]string, len(results))
		for i, r := range results {
			outputs[i] = r.Output
		}
		scores := m.spark.ObserveEnsemble(outputs)
		if scores != nil && len(scores) == len(results) {
			for i, s := range scores {
				results[i].Score = s
			}
		}
	}

	// Synthesize
	sr, err := m.consensus.Merge(results)
	if err != nil {
		return nil, err
	}

	if m.sense != nil {
		conflictCount := len(sr.Conflicts)
		confidence := sr.Confidence
		if confidence == 0 {
			confidence = 0.75
		}
		m.sense.LogSREEnsemble(conflictCount, confidence)
	}

	return sr, nil
}

// runTrace: checks TemperatureConfigurable; falls back to plain GenerateWithHistory.
func (m *EnsembleManager) runTrace(ctx context.Context, label string, temp float32,
	history *ai.ConversationHistory) TrajectoryResult {

	start := time.Now()
	provider := m.provider

	// Try to use temperature variant if supported
	if tc, ok := m.provider.(TemperatureConfigurable); ok {
		provider = tc.WithTemperature(temp)
	}

	output, err := provider.GenerateWithHistory(ctx, history)
	elapsed := time.Since(start)

	return TrajectoryResult{
		Label:   label,
		Output:  output,
		Score:   0, // Will be set by SPARK if available
		Elapsed: elapsed,
		Err:     err,
	}
}

// ── ConsensusOrchestrator ─────────────────────────────────────────────────────

// ConsensusOrchestrator wraps subagents.Synthesizer with score-weighted ordering.
type ConsensusOrchestrator struct {
	synthesizer *subagents.Synthesizer
}

func newConsensusOrchestrator(provider ai.AIProvider) *ConsensusOrchestrator {
	var synth *subagents.Synthesizer
	if provider != nil {
		synth = subagents.NewSynthesizer(provider)
	}
	return &ConsensusOrchestrator{
		synthesizer: synth,
	}
}

// Merge converts scored TrajectoryResults to SourcedResults (sorted by Score desc),
// then calls synthesizer.Synthesize(). Falls back to heuristic on provider=nil.
func (co *ConsensusOrchestrator) Merge(results []TrajectoryResult) (*subagents.SynthesisResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("merge: no results")
	}

	// Filter out failed traces
	var validResults []TrajectoryResult
	for _, r := range results {
		if r.Err == nil && r.Output != "" {
			validResults = append(validResults, r)
		}
	}

	if len(validResults) == 0 {
		return nil, fmt.Errorf("merge: all traces failed")
	}

	// Sort by score descending
	sort.Slice(validResults, func(i, j int) bool {
		return validResults[i].Score > validResults[j].Score
	})

	if co.synthesizer == nil {
		// Fallback: heuristic — return the highest-scoring result
		best := validResults[0]
		return &subagents.SynthesisResult{
			Consensus:  best.Output,
			Confidence: best.Score,
			Sources:    []string{best.Label},
		}, nil
	}

	// Convert to SourcedResults
	sourceResults := make([]subagents.SourcedResult, len(validResults))
	for i, r := range validResults {
		sourceResults[i] = subagents.SourcedResult{
			Label:  r.Label,
			Output: r.Output,
		}
	}

	return co.synthesizer.Synthesize(context.Background(), sourceResults)
}
