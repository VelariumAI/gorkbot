package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TraceEvent is a single recorded step in the execution trace log.
type TraceEvent struct {
	Timestamp time.Time              `json:"ts"`
	Kind      string                 `json:"kind"` // "tool_call", "tool_result", "llm_request", "llm_response", "hook", "mode_change"
	ToolName  string                 `json:"tool,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
	Result    string                 `json:"result,omitempty"`
	Success   bool                   `json:"success,omitempty"`
	Elapsed   float64                `json:"elapsed_ms,omitempty"`
	Extra     map[string]string      `json:"extra,omitempty"`
}

// TraceLogger writes a newline-delimited JSON trace to a file, one event per line.
// It is safe for concurrent use.
type TraceLogger struct {
	mu      sync.Mutex
	enabled bool
	path    string
	file    *os.File
}

// NewTraceLogger creates and opens a trace log file. If traceDir is empty or
// tracing is not enabled, all calls become no-ops.
func NewTraceLogger(traceDir string, enabled bool) *TraceLogger {
	tl := &TraceLogger{enabled: enabled}
	if !enabled || traceDir == "" {
		return tl
	}

	if err := os.MkdirAll(traceDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "trace: failed to create trace dir: %v\n", err)
		return tl
	}

	name := fmt.Sprintf("trace_%s.jsonl", time.Now().Format("20060102_150405"))
	path := filepath.Join(traceDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace: failed to open trace file: %v\n", err)
		return tl
	}

	tl.path = path
	tl.file = f
	fmt.Fprintf(os.Stderr, "[TRACE] Writing execution trace to: %s\n", path)
	return tl
}

// Enabled returns whether tracing is active.
func (tl *TraceLogger) Enabled() bool { return tl.enabled && tl.file != nil }

// LogToolCall records a tool invocation before execution.
func (tl *TraceLogger) LogToolCall(toolName string, params map[string]interface{}) {
	tl.write(TraceEvent{
		Kind:     "tool_call",
		ToolName: toolName,
		Params:   params,
	})
}

// LogToolResult records the outcome of a tool execution.
func (tl *TraceLogger) LogToolResult(toolName string, result string, success bool, elapsed time.Duration) {
	// Truncate very large results for the trace file.
	const maxResultLen = 4096
	if len(result) > maxResultLen {
		result = result[:maxResultLen] + "... [truncated]"
	}
	tl.write(TraceEvent{
		Kind:     "tool_result",
		ToolName: toolName,
		Result:   result,
		Success:  success,
		Elapsed:  float64(elapsed.Milliseconds()),
	})
}

// LogLLMRequest records that a request was sent to the primary AI.
func (tl *TraceLogger) LogLLMRequest(model string, promptLen int) {
	tl.write(TraceEvent{
		Kind: "llm_request",
		Extra: map[string]string{
			"model":      model,
			"prompt_len": fmt.Sprintf("%d", promptLen),
		},
	})
}

// LogLLMResponse records the result from the primary AI.
func (tl *TraceLogger) LogLLMResponse(model string, responseLen int, elapsed time.Duration) {
	tl.write(TraceEvent{
		Kind:    "llm_response",
		Elapsed: float64(elapsed.Milliseconds()),
		Extra: map[string]string{
			"model":        model,
			"response_len": fmt.Sprintf("%d", responseLen),
		},
	})
}

// LogModeChange records an execution mode transition.
func (tl *TraceLogger) LogModeChange(from, to string) {
	tl.write(TraceEvent{
		Kind: "mode_change",
		Extra: map[string]string{"from": from, "to": to},
	})
}

// LogHook records a hook firing event.
func (tl *TraceLogger) LogHook(event, result string, blocked bool) {
	tl.write(TraceEvent{
		Kind:    "hook",
		Success: !blocked,
		Extra: map[string]string{
			"event":   event,
			"result":  result,
			"blocked": fmt.Sprintf("%v", blocked),
		},
	})
}

// TracePath returns the path to the trace file (empty if not active).
func (tl *TraceLogger) TracePath() string { return tl.path }

// Close flushes and closes the trace file.
func (tl *TraceLogger) Close() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.file != nil {
		_ = tl.file.Close()
		tl.file = nil
	}
}

func (tl *TraceLogger) write(ev TraceEvent) {
	if !tl.enabled || tl.file == nil {
		return
	}
	ev.Timestamp = time.Now()
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	_, _ = tl.file.Write(append(data, '\n'))
}
