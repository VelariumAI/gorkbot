package selfimprove

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/internal/events"
)

// errPipelineAbort is a sentinel error returned when a pipeline stage decides to
// gracefully abort (e.g., no SPARK state, no candidate selected). It is NOT
// treated as a fatal error by Run().
var errPipelineAbort = errors.New("pipeline abort: no action to take")

// PipelineContext carries mutable state across all 7 stages.
type PipelineContext struct {
	CycleID   string                // short UUID prefix (8 chars)
	CorrID    string                // full UUID for event correlation
	Mode      EmotionalMode         // computed emotional mode
	Signals   *SignalSnapshot       // gathered signals
	Candidate *ImprovementCandidate // selected candidate
	PreScore  float64               // SPARK drive score before execution
	PostScore float64               // SPARK drive score after execution
	Accepted  bool                  // whether improvement was accepted
	ToolOut   string                // tool execution output
	ToolErr   error                 // tool execution error
}

// SIPipeline is the 7-stage unified self-improvement pipeline that replaces
// the implicit 6-step logic in Driver.executeCycle().
type SIPipeline struct {
	spark    SPARKFacade
	freeWill FreeWillFacade
	harness  HarnessFacade
	research ResearchFacade
	registry ToolRegistryFacade
	notify   NotifyFacade
	obs      ObservabilityFacade

	bus      *events.Bus
	rollback RollbackStoreFacade

	planner   *ImprovementPlanner
	motivator *Motivator

	rollbackThreshold float64 // default 0.05 (5% drive score drop tolerance)
	logger            *slog.Logger
	phaseFn           func(string) // phase hook set by Driver for TUI refresh
}

// NewSIPipeline constructs a new 7-stage pipeline with shared planner and motivator.
func NewSIPipeline(
	spark SPARKFacade,
	fw FreeWillFacade,
	h HarnessFacade,
	res ResearchFacade,
	reg ToolRegistryFacade,
	n NotifyFacade,
	obs ObservabilityFacade,
	bus *events.Bus,
	rollback RollbackStoreFacade,
	logger *slog.Logger,
) *SIPipeline {
	if logger == nil {
		logger = slog.Default()
	}

	return &SIPipeline{
		spark:             spark,
		freeWill:          fw,
		harness:           h,
		research:          res,
		registry:          reg,
		notify:            n,
		obs:               obs,
		bus:               bus,
		rollback:          rollback,
		planner:           NewImprovementPlanner(),
		motivator:         NewMotivator(0.15),
		rollbackThreshold: 0.05,
		logger:            logger,
	}
}

// SetPhaseFunc wires a phase callback for TUI updates (called by Driver.SetPipeline).
func (p *SIPipeline) SetPhaseFunc(fn func(string)) {
	p.phaseFn = fn
}

// Run executes all 7 stages in order. Returns (true, nil) if accepted,
// (false, nil) if aborted gracefully, or (false, error) on fatal failure.
func (p *SIPipeline) Run(ctx context.Context) (bool, error) {
	pCtx := &PipelineContext{
		CycleID: uuid.New().String()[:8],
		CorrID:  uuid.New().String(),
	}

	stages := []func(context.Context, *PipelineContext) error{
		p.stageDetect,
		p.stageHypothesise,
		p.stagePropose,
		p.stageSandbox,
		p.stageScore,
		p.stageGate,
		p.stagePersist,
	}

	for _, stage := range stages {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := stage(ctx, pCtx); err != nil {
			if errors.Is(err, errPipelineAbort) {
				return false, nil // graceful abort
			}
			return false, err // fatal error
		}
	}

	return pCtx.Accepted, nil
}

// ─── 7-Stage Implementations ─────────────────────────────────────────────────

