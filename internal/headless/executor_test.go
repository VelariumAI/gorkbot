package headless

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestTokenCollector_Accumulation(t *testing.T) {
	tc := NewTokenCollector()

	tokens := []string{"Hello", " ", "world"}
	for _, token := range tokens {
		if err := tc.OnToken(token); err != nil {
			t.Fatalf("OnToken failed: %v", err)
		}
	}

	result := tc.GetContent()
	expected := "Hello world"
	if result != expected {
		t.Errorf("content mismatch: %q vs %q", result, expected)
	}
}

func TestTokenCollector_Duration(t *testing.T) {
	tc := NewTokenCollector()

	time.Sleep(10 * time.Millisecond)
	duration := tc.GetDuration()

	if duration < 10*time.Millisecond {
		t.Errorf("duration too short: %v", duration)
	}
	if duration > 100*time.Millisecond {
		t.Errorf("duration too long: %v", duration)
	}
}

func TestExecutorAdapter_NoExecuteFunc(t *testing.T) {
	adapter := NewExecutorAdapter(nil)

	resp, _ := adapter.Execute(context.Background(), &Request{Prompt: "test"})
	if resp.Success {
		t.Error("expected failure with no execute function")
	}
	if resp.Error == nil {
		t.Error("expected error detail")
	}
}

func TestExecutorAdapter_SimpleExecution(t *testing.T) {
	// Mock execute function
	executeFunc := func(ctx context.Context, prompt string) (<-chan string, error) {
		ch := make(chan string, 3)
		go func() {
			ch <- "Hello"
			ch <- " "
			ch <- "world"
			close(ch)
		}()
		return ch, nil
	}

	adapter := NewExecutorAdapter(executeFunc)
	resp, _ := adapter.Execute(context.Background(), &Request{Prompt: "test"})
	if !resp.Success {
		t.Errorf("expected success, got error: %v", resp.Error)
	}
	if resp.Content != "Hello world" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
}

func TestExecutorAdapter_WithMetrics(t *testing.T) {
	executeFunc := func(ctx context.Context, prompt string) (<-chan string, error) {
		ch := make(chan string, 1)
		ch <- "test"
		close(ch)
		return ch, nil
	}

	metricsFunc := func() *MetricsDetail {
		return &MetricsDetail{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
			CostUSD:      0.0001,
			LatencyMS:    100,
		}
	}

	adapter := NewExecutorAdapter(executeFunc)
	adapter.SetMetricsCollector(metricsFunc)

	resp, _ := adapter.Execute(context.Background(), &Request{Prompt: "test", IncludeMetrics: true})

	if resp.Metrics == nil {
		t.Error("expected metrics in response")
	} else if resp.Metrics.TotalTokens != 15 {
		t.Errorf("token count mismatch: %d", resp.Metrics.TotalTokens)
	}
}

func TestExecutorAdapter_TokenApproximation(t *testing.T) {
	content := "Hello world!"  // 12 chars ≈ 3 tokens
	tokens := countTokensApprox(content)

	expectedMin := 2
	expectedMax := 4
	if tokens < expectedMin || tokens > expectedMax {
		t.Errorf("token estimate %d outside expected range [%d, %d]", tokens, expectedMin, expectedMax)
	}
}

func TestStreamingWriter_TextFormat(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	sw := NewStreamingWriter(buf, FormatText)

	tokens := []string{"Hello", " ", "world"}
	for _, token := range tokens {
		if err := sw.WriteToken(token); err != nil {
			t.Fatalf("WriteToken failed: %v", err)
		}
	}

	output := buf.String()
	if output != "Hello world" {
		t.Errorf("output mismatch: %q", output)
	}
}

func TestStreamingWriter_MarkdownFormat(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	sw := NewStreamingWriter(buf, FormatMarkdown)

	tokens := []string{"# Title", "\n\n", "Content"}
	for _, token := range tokens {
		if err := sw.WriteToken(token); err != nil {
			t.Fatalf("WriteToken failed: %v", err)
		}
	}

	output := buf.String()
	if !contains(output, "Title") {
		t.Errorf("markdown not preserved: %q", output)
	}
}

func TestHeadlessModeConfig_Validation(t *testing.T) {
	// Valid config
	config := &HeadlessModeConfig{
		Format:   FormatJSON,
		Timeout:  30 * time.Second,
		MaxTokens: 2048,
	}
	if err := config.ValidateConfig(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}

	// Invalid format
	config.Format = OutputFormat("invalid")
	if err := config.ValidateConfig(); err == nil {
		t.Error("invalid format accepted")
	}

	// Invalid timeout
	config.Format = FormatText
	config.Timeout = 0
	if err := config.ValidateConfig(); err == nil {
		t.Error("zero timeout accepted")
	}

	// Invalid max tokens
	config.Timeout = 30 * time.Second
	config.MaxTokens = -1
	if err := config.ValidateConfig(); err == nil {
		t.Error("negative max tokens accepted")
	}
}

func TestDefaultHeadlessModeConfig(t *testing.T) {
	config := DefaultHeadlessModeConfig()

	if config.Format != FormatText {
		t.Errorf("default format should be text, got %q", config.Format)
	}
	if config.Timeout != 60*time.Second {
		t.Errorf("default timeout should be 60s, got %v", config.Timeout)
	}
	if config.MaxTokens != 4096 {
		t.Errorf("default max tokens should be 4096, got %d", config.MaxTokens)
	}
	if config.IncludeMetrics {
		t.Error("metrics should be disabled by default")
	}
}

func TestRequestBuilder_Fluent(t *testing.T) {
	req := NewRequestBuilder("test query").
		WithFormat(FormatJSON).
		WithTimeout(30 * time.Second).
		WithAllowTools("bash", "read_file").
		WithDenyTools("write_file").
		WithMetrics().
		WithThinking().
		Build()

	if req.Prompt != "test query" {
		t.Errorf("prompt mismatch: %q", req.Prompt)
	}
	if req.Format != FormatJSON {
		t.Errorf("format mismatch: %q", req.Format)
	}
	if req.Timeout != 30*time.Second {
		t.Errorf("timeout mismatch: %v", req.Timeout)
	}
	if len(req.AllowTools) != 2 {
		t.Errorf("allow tools count mismatch: %d", len(req.AllowTools))
	}
	if len(req.DenyTools) != 1 {
		t.Errorf("deny tools count mismatch: %d", len(req.DenyTools))
	}
	if !req.IncludeMetrics {
		t.Error("metrics flag should be true")
	}
	if !req.IncludeThinking {
		t.Error("thinking flag should be true")
	}
}

func TestRequestBuilder_AllowToolsAccumulation(t *testing.T) {
	req := NewRequestBuilder("test").
		WithAllowTools("tool1").
		WithAllowTools("tool2", "tool3").
		Build()

	if len(req.AllowTools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(req.AllowTools))
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
