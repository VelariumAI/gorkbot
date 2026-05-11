package vcseclient

import (
	"context"
	"net/http"
)

// Health checks GET /health.
func (c *Client) Health(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/health", nil)
	return err
}

// Ready checks GET /ready.
func (c *Client) Ready(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/ready", nil)
	return err
}
