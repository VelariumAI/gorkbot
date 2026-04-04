package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRequest_Defaults(t *testing.T) {
	req := &Request{
		Prompt: "test",
	}

	if req.Prompt != "test" {
		t.Errorf("expected prompt to be 'test', got %q", req.Prompt)
	}
	if req.Format != "" {
		t.Errorf("expected empty format, got %q", req.Format)
	}
	if req.Timeout != 0 {
		t.Errorf("expected zero timeout, got %v", req.Timeout)
	}
}

func TestResponse_Serialization(t *testing.T) {
	resp := &Response{
		Content: "Hello, world!",
		Success: true,
		TokenCount: 10,
		Duration: 500 * time.Millisecond,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if decoded.Content != resp.Content {
		t.Errorf("content mismatch: %q vs %q", decoded.Content, resp.Content)
	}
	if decoded.TokenCount != resp.TokenCount {
		t.Errorf("token count mismatch: %d vs %d", decoded.TokenCount, resp.TokenCount)
	}
}

func TestWriter_FormatText(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wr := NewWriter(buf, FormatText, logger)

	resp := &Response{
		Content: "Test response",
		Success: true,
		Metrics: &MetricsDetail{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.001,
			LatencyMS:    100,
		},
	}

	if err := wr.WriteResponse(resp); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Test response") {
		t.Errorf("output missing content: %s", output)
	}
	if !strings.Contains(output, "Tokens:") {
		t.Errorf("output missing metrics: %s", output)
	}
}

func TestWriter_FormatJSON(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wr := NewWriter(buf, FormatJSON, logger)

	resp := &Response{
		Content: "Test",
		Success: true,
		ExitCode: 0,
	}

	if err := wr.WriteResponse(resp); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}

	output := buf.String()
	var decoded Response
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if decoded.Content != "Test" {
		t.Errorf("content mismatch: %q", decoded.Content)
	}
	if !decoded.Success {
		t.Errorf("expected success=true")
	}
}

func TestWriter_FormatJSONL_SingleResponse(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wr := NewWriter(buf, FormatJSONL, logger)

	resp := &Response{
		Content: "Line response",
		Success: true,
	}

	if err := wr.WriteResponse(resp); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}

	var decoded Response
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("line is not valid JSON: %v", err)
	}
}

func TestWriter_FormatMarkdown(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wr := NewWriter(buf, FormatMarkdown, logger)

	resp := &Response{
		Content: "# Title\n\nContent",
		Success: true,
		Metrics: &MetricsDetail{
			TotalTokens: 100,
			CostUSD:     0.001,
			LatencyMS:   50,
			ProviderUsed: "xai",
		},
		Tools: []ToolExecution{
			{
				Name:       "bash",
				Status:     "success",
				DurationMS: 100,
			},
		},
	}

	if err := wr.WriteResponse(resp); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "# Title") {
		t.Errorf("markdown missing title: %s", output)
	}
	if !strings.Contains(output, "## Metrics") {
		t.Errorf("markdown missing metrics section: %s", output)
	}
	if !strings.Contains(output, "## Tool Executions") {
		t.Errorf("markdown missing tools section: %s", output)
	}
}

func TestWriter_StreamingToken_JSONL(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wr := NewWriter(buf, FormatJSONL, logger)

	tokens := []string{"Hello", " ", "world", "!"}
	for _, token := range tokens {
		if err := wr.WriteStreamingToken(token, nil); err != nil {
			t.Fatalf("failed to write token: %v", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(lines))
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatalf("line is not valid JSON: %v", err)
	}
	if obj["type"] != "token" {
		t.Errorf("expected type=token, got %v", obj["type"])
	}
}

func TestRunner_ValidatePrompt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	executed := false

	executeFunc := func(ctx context.Context, req *Request) (*Response, error) {
		executed = true
		return &Response{Success: true, Content: "OK"}, nil
	}

	runner := NewRunner(logger, executeFunc)

	// Empty prompt should be rejected
	resp, _ := runner.Execute(&Request{Prompt: ""})
	if resp.Success {
		t.Error("empty prompt should fail validation")
	}
	if executed {
		t.Error("execute should not be called for invalid prompt")
	}

	// Whitespace-only prompt should be rejected
	resp, _ = runner.Execute(&Request{Prompt: "   \n  \t  "})
	if resp.Success {
		t.Error("whitespace-only prompt should fail validation")
	}
}

