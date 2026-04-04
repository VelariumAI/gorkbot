package headless

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// TokenCollector aggregates tokens streamed during execution
type TokenCollector struct {
	buffer   strings.Builder
	startTime time.Time
}

// NewTokenCollector creates a token collector
func NewTokenCollector() *TokenCollector {
	return &TokenCollector{
		startTime: time.Now(),
	}
}

// OnToken appends a token to the buffer
func (tc *TokenCollector) OnToken(token string) error {
	tc.buffer.WriteString(token)
	return nil
}

// GetContent returns accumulated tokens
func (tc *TokenCollector) GetContent() string {
	return tc.buffer.String()
}

// GetDuration returns elapsed time since collection started
func (tc *TokenCollector) GetDuration() time.Duration {
	return time.Since(tc.startTime)
}

// ExecutorAdapter bridges headless requests to an orchestrator-like interface
type ExecutorAdapter struct {
	// Execute function that takes context and prompt, returns content
	// This would be implemented by integrating with pkg/ai/provider or internal/engine/orchestrator
	executeFunc func(ctx context.Context, prompt string) (<-chan string, error)

	// Optional: Tool gating function for allow/deny lists
	toolGate func(toolName string, allowList, denyList []string) bool

	// Optional: Metrics collector
	metricsCollector func() *MetricsDetail
}

// NewExecutorAdapter creates a new executor adapter
func NewExecutorAdapter(
	executeFunc func(ctx context.Context, prompt string) (<-chan string, error),
) *ExecutorAdapter {
	return &ExecutorAdapter{
		executeFunc: executeFunc,
	}
}

// SetToolGate registers a tool allow/deny gate function
func (ea *ExecutorAdapter) SetToolGate(gateFunc func(toolName string, allowList, denyList []string) bool) {
	ea.toolGate = gateFunc
}

// SetMetricsCollector registers a metrics collection function
func (ea *ExecutorAdapter) SetMetricsCollector(collectorFunc func() *MetricsDetail) {
	ea.metricsCollector = collectorFunc
}

// Execute implements the execution flow for a headless request
func (ea *ExecutorAdapter) Execute(ctx context.Context, req *Request) (*Response, error) {
	startTime := time.Now()

	// Validate tools if gate is set
	if ea.toolGate != nil {
		for _, toolName := range req.DenyTools {
			if !ea.toolGate(toolName, req.AllowTools, req.DenyTools) {
				return &Response{
					Success: false,
					Error: &ErrorDetail{
						Type:    "tool_denied",
						Message: fmt.Sprintf("tool %q denied", toolName),
						Code:    403,
					},
				}, nil
			}
		}
	}

	// Check if execute function is configured
	if ea.executeFunc == nil {
		return &Response{
			Success: false,
			Error: &ErrorDetail{
				Type:    "configuration",
				Message: "execute function not configured",
				Code:    500,
			},
		}, nil
	}

	// Collect tokens from streaming response
	collector := NewTokenCollector()
	tokenChan, err := ea.executeFunc(ctx, req.Prompt)
	if err != nil {
		return &Response{
			Success: false,
			Duration: time.Since(startTime),
			Error: &ErrorDetail{
				Type:    "execution",
				Message: fmt.Sprintf("execution failed: %v", err),
				Code:    500,
			},
		}, nil
	}

	// Collect all tokens
	if tokenChan != nil {
		for token := range tokenChan {
			if err := collector.OnToken(token); err != nil {
				return &Response{
					Success: false,
					Duration: time.Since(startTime),
					Error: &ErrorDetail{
						Type:    "stream_error",
						Message: fmt.Sprintf("token collection failed: %v", err),
						Code:    500,
					},
				}, nil
			}
		}
	}

	// Build response
	resp := &Response{
		Content:    collector.GetContent(),
		Success:    true,
		Duration:   time.Since(startTime),
		TokenCount: countTokensApprox(collector.GetContent()),
	}

	// Attach metrics if available
	if ea.metricsCollector != nil {
		resp.Metrics = ea.metricsCollector()
	}

	// Add duration in milliseconds for JSON output
	resp.ExitCode = 0

	return resp, nil
}

// countTokensApprox provides a rough token count estimate (1 token ≈ 4 characters)
func countTokensApprox(content string) int {
	// Rough approximation: 1 token ≈ 4 characters (varies by model/tokenizer)
	// This is just for estimates; actual token count requires model-specific tokenizer
	return (len(content) + 3) / 4
}