// stageDetect gathers SPARK state, computes emotional mode, and records cycle start.
// Returns errPipelineAbort if SPARK state is nil or drive score is too low in Calm mode.
func (p *SIPipeline) stageDetect(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("detecting")

	// Gather SPARK state
	sparkState := p.spark.GetLastState()
	if sparkState == nil {
		p.logger.Debug("stageDetect: no SPARK state, aborting")
		return errPipelineAbort
	}

	// Build signals snapshot
	signals := &SignalSnapshot{
		SPARKDriveScore:          sparkState.DriveScore,
		SPARKActiveDirectives:    sparkState.ActiveDirectives,
		SPARKIDLDebt:             sparkState.IDLDebt,
		HarnessFailing:           p.harness.FailingCount(),
		HarnessTotal:             p.harness.TotalCount(),
		FreeWillProposalsPending: len(p.freeWill.GetPendingProposals()),
		ResearchBufferedDocs:     p.research.BufferedCount(),
	}

	// Update motivator, get mode
	mode := p.motivator.Update(signals)

	// Abort if in Calm mode and drive is too low
	if mode == ModeCalm && sparkState.DriveScore < 0.1 {
		p.logger.Debug("stageDetect: calm mode with low drive, aborting",
			"drive_score", sparkState.DriveScore)
		return errPipelineAbort
	}

	// Store in context
	pCtx.Mode = mode
	pCtx.Signals = signals

	// Record cycle start
	if p.obs != nil {
		p.obs.RecordSICycleStart()
	}

	p.logger.Debug("stageDetect: complete", "cycle_id", pCtx.CycleID, "mode", mode)
	return nil
}

// stageHypothesise feeds SPARK observation to FreeWill, then selects best candidate.
// Returns errPipelineAbort if planner returns nil candidate.
func (p *SIPipeline) stageHypothesise(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("selecting")

	// Feed SPARK state to FreeWill
	sparkState := p.spark.GetLastState()
	if sparkState != nil {
		obs := FreeWillObsInput{
			Context: fmt.Sprintf("SPARK drive_score=%.2f directives=%d idl_debt=%d",
				sparkState.DriveScore, sparkState.ActiveDirectives, sparkState.IDLDebt),
			ToolName:   "spark_daemon",
			Outcome:    "observation",
			Latency:    0,
			Confidence: 0.85,
		}
		_ = p.freeWill.AddObservation(ctx, obs)
	}

	// Gather candidates and select best.
	// SPARK facade currently exposes directive count only; create stable labels
	// so planner weighting can still account for active directive pressure.
	sparkDirs := deriveDirectiveHints(sparkState)
	fwProps := p.freeWill.GetPendingProposals()
	harnessInfo := &HarnessFailureInfo{
		FailingCount: p.harness.FailingCount(),
		TotalCount:   p.harness.TotalCount(),
	}
	researchDocs := p.research.BufferedCount()

	candidate := p.planner.Select(pCtx.Mode, sparkDirs, fwProps, harnessInfo, researchDocs)
	if candidate == nil {
		p.logger.Debug("stageHypothesise: no candidate selected, aborting")
		return errPipelineAbort
	}

	pCtx.Candidate = candidate
	p.logger.Debug("stageHypothesise: candidate selected", "target", candidate.Target, "source", candidate.Source)
	return nil
}

func deriveDirectiveHints(s *SPARKStateSnapshot) []string {
	if s == nil || s.ActiveDirectives <= 0 {
		return nil
	}
	hints := make([]string, 0, s.ActiveDirectives)
	for i := 0; i < s.ActiveDirectives; i++ {
		hints = append(hints, fmt.Sprintf("spark_directive_%02d", i+1))
	}
	return hints
}

