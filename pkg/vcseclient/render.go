package vcseclient

import (
	"context"
	"net/http"
)

// VerifyRenderedAnswer calls POST /render/verify.
func (c *Client) VerifyRenderedAnswer(ctx context.Context, answer any, claims any) (*ValidationResult, error) {
	payload := map[string]any{
		"answer": answer,
		"claims": claims,
	}
	body, err := c.do(ctx, http.MethodPost, "/render/verify", payload)
	if err != nil {
		return nil, err
	}
	return parseValidationResult(body)
}
