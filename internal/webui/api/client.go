// Package api — REST API Client for WebUI
// Phase 3: API Integration with orchestrator backend
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client manages REST API communication with the orchestrator backend.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	headers    map[string]string
	retries    int
}

// NewClient creates a new API client.
func NewClient(baseURL string, logger *slog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		headers: make(map[string]string),
		retries: 3,
	}
}

// SetHeader sets a default header for all requests.
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetAuthToken sets the bearer token for authorization.
func (c *Client) SetAuthToken(token string) {
	c.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
}

// Request represents an HTTP request.
type Request struct {
	Method   string
	Path     string
	Body     interface{}
	Headers  map[string]string
	Timeout  time.Duration
}

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// do performs an HTTP request with retry logic.
func (c *Client) do(ctx context.Context, req *Request) (*Response, error) {
	if req.Timeout == 0 {
		req.Timeout = 10 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		resp, err := c.doSingle(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on client errors (4xx)
		if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return resp, err
		}

		// Back off before retry
		if attempt < c.retries {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, lastErr
}

// doSingle performs a single HTTP request without retry.
func (c *Client) doSingle(ctx context.Context, req *Request) (*Response, error) {
	url := c.baseURL + req.Path

	var body io.Reader
	if req.Body != nil {
		data, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set default headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Set custom headers from client
	for key, value := range c.headers {
		httpReq.Header.Set(key, value)
	}

	// Override with request-specific headers
	if req.Headers != nil {
		for key, value := range req.Headers {
			httpReq.Header.Set(key, value)
		}
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Body:       respBody,
		Headers:    httpResp.Header,
	}

	// Log response
	c.logger.Debug("API response",
		"method", req.Method,
		"path", req.Path,
		"status", httpResp.StatusCode,
		"size", len(respBody),
	)

	// Check for errors
	if httpResp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(respBody, &errResp)

		return resp, fmt.Errorf("API error: %s (status %d)", errResp.Error, httpResp.StatusCode)
	}

	return resp, nil
}

// ParseJSON parses the response body as JSON.
func (r *Response) ParseJSON(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// ────────────────────────────────────────────────────────────
// API Methods
// ────────────────────────────────────────────────────────────

// ChatRequest represents a chat message request.
type ChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

// ChatResponse represents a chat response.
type ChatResponse struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	Tokens   int    `json:"tokens,omitempty"`
	Latency  int64  `json:"latency_ms,omitempty"`
	Error    string `json:"error,omitempty"`
}

// SendChat sends a chat message to the orchestrator.
func (c *Client) SendChat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	httpReq := &Request{
		Method: "POST",
		Path:   "/api/chat",
		Body:   req,
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var chatResp ChatResponse
	if err := resp.ParseJSON(&chatResp); err != nil {
		return nil, fmt.Errorf("parse chat response: %w", err)
	}

	return &chatResp, nil
}

// GetRuns retrieves recent runs from the orchestrator.
func (c *Client) GetRuns(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	path := "/api/runs"
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}

	httpReq := &Request{
		Method: "GET",
		Path:   path,
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var runs []map[string]interface{}
	if err := resp.ParseJSON(&runs); err != nil {
		return nil, fmt.Errorf("parse runs response: %w", err)
	}

	return runs, nil
}

// GetRunDetails retrieves details for a specific run.
func (c *Client) GetRunDetails(ctx context.Context, runID string) (map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   fmt.Sprintf("/api/entities/runs/%s", runID),
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var run map[string]interface{}
	if err := resp.ParseJSON(&run); err != nil {
		return nil, fmt.Errorf("parse run details response: %w", err)
	}

	return run, nil
}

// GetWorkspaces retrieves available workspaces.
func (c *Client) GetWorkspaces(ctx context.Context) ([]map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/workspaces",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var workspaces []map[string]interface{}
	if err := resp.ParseJSON(&workspaces); err != nil {
		return nil, fmt.Errorf("parse workspaces response: %w", err)
	}

	return workspaces, nil
}

// GetAnalyticsMetrics retrieves analytics data.
func (c *Client) GetAnalyticsMetrics(ctx context.Context) (map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/analytics/metrics",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var metrics map[string]interface{}
	if err := resp.ParseJSON(&metrics); err != nil {
		return nil, fmt.Errorf("parse metrics response: %w", err)
	}

	return metrics, nil
}

// GetThemeTokens retrieves design tokens as CSS.
func (c *Client) GetThemeTokens(ctx context.Context) (string, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/theme/tokens.css",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return "", err
	}

	return string(resp.Body), nil
}

// GetTools retrieves available tools.
func (c *Client) GetTools(ctx context.Context) ([]map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/tools",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var tools []map[string]interface{}
	if err := resp.ParseJSON(&tools); err != nil {
		return nil, fmt.Errorf("parse tools response: %w", err)
	}

	return tools, nil
}

// GetMemory retrieves memory items.
func (c *Client) GetMemory(ctx context.Context) ([]map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/memory",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var memory []map[string]interface{}
	if err := resp.ParseJSON(&memory); err != nil {
		return nil, fmt.Errorf("parse memory response: %w", err)
	}

	return memory, nil
}

// GetAgents retrieves agent status.
func (c *Client) GetAgents(ctx context.Context) ([]map[string]interface{}, error) {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/agents",
	}

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	var agents []map[string]interface{}
	if err := resp.ParseJSON(&agents); err != nil {
		return nil, fmt.Errorf("parse agents response: %w", err)
	}

	return agents, nil
}

// HealthCheck verifies API connectivity.
func (c *Client) HealthCheck(ctx context.Context) error {
	httpReq := &Request{
		Method: "GET",
		Path:   "/api/state",
	}

	_, err := c.do(ctx, httpReq)
	return err
}
