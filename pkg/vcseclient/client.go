package vcseclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ValidationResult is the generic VCSE validation response shape.
type ValidationResult struct {
	Status   string          `json:"status"`
	Valid    bool            `json:"valid"`
	Accepted bool            `json:"accepted"`
	Issues   []string        `json:"issues"`
	Raw      json.RawMessage `json:"-"`
}

// Client talks to VCSE HTTP endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a VCSE client.
func New(config Config) *Client {
	def := DefaultConfig()
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = def.BaseURL
	}
	if config.Timeout <= 0 {
		config.Timeout = def.Timeout
	}
	return &Client{
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func (c *Client) do(ctx context.Context, method, endpoint string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyTransportErr(err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(respBody)}
	}
	return respBody, nil
}

func parseValidationResult(body []byte) (*ValidationResult, error) {
	var out ValidationResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, wrapInvalidResponse(err)
	}
	out.Raw = append(json.RawMessage(nil), body...)
	if out.Issues == nil {
		out.Issues = []string{}
	}
	return &out, nil
}

func classifyTransportErr(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(ErrTimeout, err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errors.Join(ErrTimeout, err)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		msg := strings.ToLower(urlErr.Error())
		if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no route to host") || strings.Contains(msg, "dial tcp") {
			return errors.Join(ErrUnavailable, err)
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Join(ErrUnavailable, err)
	}

	return err
}

func wrapInvalidResponse(err error) error {
	return errors.Join(ErrInvalidResponse, err)
}