// stagePropose records pre-execution score, publishes proposal event, and logs metric.
func (p *SIPipeline) stagePropose(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("proposing")

	// Record pre-execution score
	sparkState := p.spark.GetLastState()
	if sparkState != nil {
		pCtx.PreScore = sparkState.DriveScore
	}

	// Publish proposal event to bus
	if p.bus != nil {
		evt := &events.ImprovementProposalEvent{
			BaseEvent:  events.NewBaseEventWithID(pCtx.CorrID),
			ProposalID: pCtx.CycleID,
			Target:     pCtx.Candidate.Target,
			Type:       sourceToType(pCtx.Candidate.Source),
			Confidence: int(pCtx.Candidate.BaseScore * 100),
			RiskLevel:  "low",
			Details:    fmt.Sprintf("Pipeline proposal: %s via %s", pCtx.Candidate.Target, pCtx.Candidate.Source.String()),
		}
		p.bus.Publish(ctx, evt)
	}

	// Record metric
	if p.obs != nil {
		p.obs.RecordSIProposal()
	}

	p.logger.Debug("stagePropose: event published", "cycle_id", pCtx.CycleID, "target", pCtx.Candidate.Target)
	return nil
}

// stageSandbox creates a rollback snapshot and executes the candidate tool.
func (p *SIPipeline) stageSandbox(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("executing:" + pCtx.Candidate.Target)

	// Snapshot for rollback
	if p.rollback != nil {
		if err := p.rollback.Snapshot(pCtx.CycleID, nil); err != nil {
			p.logger.Warn("stageSandbox: snapshot failed", "cycle_id", pCtx.CycleID, "error", err)
			// Non-fatal; continue without rollback capability
		}
	}

	// Execute tool with latency tracking
	execStart := time.Now()
	params := p.buildToolParams(pCtx.Candidate)
	toolName := p.toolNameForSource(pCtx.Candidate.Source)
	out, err := p.registry.ExecuteTool(ctx, toolName, params)
	execLatency := time.Since(execStart)

	pCtx.ToolOut = out
	pCtx.ToolErr = err

	// Record execution latency
	if p.obs != nil {
		p.obs.RecordSIExecutionLatency(execLatency)
	}

	// Record tool error if one occurred
	if err != nil {
		if p.obs != nil {
			p.obs.RecordSIToolError(toolName)
		}
		p.logger.Warn("stageSandbox: execution failed", "cycle_id", pCtx.CycleID, "target", pCtx.Candidate.Target, "error", err, "latency_ms", execLatency.Milliseconds())
	} else {
		p.logger.Debug("stageSandbox: execution succeeded", "cycle_id", pCtx.CycleID, "target", pCtx.Candidate.Target, "latency_ms", execLatency.Milliseconds())
	}

	return nil
}

// stageScore queries SPARK state post-execution and computes score delta.
func (p *SIPipeline) stageScore(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("scoring")

	// Re-query SPARK state
	sparkState := p.spark.GetLastState()
	if sparkState != nil {
		pCtx.PostScore = sparkState.DriveScore
	} else {
		pCtx.PostScore = pCtx.PreScore // no change if no state
	}

	delta := pCtx.PostScore - pCtx.PreScore

	// Record score delta
	if p.obs != nil {
		p.obs.RecordSIScoreDelta(delta)
	}

	p.logger.Debug("stageScore: score computed", "cycle_id", pCtx.CycleID,
		"pre_score", pCtx.PreScore, "post_score", pCtx.PostScore, "delta", delta)

	return nil
}

