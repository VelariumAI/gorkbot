package vcseclient

import (
	"context"
	"net/http"
)

// ValidateProposal calls POST /proposal/validate.
func (c *Client) ValidateProposal(ctx context.Context, payload any) (*ValidationResult, error) {
	body, err := c.do(ctx, http.MethodPost, "/proposal/validate", payload)
	if err != nil {
		return nil, err
	}
	return parseValidationResult(body)
}
