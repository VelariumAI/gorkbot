package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// OutputFormat defines the output serialization format for headless mode
type OutputFormat string

const (
	FormatText   OutputFormat = "text"
	FormatJSON   OutputFormat = "json"
	FormatJSONL  OutputFormat = "jsonl"  // Streaming JSON lines
	FormatMarkdown OutputFormat = "markdown"
)

// Request encapsulates a headless mode query with configuration
type Request struct {
	Prompt          string        // User query
	Format          OutputFormat  // Output format
	Timeout         time.Duration // Operation timeout
	AllowTools      []string      // Allowed tool names (empty = all)
	DenyTools       []string      // Denied tool names
	IncludeMetrics  bool          // Include observability metrics in response
	IncludeThinking bool          // Include AI thinking/reasoning
	Verbose         bool          // Verbose logging
	MaxTokens       int           // Max tokens in response
}

// Response encapsulates a headless mode result
type Response struct {
	// Core response
	Content string `json:"content"` // The main response text

	// Metadata
	Duration   time.Duration `json:"duration_ms,omitempty"`
	TokenCount int           `json:"tokens,omitempty"`
	ExitCode   int           `json:"exit_code,omitempty"`
	Success    bool          `json:"success"`

	// Optional details
	Error      *ErrorDetail   `json:"error,omitempty"`
	Metrics    *MetricsDetail `json:"metrics,omitempty"`
	Thinking   string         `json:"thinking,omitempty"`
	Tools      []ToolExecution `json:"tools,omitempty"`
}

// ErrorDetail provides structured error information
type ErrorDetail struct {
	Type    string `json:"type"`    // error type (validation, timeout, provider_error, etc.)
	Message string `json:"message"` // human-readable message
	Code    int    `json:"code"`    // error code (400, 503, etc.)
}

// MetricsDetail provides observability data
type MetricsDetail struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	LatencyMS        int     `json:"latency_ms"`
	ProviderUsed     string  `json:"provider_used"`
	CacheHitRate     float64 `json:"cache_hit_rate,omitempty"`
	ContextUsagePercent float64 `json:"context_usage_percent,omitempty"`
}

