package engine

// cci_integration.go — Wires the CCI (Codified Context Infrastructure) system
// into the Orchestrator lifecycle.
//
// Integration points:
//
//  1. InitEnhancements() → calls InitCCI() to boot the CCILayer.
//  2. ExecuteTaskWithStreaming() → on first message, BuildSystemContext(prompt)
//     is prepended to the system prompt (Tier 1 hot memory + optional Tier 2 specialist).
//  3. After workspace git checkpoint → RunDriftCheck(cwd) warns about stale Tier 3 specs.
//  4. mcp_context_get_subsystem returning "" → HandleGap() shifts to PLAN mode.

import (
	"context"
	"strings"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// InitCCI initializes the CCILayer and attaches it to the Orchestrator.
// Should be called from InitEnhancements() after configDir is known.
func (o *Orchestrator) InitCCI(configDir, cwd string) {
	layer := adaptive.NewCCILayer(configDir, cwd, o.Logger)

	// Wire the ARC trigger table as the specialist routing function.
	layer.TriggerFn = adaptive.MatchTrigger

	o.CCI = layer

	o.Logger.Info("CCI layer initialized",
		"specialists", len(layer.Specialists.List()),
		"docs", len(layer.ColdStore.ListSubsystems()),
	)
}

// BuildCCISystemContext constructs the full CCI system prompt prefix.
// This is injected BEFORE any other context when the conversation history is empty.
//
// Includes:
//   - Drift warnings from the Truth Sentry (if any stale docs detected)
//   - Tier 1 hot memory block (universal conventions + trigger table + subsystem pointers)
//   - Tier 2 specialist persona (if the trigger table routes the prompt to one)
func (o *Orchestrator) BuildCCISystemContext(prompt string) string {
	if o.CCI == nil {
		return ""
	}
	return o.CCI.BuildSystemContext(prompt)
}

// RunCCIDriftCheck runs the Truth Sentry drift detection independently.
// Returns a formatted warning string to inject into the system prompt,
// or "" if all specs are up-to-date.
func (o *Orchestrator) RunCCIDriftCheck() string {
	if o.CCI == nil {
		return ""
	}
	return o.CCI.RunDriftCheckDefault()
}

// HandleCCIGap is called when mcp_context_get_subsystem returns empty.
// Switches the ModeManager to PLAN mode and returns a user-facing notification.
func (o *Orchestrator) HandleCCIGap(subsystem string) string {
	if o.CCI == nil {
		return ""
	}
	var mm adaptive.ModeManagerIface
	if o.ModeManager != nil {
		mm = o.ModeManager
	}
	msg := o.CCI.HandleGap(subsystem, mm)
	// Also inject a system message so the AI knows about the mode shift.
	if o.ConversationHistory != nil {
		o.ConversationHistory.AddSystemMessage(
			"[CCI] Gap detected for subsystem \"" + subsystem + "\". " +
				"PLAN mode activated — use read-only tools to map the subsystem, " +
				"then call mcp_context_update_subsystem to document it, " +
				"then /mode normal to resume coding.")
	}
	return msg
}

// InjectCCIContextIntoRegistry injects the CCILayer into a context so
// mcp_context_* tools can access it during execution.
// Call this before passing ctx to Registry.Execute().
func (o *Orchestrator) InjectCCIContextIntoRegistry(ctx context.Context) context.Context {
	if o.CCI == nil {
		return ctx
	}
	return tools.WithCCILayer(ctx, o.CCI)
}

// GetCCIStatus returns a diagnostic string about CCI state.
func (o *Orchestrator) GetCCIStatus() string {
	if o.CCI == nil {
		return "CCI layer not initialized."
	}
	return o.CCI.GetStatus()
}

// cciPrefixForSystemMessage builds the CCI block to prepend to the system prompt.
// It merges the drift warnings and the full hot+specialist context into one string.
func (o *Orchestrator) cciPrefixForSystemMessage(prompt string) string {
	if o.CCI == nil {
		return ""
	}

	parts := []string{}

	// Drift check (Truth Sentry).
	driftWarn := o.RunCCIDriftCheck()
	if driftWarn != "" {
		parts = append(parts, driftWarn)
	}

	// Tier 1 + optional Tier 2.
	cciCtx := o.CCI.BuildSystemContext(prompt)
	if cciCtx != "" {
		parts = append(parts, cciCtx)
	}

	return strings.Join(parts, "\n\n")
}