func TestRunner_Timeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	executeFunc := func(ctx context.Context, req *Request) (*Response, error) {
		// Simulate a slow operation
		time.Sleep(200 * time.Millisecond)
		return &Response{Success: true, Content: "Done"}, nil
	}

	runner := NewRunner(logger, executeFunc)

	// Request with short timeout
	req := &Request{
		Prompt:  "test",
		Timeout: 50 * time.Millisecond,
	}

	// This should timeout during execution
	resp, _ := runner.Execute(req)
	if resp == nil {
		t.Fatal("expected response even on timeout")
	}
}

func TestRunner_DefaultTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	executeFunc := func(ctx context.Context, req *Request) (*Response, error) {
		if req.Timeout != 60*time.Second {
			t.Errorf("expected 60s default timeout, got %v", req.Timeout)
		}
		return &Response{Success: true, Content: "OK"}, nil
	}

	runner := NewRunner(logger, executeFunc)
	runner.Execute(&Request{Prompt: "test"})
}

func TestRunner_DefaultFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	executeFunc := func(ctx context.Context, req *Request) (*Response, error) {
		if req.Format != FormatText {
			t.Errorf("expected text format, got %q", req.Format)
		}
		return &Response{Success: true, Content: "OK"}, nil
	}

	runner := NewRunner(logger, executeFunc)
	runner.Execute(&Request{Prompt: "test"})
}

func TestErrorDetail_Serialization(t *testing.T) {
	resp := &Response{
		Success: false,
		Error: &ErrorDetail{
			Type:    "validation",
			Message: "Invalid input",
			Code:    400,
		},
	}

	data, _ := json.Marshal(resp)
	output := string(data)

	if !strings.Contains(output, "validation") {
		t.Errorf("error type missing: %s", output)
	}
	if !strings.Contains(output, "Invalid input") {
		t.Errorf("error message missing: %s", output)
	}
}

func TestMetricsDetail_Serialization(t *testing.T) {
	resp := &Response{
		Success: true,
		Metrics: &MetricsDetail{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
			CostUSD:      0.0225,
			LatencyMS:    2500,
			ProviderUsed: "xai",
			CacheHitRate: 0.75,
		},
	}

	data, _ := json.Marshal(resp)

	var decoded Response
	json.Unmarshal(data, &decoded)

	if decoded.Metrics.TotalTokens != 1500 {
		t.Errorf("tokens mismatch: %d", decoded.Metrics.TotalTokens)
	}
	if decoded.Metrics.CostUSD != 0.0225 {
		t.Errorf("cost mismatch: %f", decoded.Metrics.CostUSD)
	}
	if decoded.Metrics.CacheHitRate != 0.75 {
		t.Errorf("cache hit rate mismatch: %f", decoded.Metrics.CacheHitRate)
	}
}

func TestToolExecution_List(t *testing.T) {
	tools := []ToolExecution{
		{
			Name:       "bash",
			Status:     "success",
			DurationMS: 100,
		},
		{
			Name:       "read_file",
			Status:     "error",
			DurationMS: 50,
			Error:      "file not found",
		},
	}

	resp := &Response{
		Success: true,
		Tools:   tools,
	}

	data, _ := json.Marshal(resp)

	var decoded Response
	json.Unmarshal(data, &decoded)

	if len(decoded.Tools) != 2 {
		t.Errorf("tools count mismatch: %d", len(decoded.Tools))
	}
	if decoded.Tools[1].Error != "file not found" {
		t.Errorf("tool error missing: %s", decoded.Tools[1].Error)
	}
}
