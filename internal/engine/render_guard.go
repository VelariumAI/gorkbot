package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/governance"
)

func (o *Orchestrator) LastRenderGuardDecision() *governance.RendererGuardDecision {
	o.lastRenderGuardMu.Lock()
	defer o.lastRenderGuardMu.Unlock()
	if o.lastRenderGuardDecision == nil {
		return nil
	}
	cp := *o.lastRenderGuardDecision
	cp.Issues = append([]string(nil), cp.Issues...)
	cp.AcceptedClaimIDs = append([]string(nil), cp.AcceptedClaimIDs...)
	cp.RejectedClaimIDs = append([]string(nil), cp.RejectedClaimIDs...)
	return &cp
}

func (o *Orchestrator) applyFinalAnswerGuard(ctx context.Context, answer string, metadata map[string]any) string {
	if o.Governor == nil {
		o.setRenderGuardDecision(nil)
		return answer
	}

	start := time.Now()
	decision := o.Governor.VerifyFinalAnswer(ctx, governance.FinalAnswerVerificationInput{
		AnswerText: answer,
		Metadata:   metadata,
	})
	o.setRenderGuardDecision(&decision)
	o.logRenderGuardDecision(decision, time.Since(start))

	if !renderGuardIsEnforced(decision, o.Governor.Policy.Mode, metadata) {
		return answer
	}

	if decision.Valid {
		return "[VERIFIED — renderer guard passed]\n" + answer
	}

	if decision.FinalStatus == governance.RENDER_POLICY_BLOCKED {
		return fmt.Sprintf(
			"I cannot present this as verified because the renderer guard rejected it:\nreason=%s\nissues=%s",
			decision.ReasonCode,
			shortIssues(decision.Issues),
		)
	}

	return fmt.Sprintf(
		"[UNVERIFIED — renderer guard did not validate this answer]\nreason=%s\nissues=%s\n\n%s",
		decision.ReasonCode,
		shortIssues(decision.Issues),
		answer,
	)
}

func (o *Orchestrator) renderGuardStreamingNotice(ctx context.Context, answer string, metadata map[string]any, cb StreamCallback) {
	if o.Governor == nil {
		o.setRenderGuardDecision(nil)
		return
	}
	start := time.Now()
	decision := o.Governor.VerifyFinalAnswer(ctx, governance.FinalAnswerVerificationInput{
		AnswerText: answer,
		Metadata:   metadata,
	})
	o.setRenderGuardDecision(&decision)
	o.logRenderGuardDecision(decision, time.Since(start))
	if cb == nil || !renderGuardIsEnforced(decision, o.Governor.Policy.Mode, metadata) {
		return
	}

	if decision.Valid {
		cb("\n\n[VERIFIED — renderer guard passed]\n")
		return
	}
	if decision.FinalStatus == governance.RENDER_POLICY_BLOCKED {
		cb(fmt.Sprintf(
			"\n\nRenderer guard blocked verified rendering:\n%s\n%s\n",
			decision.ReasonCode,
			shortIssues(decision.Issues),
		))
		return
	}
	cb(fmt.Sprintf(
		"\n\n[UNVERIFIED — renderer guard did not validate this answer]\nreason=%s\nissues=%s\n",
		decision.ReasonCode,
		shortIssues(decision.Issues),
	))
}

func (o *Orchestrator) setRenderGuardDecision(decision *governance.RendererGuardDecision) {
	o.lastRenderGuardMu.Lock()
	defer o.lastRenderGuardMu.Unlock()
	o.lastRenderGuardDecision = decision
}

func (o *Orchestrator) logRenderGuardDecision(decision governance.RendererGuardDecision, duration time.Duration) {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("renderer_guard_decision",
		"answer_id", decision.AnswerID,
		"governance_mode", o.Governor.Policy.Mode,
		"valid", decision.Valid,
		"final_status", decision.FinalStatus,
		"reason_code", decision.ReasonCode,
		"claim_count", decision.ClaimCount,
		"accepted_claim_count", len(decision.AcceptedClaimIDs),
		"rejected_claim_count", len(decision.RejectedClaimIDs),
		"duration_ms", duration.Milliseconds(),
		"issues", shortIssues(decision.Issues),
	)
}

func renderGuardIsEnforced(decision governance.RendererGuardDecision, mode governance.Mode, metadata map[string]any) bool {
	if mode == governance.GOVERNANCE_CORRECTNESS {
		return true
	}
	if !metadataCorrectnessRequested(metadata) {
		return false
	}
	return decision.ReasonCode != governance.RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE
}

func metadataCorrectnessRequested(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	raw, ok := metadata["correctness_requested"]
	if !ok {
		return false
	}
	switch t := raw.(type) {
	case bool:
		return t
	case string:
		v := strings.ToLower(strings.TrimSpace(t))
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}

func shortIssues(issues []string) string {
	if len(issues) == 0 {
		return ""
	}
	const maxIssues = 3
	const maxLen = 200
	parts := make([]string, 0, maxIssues)
	for i, issue := range issues {
		if i >= maxIssues {
			break
		}
		parts = append(parts, strings.TrimSpace(issue))
	}
	out := strings.Join(parts, "; ")
	if len(issues) > maxIssues {
		out += fmt.Sprintf(" (+%d more)", len(issues)-maxIssues)
	}
	if len(out) > maxLen {
		return out[:maxLen] + "..."
	}
	return out
}
