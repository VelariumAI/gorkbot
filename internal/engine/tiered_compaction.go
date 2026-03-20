// Package engine — tiered_compaction.go
//
// TieredCompactor enforces context limits before every LLM call with a
// two-stage tiered strategy ported from the build-your-own-openclaw tutorial
// (Step 05) but implemented in idiomatic Go:
//
//  Stage 1 — Soft trim (SoftThresholdPct, default 75%):
//    Walk the history and truncate any tool-result message whose content
//    exceeds MaxToolResultBytes (default 2 KB) down to a short summary
//    header.  Tool-call/tool-result pairs are kept in sync so the history
//    stays valid.
//
//  Stage 2 — Hard compress (HardThresholdPct, default 90%):
//    If the history is still over the hard threshold after Stage 1, invoke
//    the CompressionPipe synchronously (blocking) to SENSE-compress old
//    messages into a summary system message.
//
// This replaces the async-only "fire at 95%" approach, which could let the
// conversation momentarily exceed the provider limit.
//
// Wire into the orchestrator:
//
//	orch.TieredCompactor = engine.NewTieredCompactor(orch.ContextMgr, orch.CompressionPipe, orch.Logger)
//	// then inside the generation loop, before every Primary.Chat call:
//	if err := orch.TieredCompactor.Check(ctx, orch.ConversationHistory); err != nil {
//	    orch.Logger.Warn("tiered compaction failed", "err", err)
//	}
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
)

const (
	// defaultSoftThresholdPct triggers Stage-1 (tool-result truncation).
	defaultSoftThresholdPct = 0.75
	// defaultHardThresholdPct triggers Stage-2 (full SENSE compression).
	defaultHardThresholdPct = 0.90
	// defaultMaxToolResultBytes — tool results larger than this are trimmed.
	defaultMaxToolResultBytes = 2 * 1024
	// truncatedSuffix appended after trimming a tool result.
	truncatedSuffix = "\n[... output truncated by TieredCompactor — full result omitted to save context ...]"
)

// TieredCompactor enforces context limits before each LLM call using a
// two-stage approach: trim large tool results first, then SENSE-compress.
type TieredCompactor struct {
	contextMgr        *ContextManager
	pipe              *CompressionPipe // may be nil
	logger            *slog.Logger
	SoftThresholdPct  float64 // default 0.75
	HardThresholdPct  float64 // default 0.90
	MaxToolResultBytes int    // default 2048
}

// NewTieredCompactor creates a TieredCompactor. pipe may be nil (Stage-2
// compression will be skipped gracefully).
func NewTieredCompactor(cm *ContextManager, pipe *CompressionPipe, logger *slog.Logger) *TieredCompactor {
	return &TieredCompactor{
		contextMgr:        cm,
		pipe:              pipe,
		logger:            logger,
		SoftThresholdPct:  defaultSoftThresholdPct,
		HardThresholdPct:  defaultHardThresholdPct,
		MaxToolResultBytes: defaultMaxToolResultBytes,
	}
}

// Check runs the tiered compaction strategy before an LLM call.
// It is a no-op when the context window is below the soft threshold.
// Never returns a fatal error — compaction failures are logged and the
// conversation proceeds unchanged.
func (tc *TieredCompactor) Check(ctx context.Context, history *ai.ConversationHistory) error {
	if tc.contextMgr == nil {
		return nil
	}

	used := tc.contextMgr.UsedPct()

	// Fast path — nothing to do.
	if used < tc.SoftThresholdPct {
		return nil
	}

	tc.logger.Info("tiered compaction triggered",
		"used_pct", fmt.Sprintf("%.1f%%", used*100),
		"stage", stageLabel(used, tc.SoftThresholdPct, tc.HardThresholdPct),
	)

	// ── Stage 1: Truncate oversized tool results ──────────────────────────
	trimmed := tc.trimToolResults(history)
	if trimmed > 0 {
		tc.logger.Info("stage-1 tool-result trim", "messages_trimmed", trimmed)
	}

	// Re-check after trim — might be enough.
	usedAfterTrim := tc.estimateUsedPct(history)
	if usedAfterTrim < tc.HardThresholdPct {
		return nil
	}

	// ── Stage 2: Full SENSE compression ───────────────────────────────────
	if tc.pipe == nil {
		tc.logger.Warn("stage-2 compression skipped: CompressionPipe not wired")
		return nil
	}

	// force=true bypasses the pipe's ThresholdToks guard — we are already over
	// the hard threshold so we must compress regardless.
	err := tc.pipe.MaybeCompress(ctx, history, true)
	if err != nil {
		tc.logger.Warn("stage-2 SENSE compression failed", "err", err)
		return nil // non-fatal
	}

	tc.logger.Info("stage-2 SENSE compression complete",
		"tokens_now", history.EstimateTokens(),
	)
	return nil
}

// trimToolResults walks the history and truncates any role:"tool" message
// whose content exceeds MaxToolResultBytes. Returns the number of messages
// that were trimmed.
func (tc *TieredCompactor) trimToolResults(history *ai.ConversationHistory) int {
	msgs := history.GetMessages()
	trimCount := 0

	for i, msg := range msgs {
		if msg.Role != "tool" {
			continue
		}
		if len(msg.Content) <= tc.MaxToolResultBytes {
			continue
		}
		// Build a concise replacement: first MaxToolResultBytes of the original
		// plus a truncation notice.
		preview := msg.Content[:tc.MaxToolResultBytes]
		// Trim to the last newline so we don't cut mid-line.
		if nl := strings.LastIndexByte(preview, '\n'); nl > tc.MaxToolResultBytes/2 {
			preview = preview[:nl]
		}
		msgs[i].Content = preview + truncatedSuffix
		trimCount++
	}

	if trimCount > 0 {
		history.SetMessages(msgs)
	}
	return trimCount
}

// estimateUsedPct returns the estimated fraction of the context window used,
// based on the conversation history's own token estimate.
func (tc *TieredCompactor) estimateUsedPct(history *ai.ConversationHistory) float64 {
	if tc.contextMgr == nil || tc.contextMgr.MaxTokens() == 0 {
		return 0
	}
	return float64(history.EstimateTokens()) / float64(tc.contextMgr.MaxTokens())
}

func stageLabel(used, soft, hard float64) string {
	switch {
	case used >= hard:
		return "stage-2 (compress)"
	case used >= soft:
		return "stage-1 (trim)"
	default:
		return "none"
	}
}
