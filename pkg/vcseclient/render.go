package vcseclient

import (
	"context"
	"encoding/json"
	"net/http"
)

type RenderVerifyRequest struct {
	Answer any `json:"answer"`
	Claims any `json:"claims"`
	Policy any `json:"policy,omitempty"`
}

type RenderVerifyDecision struct {
	AnswerID         string          `json:"answer_id"`
	FinalStatus      string          `json:"final_status"`
	Valid            bool            `json:"valid"`
	ReasonCode       string          `json:"reason_code"`
	Issues           []string        `json:"issues,omitempty"`
	ClaimCount       int             `json:"claim_count"`
	AcceptedClaimIDs []string        `json:"accepted_claim_ids,omitempty"`
	RejectedClaimIDs []string        `json:"rejected_claim_ids,omitempty"`
	RenderMode       string          `json:"render_mode"`
	Raw              json.RawMessage `json:"-"`
}

// VerifyRenderedAnswer calls POST /render/verify.
func (c *Client) VerifyRenderedAnswer(ctx context.Context, req RenderVerifyRequest) (*RenderVerifyDecision, error) {
	body, err := c.do(ctx, http.MethodPost, "/render/verify", req)
	if err != nil {
		return nil, err
	}
	var out RenderVerifyDecision
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, wrapInvalidResponse(err)
	}
	out.Raw = append(json.RawMessage(nil), body...)
	if out.Issues == nil {
		out.Issues = []string{}
	}
	return &out, nil
}
