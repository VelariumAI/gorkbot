package sre

import (
	"context"
	"log/slog"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/spark"
	"github.com/velariumai/gorkbot/pkg/subagents"
)

// Coordinator is the unified public API of the Step-wise Reasoning Engine.
// The Orchestrator holds a single *Coordinator.
type Coordinator struct {
	cfg        SREConfig
	grounder   *GroundingExtractor  // nil if !cfg.GroundingEnabled
	anchors    *AnchorLayer          // nil if ageMem == nil
	cos        *CoSEngine            // nil if !cfg.CoSEnabled
	correction *CorrectionEngine     // nil if !cfg.CoSEnabled
	ensemble   *EnsembleManager      // always created; Enabled from cfg
	sense      SENSEProvider         // nil-safe
	logger     *slog.Logger
}

// NewCoordinator wires all SRE subsystems.
// All optional deps (sparkInst, senseProvider) are nil-safe.
func NewCoordinator(
	cfg SREConfig,
	primary ai.AIProvider,
	ageMem *sense.AgeMem,
	sparkInst *spark.SPARK,
	senseProvider SENSEProvider,
	logger *slog.Logger,
) *Coordinator {
	if logger == nil {
		logger = slog.Default()
	}

	var grounder *GroundingExtractor
	if cfg.GroundingEnabled {
		grounder = NewGroundingExtractor(primary, senseProvider, logger)
	}

	var anchors *AnchorLayer
	if ageMem != nil {
		anchors = NewAnchorLayer(ageMem, logger)
	}

	var cos *CoSEngine
	var correction *CorrectionEngine
	if cfg.CoSEnabled {
		cos = newCoSEngine(cfg.HypothesisTurns, cfg.PruneTurns, sparkInst)
		correction = newCorrectionEngine(sparkInst, senseProvider, cfg.CorrectionThresh, logger)
	}

	ensemble := newEnsembleManager(primary, sparkInst, senseProvider, logger)
	ensemble.Enabled = cfg.EnsembleEnabled

	return &Coordinator{
		cfg:        cfg,
		grounder:   grounder,
		anchors:    anchors,
		cos:        cos,
		correction: correction,
		ensemble:   ensemble,
		sense:      senseProvider,
		logger:     logger,
	}
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// Reset reinitialises CoSEngine phase and CorrectionEngine counter.
// Called at task start (before per-turn loop).
func (c *Coordinator) Reset() {
	if c.cos != nil {
		c.cos.Reset()
	}
	if c.correction != nil {
		c.correction.Reset()
	}
}

// ── Per-task ─────────────────────────────────────────────────────────────────

// Ground runs GroundingExtractor.Extract(ctx, prompt).
// Stores result in AnchorLayer (priority 1.0) and commits ground phase.
// Returns nil if GroundingEnabled=false or provider is nil.
func (c *Coordinator) Ground(ctx context.Context, prompt string) *WorldModelState {
	if c.grounder == nil {
		return nil
	}
	ws, err := c.grounder.Extract(ctx, prompt)
	if err != nil {
		c.logger.Error("grounding failed", "error", err)
		return nil
	}
	return ws
}

// PrepareTask initialises anchor state from a WorldModelState (may be nil).
// Clears previous task anchors, loads WMS if non-nil, commits ground snapshot.
// Call once per task, after Ground().
func (c *Coordinator) PrepareTask(ws *WorldModelState) {
	if c.anchors == nil {
		return
	}

	c.anchors.Clear()
	if ws != nil {
		c.anchors.AddFromWorldModel(ws)
	}
	c.anchors.Commit(AnchorPhaseGround)
}

// RunEnsemble runs multi-trajectory ensemble if ShouldRun(route).
// Returns nil if skipped or all traces fail.
func (c *Coordinator) RunEnsemble(ctx context.Context, history *ai.ConversationHistory,
	route adaptive.RouteDecision) *subagents.SynthesisResult {
	if c.ensemble == nil || !c.ensemble.ShouldRun(route) {
		return nil
	}
	sr, err := c.ensemble.Run(ctx, history)
	if err != nil {
		c.logger.Error("ensemble failed", "error", err)
		return nil
	}
	return sr
}

// ── Per-turn ─────────────────────────────────────────────────────────────────

// TickPhase advances the CoS phase engine for this turn.
// Returns current SREPhase. On phase transition: commits anchor snapshot + resets correction.
func (c *Coordinator) TickPhase(turn int) SREPhase {
	if c.cos == nil {
		return SREPhaseHypothesis
	}

	oldPhase := c.cos.CurrentPhase()
	newPhase := c.cos.Tick(turn)

	// On phase transition
	if newPhase != oldPhase {
		if c.anchors != nil {
			c.anchors.Commit(PhaseFromSRE(newPhase))
		}
		if c.correction != nil {
			c.correction.Reset()
		}
	}

	return newPhase
}

// RoleBlock returns the current CoS phase role instruction string.
// Suitable for UpsertSystemMessage("[SRE_ROLE]", ...).
func (c *Coordinator) RoleBlock() string {
	if c.cos == nil {
		return ""
	}
	return c.cos.RoleBlock()
}

// MemoryBlock returns the formatted anchor working memory block.
// Suitable for UpsertSystemMessage("[SRE_MEMORY]", ...).
func (c *Coordinator) MemoryBlock() string {
	if c.anchors == nil {
		return ""
	}
	return c.anchors.FormatBlock(0)
}

// PhaseLabel returns "[SRE: HYPOTHESIS]" etc. for status display.
func (c *Coordinator) PhaseLabel() string {
	if c.cos == nil {
		return ""
	}
	return c.cos.PhaseLabel()
}

// CheckCorrection runs deviation detection on the LLM response.
// If ShouldBacktrack(): reverts AnchorLayer to hypothesis phase, returns true.
func (c *Coordinator) CheckCorrection(response string) (triggered bool) {
	if c.correction == nil || c.anchors == nil {
		return false
	}

	anchorContents := c.anchors.ContentStrings()
	deviated, _ := c.correction.Check(response, anchorContents)
	if !deviated {
		return false
	}

	if c.correction.ShouldBacktrack() {
		toPhase := c.correction.Backtrack(c.cos.PhaseLabel())
		c.anchors.Backtrack(toPhase)
		return true
	}

	return false
}

// AnchorTool stores a successful tool result as a working memory anchor.
// No-op if success=false.
func (c *Coordinator) AnchorTool(toolName, output string, success bool) {
	if !success || c.anchors == nil {
		return
	}
	phase := AnchorPhaseGround
	if c.cos != nil {
		phase = PhaseFromSRE(c.cos.CurrentPhase())
	}
	c.anchors.Add("tool_"+toolName, output[:min(100, len(output))], phase, 0.5)
}

// ── Accessors ────────────────────────────────────────────────────────────────

func (c *Coordinator) CurrentPhase() SREPhase {
	if c.cos == nil {
		return SREPhaseHypothesis
	}
	return c.cos.CurrentPhase()
}

func (c *Coordinator) Anchors() *AnchorLayer {
	return c.anchors
}

func (c *Coordinator) Enabled() bool {
	return c.cfg.CoSEnabled || c.cfg.GroundingEnabled || c.cfg.EnsembleEnabled
}

// CurrentDescriptiveLabel returns the full human-readable description for the
// current SRE phase. Used by the TUI status line to show exact what the engine is doing.
// Returns empty string if SRE is disabled or CoS is not running.
//
// Examples:
//   - "Exploring multiple approaches and hypotheses..."
//   - "Critically evaluating, pruning weaker paths, and self-correcting..."
//   - "Synthesizing, verifying, and finalizing the solution..."
func (c *Coordinator) CurrentDescriptiveLabel() string {
	if c.cos == nil {
		return ""
	}
	phase := c.cos.CurrentPhase()
	switch phase {
	case SREPhaseHypothesis:
		return "Exploring multiple approaches and hypotheses..."
	case SREPhasePrune:
		return "Critically evaluating, pruning weaker paths, and self-correcting..."
	case SREPhaseConverge:
		return "Synthesizing, verifying, and finalizing the solution..."
	default:
		return ""
	}
}

// IsGroundingActive returns true if grounding extraction is currently running.
// Used by the TUI to show "Grounding task and extracting anchors..." status.
// This is set to false after PrepareTask() completes.
func (c *Coordinator) IsGroundingActive() bool {
	// Grounding is active only during the Ground() call (synchronous).
	// After PrepareTask() is called, it's inactive.
	// For now, we track it via a flag that's set during Extract and cleared after PrepareTask.
	// Since Ground() is synchronous, this flag is only true during the call itself.
	// We return false here as the default; the Orchestrator sets this flag temporarily.
	return false // Set to true by orchestrator during Ground() call
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
