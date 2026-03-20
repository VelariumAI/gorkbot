package engine

// spark_hooks.go — Orchestrator methods for the SPARK autonomous reasoning daemon.
//
// Phase 2 (hot path, called before each LLM call):
//   - prepareSPARKContext: injects [SPARK_CONTEXT] header via UpsertSystemMessage.
//
// Phase 1 (async, called after each task completes):
//   - launchSPARKIntrospection: fires TriggerCycle() after RalphLoop.Commit().
//   - appendSPARKToolEvent: updates TII after every ExecuteTool call.
//
// Lifecycle:
//   - InitSPARK: called from main.go after InitXSkill.
//   - StartSPARK: called from main.go after InitSPARK.

import (
	"context"
	"path/filepath"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/spark"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// InitSPARK initialises the SPARK autonomous reasoning daemon.
// configDir is used for persisted state (configDir/spark/).
// Returns true when the daemon was successfully created.
func (o *Orchestrator) InitSPARK(configDir string) bool {
	cfg := spark.DefaultConfig(configDir)

	// Construct TraceAnalyzer using the SENSE trace directory layout.
	ta := sense.NewTraceAnalyzer(filepath.Join(configDir, "sense", "traces"))

	daemon := spark.New(cfg, o.LIE, ta, o.AgeMem, o.Primary, o.Logger)
	// Note: o.HITLGuard does not implement spark.HITLFacade (different interface);
	// SPARK operates without HITL gating unless a compatible facade is wired later.

	daemon.SetCallbacks(spark.DirectiveCallbacks{
		OnProviderFallback: func(r string) {
			o.Logger.Info("SPARK: provider fallback directive", "rationale", r)
		},
		OnPromptAmend: func(toolName, rationale string) {
			const tag = "[SPARK_DIRECTIVE]"
			var msg string
			if toolName == "" {
				msg = tag + "\n" + rationale
			} else {
				msg = tag + "\n[Tool: " + toolName + "] " + rationale
			}
			o.ConversationHistory.UpsertSystemMessage(tag, msg)
		},
		OnToolBan: func(toolName, rationale string) {
			o.Logger.Warn("SPARK: tool ban directive", "tool", toolName, "rationale", rationale)
		},
	})

	o.SPARK = daemon
	o.Logger.Info("SPARK: daemon initialised", "config_dir", configDir)
	return true
}

// StartSPARK starts the SPARK run loop.  No-op if SPARK is nil.
func (o *Orchestrator) StartSPARK(ctx context.Context) {
	if o.SPARK != nil {
		o.SPARK.Start(ctx)
	}
}

// prepareSPARKContext injects the [SPARK_CONTEXT] header before each LLM call.
// Called from streaming.go immediately after prepareXSkillContext.
// No-op when SPARK is nil.
func (o *Orchestrator) prepareSPARKContext() {
	if o.SPARK == nil {
		return
	}
	ctx := o.SPARK.PrepareContext()
	if ctx == "" {
		return
	}
	const tag = "[SPARK_CONTEXT]"
	o.ConversationHistory.UpsertSystemMessage(tag, tag+"\n"+ctx)
	o.Logger.Debug("SPARK: context injected", "bytes", len(ctx))
}

// appendSPARKToolEvent records a tool execution outcome in the TII.
// Called from streaming.go immediately after appendXSkillStep.
// No-op when SPARK is nil.
func (o *Orchestrator) appendSPARKToolEvent(req tools.ToolRequest, result *tools.ToolResult, execErr error, startTime time.Time) {
	if o.SPARK == nil {
		return
	}
	latencyMS := time.Since(startTime).Milliseconds()
	success := execErr == nil && result != nil && result.Success
	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	} else if result != nil && !result.Success {
		errMsg = result.Error
	}
	o.SPARK.AppendToolEvent(req.ToolName, success, latencyMS, errMsg)
}

// launchSPARKIntrospection triggers a SPARK cycle after RalphLoop.Commit().
// Called from streaming.go immediately after launchXSkillAccumulation.
// No-op when SPARK is nil.
func (o *Orchestrator) launchSPARKIntrospection() {
	if o.SPARK == nil {
		return
	}
	o.SPARK.TriggerCycle()
}