// stageGate decides whether to accept or reject based on score delta and tool error.
// Publishes RollbackEvent if rejected.
func (p *SIPipeline) stageGate(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("gating")

	delta := pCtx.PostScore - pCtx.PreScore
	accept := (delta >= -p.rollbackThreshold) && (pCtx.ToolErr == nil)

	if accept {
		pCtx.Accepted = true
		if p.obs != nil {
			p.obs.RecordSIAccepted()
		}
		p.logger.Debug("stageGate: proposal accepted", "cycle_id", pCtx.CycleID, "delta", delta)
	} else {
		// Build rejection reason
		reason := fmt.Sprintf("score_delta=%.3f threshold=%.3f tool_err=%v",
			delta, p.rollbackThreshold, pCtx.ToolErr != nil)

		// Publish rollback event
		if p.bus != nil {
			evt := &events.RollbackEvent{
				BaseEvent:  events.NewBaseEventWithID(pCtx.CorrID),
				ProposalID: pCtx.CycleID,
				Reason:     reason,
			}
			p.bus.Publish(ctx, evt)
		}

		// Record rejection reason and rollback
		if p.obs != nil {
			p.obs.RecordSIRolledBack()
			p.obs.RecordSIGateRejectionReason(reason)
		}

		p.logger.Debug("stageGate: proposal rejected", "cycle_id", pCtx.CycleID, "delta", delta, "tool_err", pCtx.ToolErr, "reason", reason)
	}

	return nil
}

// stagePersist applies the rollback decision and records the cycle outcome.
func (p *SIPipeline) stagePersist(ctx context.Context, pCtx *PipelineContext) error {
	p.setPhase("") // clear phase

	// Apply rollback decision
	if p.rollback != nil {
		if pCtx.Accepted {
			if err := p.rollback.Discard(pCtx.CycleID); err != nil {
				p.logger.Warn("stagePersist: discard failed", "cycle_id", pCtx.CycleID, "error", err)
			}
		} else {
			// Record rollback latency
			rollbackStart := time.Now()
			if err := p.rollback.Rollback(ctx, pCtx.CycleID); err != nil {
				p.logger.Warn("stagePersist: rollback failed", "cycle_id", pCtx.CycleID, "error", err)
			}
			rollbackLatency := time.Since(rollbackStart)

			if p.obs != nil {
				p.obs.RecordSIRollbackLatency(rollbackLatency)
			}
			p.logger.Debug("stagePersist: rollback completed", "cycle_id", pCtx.CycleID, "latency_ms", rollbackLatency.Milliseconds())
		}
	}

	// Send notification
	outcome := "rejected"
	if pCtx.Accepted {
		outcome = "accepted"
	}
	if p.notify != nil {
		msg := fmt.Sprintf("🔄 %s: %s %s (%s)",
			pCtx.CycleID, pCtx.Candidate.Source.String(), pCtx.Candidate.Target, outcome)
		p.notify.Notify(msg)
	}

	p.logger.Debug("stagePersist: cycle complete", "cycle_id", pCtx.CycleID, "accepted", pCtx.Accepted)
	return nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// sourceToType maps a SignalSource to an event type string.
func sourceToType(s SignalSource) string {
	switch s {
	case SourceSPARK:
		return "directive_execution"
	case SourceFreeWill:
		return "parameter_tune"
	case SourceHarness:
		return "workflow_optimization"
	case SourceResearch:
		return "code_change"
	default:
		return "unknown"
	}
}

// toolNameForSource maps a source to a tool name for execution.
func (p *SIPipeline) toolNameForSource(s SignalSource) string {
	switch s {
	case SourceSPARK:
		return "harness_boot"
	case SourceFreeWill:
		return "harness_select"
	case SourceHarness:
		return "harness_complete"
	case SourceResearch:
		return "browser_search"
	default:
		return "unknown_tool"
	}
}

// buildToolParams constructs parameters for the candidate tool.
func (p *SIPipeline) buildToolParams(cand *ImprovementCandidate) map[string]interface{} {
	params := make(map[string]interface{})
	switch cand.Source {
	case SourceSPARK:
		params["feature"] = cand.Target
	case SourceFreeWill:
		params["feature_id"] = cand.Target
	case SourceHarness:
		params["feature_id"] = p.harness.ActiveFeatureID()
	case SourceResearch:
		params["query"] = cand.Target
	}
	return params
}

// setPhase updates the phase and calls the callback if wired.
func (p *SIPipeline) setPhase(phase string) {
	if p.phaseFn != nil {
		p.phaseFn(phase)
	}
}
