package engine

import (
	"context"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/sre"
)

// InitSRE wires all SRE components. Called from InitEnhancements after InitSPARK.
func (o *Orchestrator) InitSRE(cfg sre.SREConfig) {
	// Build the SRE Coordinator (all deps are nil-safe)
	o.SRE = sre.NewCoordinator(
		cfg,
		o.Primary,
		o.AgeMem,
		o.SPARK,
		o.SENSETracer, // satisfies sre.SENSEProvider via new LogSREXxx methods
		o.Logger,
	)
}

// prepareGrounding runs GroundingExtractor, anchors WorldModelState, injects system msg.
// Returns nil if SRE is nil or grounding disabled.
func (o *Orchestrator) prepareGrounding(ctx context.Context, prompt string) *sre.WorldModelState {
	if o.SRE == nil {
		return nil
	}
	ws := o.SRE.Ground(ctx, prompt)
	o.SRE.PrepareTask(ws) // anchors WMS, commits ground snapshot
	if ws != nil {
		o.ConversationHistory.UpsertSystemMessage("[SRE_GROUNDING]", ws.FormatBlock())
	}
	return ws
}

// runEnsembleIfNeeded runs ensemble for Analytical/Agentic, injects [SRE_ENSEMBLE].
func (o *Orchestrator) runEnsembleIfNeeded(ctx context.Context, route adaptive.RouteDecision) {
	if o.SRE == nil {
		return
	}
	sr := o.SRE.RunEnsemble(ctx, o.ConversationHistory, route)
	if sr != nil && sr.Consensus != "" {
		o.ConversationHistory.UpsertSystemMessage("[SRE_ENSEMBLE]", sr.Consensus)
	}
}

// prepareSREContext injects CoS phase role + anchor memory before each LLM call.
// Called before prepareSPARKContext in per-turn loop.
func (o *Orchestrator) prepareSREContext(turn int) {
	if o.SRE == nil {
		return
	}
	_ = o.SRE.TickPhase(turn) // phase is updated internally
	o.ConversationHistory.UpsertSystemMessage("[SRE_ROLE]", o.SRE.RoleBlock())
	o.ConversationHistory.UpsertSystemMessage("[SRE_MEMORY]", o.SRE.MemoryBlock())
}

// appendSRECorrectionCheck runs deviation detection; injects [SRE_BACKTRACK] if triggered.
// Returns true if backtrack was triggered.
func (o *Orchestrator) appendSRECorrectionCheck(response string) bool {
	if o.SRE == nil {
		return false
	}
	triggered := o.SRE.CheckCorrection(response)
	if triggered {
		o.ConversationHistory.UpsertSystemMessage("[SRE_BACKTRACK]", o.SRE.MemoryBlock())
	}
	return triggered
}

// anchorToolResult stores successful tool outputs in the working memory.
func (o *Orchestrator) anchorToolResult(toolName, output string, success bool) {
	if o.SRE == nil {
		return
	}
	o.SRE.AnchorTool(toolName, output, success)
}
