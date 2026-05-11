package vcseclient

import (
	"context"
	"net/http"
)

// ValidateLedgerEvent calls POST /ledger/validate.
func (c *Client) ValidateLedgerEvent(ctx context.Context, payload any) (*ValidationResult, error) {
	body, err := c.do(ctx, http.MethodPost, "/ledger/validate", payload)
	if err != nil {
		return nil, err
	}
	return parseValidationResult(body)
}