// ToolExecution records a tool invocation during headless execution
type ToolExecution struct {
	Name     string        `json:"name"`
	Status   string        `json:"status"`    // "pending", "running", "success", "error"
	DurationMS int         `json:"duration_ms"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// Writer provides output writing with format support
type Writer struct {
	w      io.Writer
	format OutputFormat
	logger *slog.Logger

	// For JSONL streaming
	isFirstLine bool
}

// NewWriter creates a new headless output writer
func NewWriter(w io.Writer, format OutputFormat, logger *slog.Logger) *Writer {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Writer{
		w:           w,
		format:      format,
		logger:      logger,
		isFirstLine: true,
	}
}

// WriteResponse writes a complete response in the configured format
func (wr *Writer) WriteResponse(resp *Response) error {
	switch wr.format {
	case FormatJSON:
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		_, err = wr.w.Write(append(data, '\n'))
		return err

	case FormatJSONL:
		data, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		_, err = wr.w.Write(append(data, '\n'))
		return err

	case FormatMarkdown:
		return wr.writeMarkdown(resp)

	case FormatText:
		fallthrough
	default:
		return wr.writeText(resp)
	}
}

// WriteStreamingToken writes a token for streaming responses (for FormatJSONL)
func (wr *Writer) WriteStreamingToken(token string, metadata map[string]interface{}) error {
	if wr.format != FormatJSONL {
		// For non-JSONL formats, just write the token as-is
		_, err := io.WriteString(wr.w, token)
		return err
	}

	// For JSONL, write structured token objects
	obj := map[string]interface{}{
		"type": "token",
		"data": token,
	}
	if metadata != nil {
		for k, v := range metadata {
			obj[k] = v
		}
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = wr.w.Write(append(data, '\n'))
	return err
}

// writeText writes response in plain text format
func (wr *Writer) writeText(resp *Response) error {
	var buf strings.Builder

	if resp.Content != "" {
		buf.WriteString(resp.Content)
		buf.WriteString("\n")
	}

	if resp.Error != nil {
		buf.WriteString("\nERROR: ")
		buf.WriteString(resp.Error.Message)
		buf.WriteString("\n")
	}

	if resp.Metrics != nil && resp.Metrics.TotalTokens > 0 {
		buf.WriteString("\n---\n")
		buf.WriteString(fmt.Sprintf("Tokens: %d input + %d output = %d total\n",
			resp.Metrics.InputTokens, resp.Metrics.OutputTokens, resp.Metrics.TotalTokens))
		buf.WriteString(fmt.Sprintf("Cost: $%.4f\n", resp.Metrics.CostUSD))
		buf.WriteString(fmt.Sprintf("Latency: %dms\n", resp.Metrics.LatencyMS))
	}

	_, err := io.WriteString(wr.w, buf.String())
	return err
}

// writeMarkdown writes response in markdown format
func (wr *Writer) writeMarkdown(resp *Response) error {
	var buf strings.Builder

	buf.WriteString(resp.Content)
	buf.WriteString("\n")

	if resp.Metrics != nil && resp.Metrics.TotalTokens > 0 {
		buf.WriteString("\n## Metrics\n\n")
		buf.WriteString(fmt.Sprintf("- **Tokens**: %d input + %d output = %d total\n",
			resp.Metrics.InputTokens, resp.Metrics.OutputTokens, resp.Metrics.TotalTokens))
		buf.WriteString(fmt.Sprintf("- **Cost**: $%.4f\n", resp.Metrics.CostUSD))
		buf.WriteString(fmt.Sprintf("- **Latency**: %dms\n", resp.Metrics.LatencyMS))
		if resp.Metrics.ProviderUsed != "" {
			buf.WriteString(fmt.Sprintf("- **Provider**: %s\n", resp.Metrics.ProviderUsed))
		}
	}

	if len(resp.Tools) > 0 {
		buf.WriteString("\n## Tool Executions\n\n")
		for _, t := range resp.Tools {
			buf.WriteString(fmt.Sprintf("- **%s**: %s (%dms)\n", t.Name, t.Status, t.DurationMS))
			if t.Error != "" {
				buf.WriteString(fmt.Sprintf("  - Error: %s\n", t.Error))
			}
		}
	}

	if resp.Error != nil {
		buf.WriteString(fmt.Sprintf("\n**Error**: %s (code: %d)\n", resp.Error.Message, resp.Error.Code))
	}

	_, err := io.WriteString(wr.w, buf.String())
	return err
}

// Runner executes headless requests
type Runner struct {
	logger *slog.Logger
	// Dependencies to be injected
	executeFunc func(ctx context.Context, req *Request) (*Response, error)
}

// NewRunner creates a new headless runner
func NewRunner(logger *slog.Logger, executeFunc func(ctx context.Context, req *Request) (*Response, error)) *Runner {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Runner{
		logger:      logger,
		executeFunc: executeFunc,
	}
}

// Execute runs a headless request with timeout
func (r *Runner) Execute(req *Request) (*Response, error) {
	if req == nil {
		return &Response{
			Success: false,
			Error: &ErrorDetail{
				Type:    "validation",
				Message: "request cannot be nil",
				Code:    400,
			},
		}, nil
	}

	// Validate prompt
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return &Response{
			Success: false,
			Error: &ErrorDetail{
				Type:    "validation",
				Message: "prompt cannot be empty",
				Code:    400,
			},
		}, nil
	}

	// Set defaults
	if req.Timeout == 0 {
		req.Timeout = 60 * time.Second
	}
	if req.Format == "" {
		req.Format = FormatText
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()

	// Execute with injected function or error
	if r.executeFunc == nil {
		return &Response{
			Success: false,
			Error: &ErrorDetail{
				Type:    "system",
				Message: "execute function not configured",
				Code:    500,
			},
		}, nil
	}

	resp, err := r.executeFunc(ctx, req)
	if err != nil {
		return &Response{
			Success: false,
			Error: &ErrorDetail{
				Type:    "execution",
				Message: err.Error(),
				Code:    500,
			},
		}, nil
	}

	return resp, nil
}
