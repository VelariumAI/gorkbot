package governance

import (
	"fmt"
	"strings"
	"time"
)

type RenderMode string

const (
	RENDER_MODE_CANONICAL_ONLY             RenderMode = "CANONICAL_ONLY"
	RENDER_MODE_NORMALIZED_CANONICAL       RenderMode = "NORMALIZED_CANONICAL"
	RENDER_MODE_EXPLICIT_ALLOWED_RENDERING RenderMode = "EXPLICIT_ALLOWED_RENDERING"
)

const (
	RENDER_VALID                      = "RENDER_VALID"
	RENDER_INVALID                    = "RENDER_INVALID"
	RENDER_NEEDS_CLAIM_MAP            = "RENDER_NEEDS_CLAIM_MAP"
	RENDER_EXCEEDS_VALIDATED_MATERIAL = "RENDER_EXCEEDS_VALIDATED_MATERIAL"
	RENDER_POLICY_BLOCKED             = "RENDER_POLICY_BLOCKED"
)

const (
	RENDER_GUARD_PASSED                       = "RENDER_GUARD_PASSED"
	MISSING_ANSWER_ID                         = "MISSING_ANSWER_ID"
	MISSING_RENDERED_TEXT                     = "MISSING_RENDERED_TEXT"
	MISSING_CLAIM_REFS                        = "MISSING_CLAIM_REFS"
	UNKNOWN_CLAIM_ID                          = "UNKNOWN_CLAIM_ID"
	CLAIM_STATUS_NOT_ALLOWED                  = "CLAIM_STATUS_NOT_ALLOWED"
	RENDERED_TEXT_NOT_CANONICAL               = "RENDERED_TEXT_NOT_CANONICAL"
	UNSUPPORTED_SEGMENT_PRESENT               = "UNSUPPORTED_SEGMENT_PRESENT"
	SOURCE_SPAN_MISMATCH                      = "SOURCE_SPAN_MISMATCH"
	INVALID_RENDER_MODE                       = "INVALID_RENDER_MODE"
	INVALID_RENDER_INPUT                      = "INVALID_RENDER_INPUT"
	RENDER_GUARD_UNAVAILABLE                  = "RENDER_GUARD_UNAVAILABLE"
	RENDER_GUARD_TIMEOUT                      = "RENDER_GUARD_TIMEOUT"
	RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE = "RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE"
)

type AnswerClaimRef struct {
	ClaimID       string   `json:"claim_id"`
	RenderedText  string   `json:"rendered_text"`
	SourceSpanIDs []string `json:"source_span_ids,omitempty"`
}

type AnswerDraft struct {
	AnswerID            string           `json:"answer_id"`
	RenderedText        string           `json:"rendered_text"`
	RenderMode          string           `json:"render_mode"`
	ClaimRefs           []AnswerClaimRef `json:"claim_refs"`
	UnsupportedSegments []string         `json:"unsupported_segments,omitempty"`
	Metadata            map[string]any   `json:"metadata,omitempty"`
}

type ValidatedClaimView struct {
	ClaimID           string   `json:"claim_id"`
	FinalStatus       string   `json:"final_status"`
	CanonicalText     string   `json:"canonical_text"`
	AllowedRenderings []string `json:"allowed_renderings,omitempty"`
	SourceSpanIDs     []string `json:"source_span_ids,omitempty"`
}

type RendererGuardPolicy struct {
	AllowedClaimStatuses []string `json:"allowed_claim_statuses,omitempty"`
	RenderMode           string   `json:"render_mode,omitempty"`
}

type RendererGuardDecision struct {
	AnswerID         string   `json:"answer_id"`
	FinalStatus      string   `json:"final_status"`
	Valid            bool     `json:"valid"`
	ReasonCode       string   `json:"reason_code"`
	Issues           []string `json:"issues,omitempty"`
	ClaimCount       int      `json:"claim_count"`
	AcceptedClaimIDs []string `json:"accepted_claim_ids,omitempty"`
	RejectedClaimIDs []string `json:"rejected_claim_ids,omitempty"`
	RenderMode       string   `json:"render_mode"`
}

func HasUnsupportedSegments(segments []string) bool {
	for _, s := range segments {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

func BuildAnswerDraftFromClaimViews(answerText string, views []ValidatedClaimView, unsupported []string) AnswerDraft {
	draft := AnswerDraft{
		AnswerID:            fmt.Sprintf("answer-%d", time.Now().UnixNano()),
		RenderedText:        answerText,
		RenderMode:          string(RENDER_MODE_CANONICAL_ONLY),
		ClaimRefs:           make([]AnswerClaimRef, 0, len(views)),
		UnsupportedSegments: append([]string(nil), unsupported...),
	}

	for _, v := range views {
		rendered := v.CanonicalText
		draft.ClaimRefs = append(draft.ClaimRefs, AnswerClaimRef{
			ClaimID:       v.ClaimID,
			RenderedText:  rendered,
			SourceSpanIDs: append([]string(nil), v.SourceSpanIDs...),
		})
	}
	return draft
}