// StreamingWriter wraps an io.Writer to support streaming token writes
type StreamingWriter struct {
	w      io.Writer
	writer *Writer
	format OutputFormat
}

// NewStreamingWriter creates a streaming writer for progressive output
func NewStreamingWriter(w io.Writer, format OutputFormat) *StreamingWriter {
	return &StreamingWriter{
		w:      w,
		format: format,
		writer: NewWriter(w, format, nil),
	}
}

// WriteToken writes a single token (may buffer depending on format)
func (sw *StreamingWriter) WriteToken(token string) error {
	switch sw.format {
	case FormatJSONL:
		// Write token as a line of JSON
		return sw.writer.WriteStreamingToken(token, nil)
	case FormatText, FormatMarkdown:
		// Direct write for text formats
		_, err := io.WriteString(sw.w, token)
		return err
	default:
		_, err := io.WriteString(sw.w, token)
		return err
	}
}

// WritePartialResponse writes an incomplete response (for streaming progress)
func (sw *StreamingWriter) WritePartialResponse(partial *Response) error {
	if sw.format != FormatJSONL {
		// For non-JSONL formats, just write content progressively
		_, err := io.WriteString(sw.w, partial.Content)
		return err
	}

	// For JSONL, write a structured partial update
	return sw.writer.WriteResponse(partial)
}

// HeadlessModeConfig configures headless execution
type HeadlessModeConfig struct {
	// Output format
	Format OutputFormat

	// Execution behavior
	Timeout time.Duration

	// Tool permissions
	AllowTools []string
	DenyTools  []string

	// Output control
	IncludeMetrics  bool
	IncludeThinking bool
	MaxTokens       int

	// Streaming options
	StreamTokens bool // Write tokens as they arrive (for JSONL)
}

// DefaultHeadlessModeConfig returns sensible defaults
func DefaultHeadlessModeConfig() *HeadlessModeConfig {
	return &HeadlessModeConfig{
		Format:          FormatText,
		Timeout:         60 * time.Second,
		IncludeMetrics:  false,
		IncludeThinking: false,
		StreamTokens:    false,
		MaxTokens:       4096,
	}
}

// ValidateConfig checks configuration validity
func (hc *HeadlessModeConfig) ValidateConfig() error {
	// Validate format
	validFormats := map[OutputFormat]bool{
		FormatText:     true,
		FormatJSON:     true,
		FormatJSONL:    true,
		FormatMarkdown: true,
	}
	if !validFormats[hc.Format] {
		return fmt.Errorf("invalid format: %q", hc.Format)
	}

	// Validate timeout
	if hc.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", hc.Timeout)
	}

	// Validate max tokens
	if hc.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive, got %d", hc.MaxTokens)
	}

	return nil
}

// RequestBuilder helps construct headless requests with fluent API
type RequestBuilder struct {
	req *Request
}

// NewRequestBuilder creates a request builder
func NewRequestBuilder(prompt string) *RequestBuilder {
	return &RequestBuilder{
		req: &Request{
			Prompt: prompt,
			Format: FormatText,
		},
	}
}

// WithFormat sets the output format
func (rb *RequestBuilder) WithFormat(format OutputFormat) *RequestBuilder {
	rb.req.Format = format
	return rb
}

// WithTimeout sets the execution timeout
func (rb *RequestBuilder) WithTimeout(timeout time.Duration) *RequestBuilder {
	rb.req.Timeout = timeout
	return rb
}

// WithAllowTools sets allowed tools
func (rb *RequestBuilder) WithAllowTools(tools ...string) *RequestBuilder {
	rb.req.AllowTools = append(rb.req.AllowTools, tools...)
	return rb
}

// WithDenyTools sets denied tools
func (rb *RequestBuilder) WithDenyTools(tools ...string) *RequestBuilder {
	rb.req.DenyTools = append(rb.req.DenyTools, tools...)
	return rb
}

// WithMetrics enables metrics in response
func (rb *RequestBuilder) WithMetrics() *RequestBuilder {
	rb.req.IncludeMetrics = true
	return rb
}

// WithThinking enables thinking/reasoning in response
func (rb *RequestBuilder) WithThinking() *RequestBuilder {
	rb.req.IncludeThinking = true
	return rb
}

// Build returns the constructed request
func (rb *RequestBuilder) Build() *Request {
	return rb.req
}
