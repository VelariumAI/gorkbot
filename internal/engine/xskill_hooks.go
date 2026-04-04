package engine

// xskill_hooks.go — Orchestrator methods for the XSKILL continual-learning system.
//
// Phase 2 (Inference — called on the hot path, before each LLM call):
//   - prepareXSkillContext: inject XSKILL context header via UpsertSystemMessage
//     and reset per-task trajectory state.
//
// Phase 1 (Accumulation — called async, after each task completes):
//   - launchXSkillAccumulation: fire a goroutine to learn from the trajectory.
//   - appendXSkillStep: add a single tool-execution step to the in-memory log.
//
// Lifecycle:
//   - InitXSkill: called from main.go after InitIntelligence.
//   - UpgradeXSkillEmbedder: called from initEmbedder goroutine when the local
//     llama.cpp model finishes loading.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/velariumai/gorkbot/internal/xskill"
	"github.com/velariumai/gorkbot/pkg/embeddings"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// ──────────────────────────────────────────────────────────────────────────────
// Lifecycle
// ──────────────────────────────────────────────────────────────────────────────

// InitXSkill initialises the XSKILL KnowledgeBase and InferenceEngine.
// configDir is used for the knowledge-base directory (configDir/xskill_kb/).
//
// Returns true when both components are ready.
// Returns false when no embedder is available; XSkillKB/XSkillEngine stay nil
// and the rest of Gorkbot runs normally without XSKILL enrichment.
func (o *Orchestrator) InitXSkill(configDir string) bool {
	emb := pickXSkillEmbedder(o.Logger)
	if emb == nil {
		// No embedder chain available — XSKILL disabled entirely.
		return false
	}

	primary := o.Primary()
	if primary == nil {
		// Primary provider required for XSKILL initialization
		o.Logger.Warn("XSKILL: primary provider not available, disabling")
		return false
	}
	provider := &mutableProvider{
		aiProvider: primary,
		embedder:   emb,
	}

	kbDir := filepath.Join(configDir, "xskill_kb")
	kb, err := xskill.NewKnowledgeBase(kbDir, provider)
	if err != nil {
		o.Logger.Warn("XSKILL: KnowledgeBase init failed", "error", err)
		return false
	}

	eng, err := xskill.NewInferenceEngine(kb, provider)
	if err != nil {
		o.Logger.Warn("XSKILL: InferenceEngine init failed", "error", err)
		return false
	}

	o.XSkillKB = kb
	o.XSkillEngine = eng
	o.xskillProvider = provider
	o.Logger.Info("XSKILL: continual-learning system ready", "kb_dir", kbDir)
	return true
}

// UpgradeXSkillEmbedder replaces the embedder in the XSKILL provider.
// Call from the background initEmbedder goroutine once the local model loads.
// No-op when XSKILL was not initialized.
func (o *Orchestrator) UpgradeXSkillEmbedder(e embeddings.Embedder) {
	if o.xskillProvider != nil && e != nil {
		o.xskillProvider.UpgradeEmbedder(e)
		o.Logger.Info("XSKILL: embedder upgraded", "embedder", e.Name())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2: context injection (hot path, called before AddUserMessage)
// ──────────────────────────────────────────────────────────────────────────────

// prepareXSkillContext runs Phase 2 of the XSKILL pipeline:
//  1. Classifies the skill domain from prompt.
//  2. Calls InferenceEngine.PrepareContext to build the injection header.
//  3. Injects the header into the conversation as a pinned system message.
//  4. Resets per-task trajectory state (steps, prompt, start time).
//
// No-op when XSkillEngine is nil (XSKILL disabled).
func (o *Orchestrator) prepareXSkillContext(prompt string) {
	if o.XSkillEngine == nil {
		return
	}
	skillName := classifySkillDomain(prompt)
	if header, err := o.XSkillEngine.PrepareContext(prompt, skillName); err == nil && header != "" {
		const tag = "[XSKILL_CONTEXT]"
		o.ConversationHistory.UpsertSystemMessage(tag, tag+"\n"+header)
		o.Logger.Info("XSKILL: context injected", "skill", skillName, "bytes", len(header))
	}
	// Reset per-task trajectory state so Phase 1 starts clean.
	o.xskillMu.Lock()
	o.xskillSteps = o.xskillSteps[:0]
	o.xskillPrompt = prompt
	o.xskillSkillName = skillName
	o.xskillTaskStart = time.Now()
	o.xskillMu.Unlock()
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1: trajectory collection + async accumulation
// ──────────────────────────────────────────────────────────────────────────────

// appendXSkillStep records one tool execution as a TrajectoryStep.
// Called from streaming.go immediately after every ExecuteTool call.
// No-op when XSkillKB is nil.
func (o *Orchestrator) appendXSkillStep(req tools.ToolRequest, result *tools.ToolResult, execErr error, startTime time.Time) {
	if o.XSkillKB == nil {
		return
	}
	step := xskill.TrajectoryStep{
		ToolName:   req.ToolName,
		DurationMS: time.Since(startTime).Milliseconds(),
	}
	if len(req.Parameters) > 0 {
		if b, merr := json.Marshal(req.Parameters); merr == nil {
			step.Parameters = string(b)
		}
	}
	if execErr != nil {
		step.Error = execErr.Error()
	} else if result != nil && !result.Success {
		step.Error = result.Error
	} else if result != nil {
		out := result.Output
		if len(out) > 512 {
			out = out[:512]
		}
		step.Output = out
	}
	o.xskillMu.Lock()
	step.StepIndex = len(o.xskillSteps)
	o.xskillSteps = append(o.xskillSteps, step)
	o.xskillMu.Unlock()
}

// launchXSkillAccumulation fires the Phase 1 accumulation goroutine.
// Call after RalphLoop.Commit() at the end of a successful task execution.
// No-op when XSkillKB is nil or no steps were recorded.
func (o *Orchestrator) launchXSkillAccumulation() {
	if o.XSkillKB == nil {
		return
	}
	o.xskillMu.Lock()
	if len(o.xskillSteps) == 0 || o.xskillPrompt == "" {
		o.xskillMu.Unlock()
		return
	}
	// Snapshot and clear — next task starts fresh.
	steps := make([]xskill.TrajectoryStep, len(o.xskillSteps))
	copy(steps, o.xskillSteps)
	taskPrompt := o.xskillPrompt
	skillName := o.xskillSkillName
	taskStart := o.xskillTaskStart
	o.xskillMu.Unlock()

	taskID := o.SessionID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().UnixMilli())
	}
	traj := xskill.Trajectory{
		TaskID:      taskID,
		Question:    taskPrompt,
		Steps:       steps,
		StartedAt:   taskStart,
		CompletedAt: time.Now(),
	}

	kb := o.XSkillKB
	go func(t xskill.Trajectory, sn string) {
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("XSKILL: accumulation panic recovered", "recover", r)
			}
		}()
		if err := kb.Accumulate(t, sn); err != nil {
			slog.Default().Warn("XSKILL: accumulation error", "error", err)
		}
	}(traj, skillName)
}
