package governance

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/vcseclient"
)

const (
	RenderGuardUnavailableBlock     = "block"
	RenderGuardUnavailableDowngrade = "downgrade"
	RenderGuardUnavailableAudit     = "audit"
)

type FinalAnswerVerificationInput struct {
	AnswerText          string
	ClaimRefs           []AnswerClaimRef
	ClaimViews          []ValidatedClaimView
	UnsupportedSegments []string
	RenderMode          string
	Metadata            map[string]any
}

func (g *Governor) VerifyFinalAnswer(ctx context.Context, input FinalAnswerVerificationInput) RendererGuardDecision {
	mode := g.Policy.Mode
	requiresGuard := mode == GOVERNANCE_CORRECTNESS || metadataCorrectnessRequested(input.Metadata)
	auditObserve := mode == GOVERNANCE_AUDIT

	d := RendererGuardDecision{
		FinalStatus: RENDER_VALID,
		Valid:       true,
		ReasonCode:  RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE,
		Issues:      []string{},
	}

	if !requiresGuard && !auditObserve {
		return d
	}

	draft := BuildAnswerDraftFromClaimViews(input.AnswerText, input.ClaimViews, input.UnsupportedSegments)
	if len(input.ClaimRefs) > 0 {
		draft.ClaimRefs = append([]AnswerClaimRef(nil), input.ClaimRefs...)
	}
	d.AnswerID = draft.AnswerID
	d.ClaimCount = len(draft.ClaimRefs)

	if strings.TrimSpace(input.RenderMode) != "" {
		draft.RenderMode = strings.TrimSpace(input.RenderMode)
	}
	if g.RenderGuardPolicy.RenderMode != "" && strings.TrimSpace(input.RenderMode) == "" {
		draft.RenderMode = g.RenderGuardPolicy.RenderMode
	}

	if !isValidRenderMode(draft.RenderMode) {
		return RendererGuardDecision{
			AnswerID:    draft.AnswerID,
			FinalStatus: RENDER_INVALID,
			Valid:       false,
			ReasonCode:  INVALID_RENDER_MODE,
			Issues:      []string{"invalid render mode"},
			ClaimCount:  len(draft.ClaimRefs),
			RenderMode:  draft.RenderMode,
		}
	}

	d.RenderMode = draft.RenderMode

	if strings.TrimSpace(draft.RenderedText) == "" {
		return RendererGuardDecision{
			AnswerID:    draft.AnswerID,
			FinalStatus: RENDER_INVALID,
			Valid:       false,
			ReasonCode:  MISSING_RENDERED_TEXT,
			Issues:      []string{"missing rendered text"},
			ClaimCount:  len(draft.ClaimRefs),
			RenderMode:  draft.RenderMode,
		}
	}

	if requiresGuard && HasUnsupportedSegments(draft.UnsupportedSegments) {
		return RendererGuardDecision{
			AnswerID:    draft.AnswerID,
			FinalStatus: RENDER_EXCEEDS_VALIDATED_MATERIAL,
			Valid:       false,
			ReasonCode:  UNSUPPORTED_SEGMENT_PRESENT,
			Issues: append([]string{
				RENDER_EXCEEDS_VALIDATED_MATERIAL,
			}, draft.UnsupportedSegments...),
			ClaimCount: len(draft.ClaimRefs),
			RenderMode: draft.RenderMode,
		}
	}

	if requiresGuard && len(draft.ClaimRefs) == 0 {
		return RendererGuardDecision{
			AnswerID:    draft.AnswerID,
			FinalStatus: RENDER_NEEDS_CLAIM_MAP,
			Valid:       false,
			ReasonCode:  MISSING_CLAIM_REFS,
			Issues:      []string{"no claim refs provided for correctness mode"},
			ClaimCount:  0,
			RenderMode:  draft.RenderMode,
		}
	}

	if g.VCSE == nil {
		return g.renderGuardUnavailableDecision(draft, requiresGuard, RENDER_GUARD_UNAVAILABLE)
	}

	timeout := g.RenderGuardTimeout
	if timeout <= 0 {
		timeout = 750 * time.Millisecond
	}
	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := vcseclient.RenderVerifyRequest{
		Answer: draft,
		Claims: input.ClaimViews,
		Policy: &g.RenderGuardPolicy,
	}
	resp, err := g.VCSE.VerifyRenderedAnswer(verifyCtx, req)
	if err != nil {
		reason := RENDER_GUARD_UNAVAILABLE
		if errors.Is(err, vcseclient.ErrTimeout) {
			reason = RENDER_GUARD_TIMEOUT
		}
		return g.renderGuardUnavailableDecision(draft, requiresGuard, reason)
	}

	out := RendererGuardDecision{
		AnswerID:         fallbackString(resp.AnswerID, draft.AnswerID),
		FinalStatus:      fallbackString(resp.FinalStatus, ternary(resp.Valid, RENDER_VALID, RENDER_INVALID)),
		Valid:            resp.Valid,
		ReasonCode:       fallbackString(resp.ReasonCode, ternary(resp.Valid, RENDER_GUARD_PASSED, INVALID_RENDER_INPUT)),
		Issues:           append([]string(nil), resp.Issues...),
		ClaimCount:       fallbackInt(resp.ClaimCount, len(draft.ClaimRefs)),
		AcceptedClaimIDs: append([]string(nil), resp.AcceptedClaimIDs...),
		RejectedClaimIDs: append([]string(nil), resp.RejectedClaimIDs...),
		RenderMode:       fallbackString(resp.RenderMode, draft.RenderMode),
	}
	if out.Issues == nil {
		out.Issues = []string{}
	}

	if !requiresGuard && auditObserve {
		return out
	}
	return out
}

func (g *Governor) renderGuardUnavailableDecision(draft AnswerDraft, requiresGuard bool, reason string) RendererGuardDecision {
	action := strings.ToLower(strings.TrimSpace(g.RenderGuardOnUnavailable))
	if action == "" {
		if g.Policy.Mode == GOVERNANCE_AUDIT {
			action = RenderGuardUnavailableAudit
		} else {
			action = RenderGuardUnavailableDowngrade
		}
	}

	if !requiresGuard || action == RenderGuardUnavailableAudit {
		return RendererGuardDecision{
			AnswerID:    draft.AnswerID,
			FinalStatus: RENDER_VALID,
			Valid:       true,
			ReasonCode:  reason,
			Issues:      []string{reason},
			ClaimCount:  len(draft.ClaimRefs),
			RenderMode:  draft.RenderMode,
		}
	}

	finalStatus := RENDER_INVALID
	if action == RenderGuardUnavailableBlock {
		finalStatus = RENDER_POLICY_BLOCKED
	}

	return RendererGuardDecision{
		AnswerID:    draft.AnswerID,
		FinalStatus: finalStatus,
		Valid:       false,
		ReasonCode:  reason,
		Issues:      []string{reason},
		ClaimCount:  len(draft.ClaimRefs),
		RenderMode:  draft.RenderMode,
	}
}

func isValidRenderMode(mode string) bool {
	switch mode {
	case string(RENDER_MODE_CANONICAL_ONLY),
		string(RENDER_MODE_NORMALIZED_CANONICAL),
		string(RENDER_MODE_EXPLICIT_ALLOWED_RENDERING):
		return true
	default:
		return false
	}
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

func fallbackString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func fallbackInt(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
