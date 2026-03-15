package sense

// tracer.go — SENSE Event Tracer
//
// Writes structured, daily-rotated JSONL trace files to:
//
//   <traceDir>/<YYYY-MM-DD>.jsonl
//
// Each line is a self-contained JSON object (SENSETrace).  Daily rotation
// keeps individual files small and makes the trace analyzer's glob pattern
// straightforward.
//
// Performance design: all disk I/O is done in a single background goroutine
// (drainLoop). The public LogXxx methods serialize to JSON and enqueue the
// bytes on a 512-entry buffered channel — they NEVER block the caller.
// If the buffer fills (>512 queued events) the event is silently dropped;
// this is preferable to stalling tool execution.
//
// All public methods are safe for concurrent use.  The tracer degrades
// gracefully: if the trace directory cannot be created, LogXxx calls are
// no-ops and drainLoop is never started.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TraceEventKind is the semantic classification of a SENSE trace event.
type TraceEventKind string

const (
	// KindToolSuccess records a tool execution that completed without error.
	KindToolSuccess TraceEventKind = "tool_success"
	// KindToolFailure records a tool execution that returned an error or
	// result.Success == false.
	KindToolFailure TraceEventKind = "tool_failure"
	// KindHallucination records evidence of a neural hallucination: the agent
	// referenced a non-existent tool, a contradicted fact, or fabricated data.
	KindHallucination TraceEventKind = "hallucination"
	// KindContextOverflow records a context token-limit exceeded event.
	KindContextOverflow TraceEventKind = "context_overflow"
	// KindSanitizerReject records a stabilization middleware rejection.
	KindSanitizerReject TraceEventKind = "sanitizer_reject"
	// KindProviderError records a transient AI provider failure (rate limit,
	// network timeout, server error).
	KindProviderError TraceEventKind = "provider_error"
	// KindParamError records a missing or invalid required parameter.
	KindParamError TraceEventKind = "param_error"
)

// traceBufSize is the number of serialised trace lines the channel can hold
// before events are dropped. 512 covers even heavy multi-tool sessions without
// stalling the caller.
const traceBufSize = 512

// SENSETrace is a single event written to the JSONL trace file.
// Every field is JSON-serialisable and uses json tags to keep the on-disk
// format stable across Go struct renames.
type SENSETrace struct {
	// Timestamp is RFC3339Nano UTC.
	Timestamp string `json:"ts"`
	// SessionID is the short identifier for the current process session.
	// Allows grouping events from the same invocation when multiple files exist.
	SessionID string `json:"sid,omitempty"`
	// Kind is the semantic classification of this event.
	Kind TraceEventKind `json:"kind"`
	// ToolName is the normalised name of the tool that was called (if any).
	ToolName string `json:"tool,omitempty"`
	// ProviderID identifies the AI provider associated with the event (if any).
	ProviderID string `json:"provider,omitempty"`
	// Input is a truncated JSON representation of the tool parameters.
	Input string `json:"input,omitempty"`
	// Output is a truncated representation of the tool result.
	Output string `json:"output,omitempty"`
	// Error is the error message or failure reason.
	Error string `json:"error,omitempty"`
	// DurationMS is the wall-clock execution time in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// ContextTokens is the estimated token count at the time of the event.
	ContextTokens int `json:"ctx_tokens,omitempty"`
	// Labels is a list of semantic tags for fast grep-based filtering.
	// Example: ["error", "timeout"], ["hallucination", "neural"]
	Labels []string `json:"labels,omitempty"`
}

// SENSETracer writes structured trace events to daily-rotated JSONL files.
// Instantiate once per process with NewSENSETracer.
//
// All I/O is performed by a single background goroutine (drainLoop); public
// methods never block on disk.
type SENSETracer struct {
	sessionID string
	traceDir  string
	// disabled is set at construction when the trace directory cannot be created.
	// Immutable after NewSENSETracer returns — no mutex needed for reads.
	disabled bool
	// writeCh carries pre-serialised JSONL lines to drainLoop.
	// Nil when disabled.
	writeCh chan []byte
	// wg tracks the drainLoop goroutine so Close() can wait for full flush.
	wg sync.WaitGroup
}

// NewSENSETracer creates a tracer that writes to traceDir.  The directory is
// created (mode 0700) if it does not exist.  sessionID is embedded in every
// event to correlate events across the same process run.
//
// If the directory cannot be created, the tracer operates in no-op mode:
// all LogXxx calls return without error or panicking.
func NewSENSETracer(traceDir, sessionID string) *SENSETracer {
	t := &SENSETracer{
		traceDir:  traceDir,
		sessionID: sessionID,
	}
	if err := os.MkdirAll(traceDir, 0700); err != nil {
		// Degrade silently — tracing should never crash the app.
		t.disabled = true
		return t
	}
	t.writeCh = make(chan []byte, traceBufSize)
	t.wg.Add(1)
	go t.drainLoop()
	return t
}

// drainLoop is the single goroutine that owns all file I/O.
// It runs until writeCh is closed (by Close()), then flushes any remaining
// buffered lines and returns.
func (t *SENSETracer) drainLoop() {
	defer t.wg.Done()

	var curDate string
	var file *os.File

	// Ensure file is closed when goroutine exits (handles Close() path).
	defer func() {
		if file != nil {
			_ = file.Sync()
			_ = file.Close()
		}
	}()

	for b := range t.writeCh {
		// Daily rotation: reopen file on date change.
		today := time.Now().UTC().Format("2006-01-02")
		if today != curDate || file == nil {
			if file != nil {
				_ = file.Sync()
				_ = file.Close()
				file = nil
			}
			path := filepath.Join(t.traceDir, today+".jsonl")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				// Cannot open file — skip this event but keep draining.
				continue
			}
			file = f
			curDate = today
		}
		_, _ = file.Write(b)
	}
}

// write serialises ev and enqueues it for async disk write.
// Returns immediately — never blocks the caller.
func (t *SENSETracer) write(ev SENSETrace) {
	if t.disabled || t.writeCh == nil {
		return
	}
	ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	ev.SessionID = t.sessionID

	b, err := json.Marshal(ev)
	if err != nil {
		return // marshalling should never fail for this struct
	}
	b = append(b, '\n')

	// Non-blocking send: if the buffer is full, drop the event rather than
	// blocking the caller (tool execution / streaming goroutine).
	select {
	case t.writeCh <- b:
	default:
		// Buffer full — event dropped. Prefer responsiveness over completeness.
	}
}

// LogToolSuccess records a successful tool execution.
func (t *SENSETracer) LogToolSuccess(tool, inputJSON, output string, durationMS int64) {
	t.write(SENSETrace{
		Kind:       KindToolSuccess,
		ToolName:   tool,
		Input:      truncateTo(inputJSON, 512),
		Output:     truncateTo(output, 512),
		DurationMS: durationMS,
		Labels:     []string{"success"},
	})
}

// LogToolFailure records a failed tool execution.
func (t *SENSETracer) LogToolFailure(tool, inputJSON, errMsg string, durationMS int64) {
	t.write(SENSETrace{
		Kind:       KindToolFailure,
		ToolName:   tool,
		Input:      truncateTo(inputJSON, 512),
		Error:      truncateTo(errMsg, 1024),
		DurationMS: durationMS,
		Labels:     classifyErrLabels(errMsg),
	})
}

// LogHallucination records a detected neural hallucination event.
// evidence should be a concise description of what contradicted reality.
func (t *SENSETracer) LogHallucination(evidence string) {
	t.write(SENSETrace{
		Kind:   KindHallucination,
		Error:  truncateTo(evidence, 1024),
		Labels: []string{"hallucination", "neural"},
	})
}

// LogContextOverflow records a context token-limit exceeded event.
func (t *SENSETracer) LogContextOverflow(providerID string, contextTokens int, errMsg string) {
	t.write(SENSETrace{
		Kind:          KindContextOverflow,
		ProviderID:    providerID,
		Error:         truncateTo(errMsg, 512),
		ContextTokens: contextTokens,
		Labels:        []string{"context_overflow", "token_limit"},
	})
}

// LogSanitizerReject records a stabilization middleware rejection.
// field is the parameter key that triggered the rejection.
func (t *SENSETracer) LogSanitizerReject(tool, field, reason string) {
	t.write(SENSETrace{
		Kind:     KindSanitizerReject,
		ToolName: tool,
		Error:    fmt.Sprintf("field=%q: %s", field, reason),
		Labels:   []string{"sanitizer", "rejected"},
	})
}

// LogProviderError records a transient AI provider failure.
func (t *SENSETracer) LogProviderError(providerID, model, errMsg string) {
	t.write(SENSETrace{
		Kind:       KindProviderError,
		ProviderID: providerID,
		Error:      truncateTo(errMsg, 512),
		Labels:     classifyErrLabels(errMsg),
	})
}

// LogParamError records a missing or invalid required parameter.
func (t *SENSETracer) LogParamError(tool, errMsg string) {
	t.write(SENSETrace{
		Kind:     KindParamError,
		ToolName: tool,
		Error:    truncateTo(errMsg, 512),
		Labels:   []string{"param_error"},
	})
}

// TraceDir returns the directory path used for trace files.
func (t *SENSETracer) TraceDir() string { return t.traceDir }

// IsEnabled returns true when the tracer is writing events to disk.
func (t *SENSETracer) IsEnabled() bool { return !t.disabled }

// Close flushes all buffered events, waits for drainLoop to finish, and
// closes the underlying trace file.  Subsequent LogXxx calls become no-ops.
func (t *SENSETracer) Close() {
	if t.writeCh != nil {
		close(t.writeCh) // signals drainLoop to flush remaining and exit
		t.wg.Wait()      // wait for full flush before returning
		t.writeCh = nil
	}
	t.disabled = true
}

// ── Internal helpers ───────────────────────────────────────────────────────

// truncateTo truncates s to at most max bytes (UTF-8 safe by re-slicing at a
// rune boundary) and appends "…" when truncation occurs.
func truncateTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Backtrack to a valid UTF-8 rune boundary.
	end := max
	for end > 0 && (s[end]&0xC0) == 0x80 {
		end--
	}
	return s[:end] + "…"
}

// classifyErrLabels derives semantic labels from an error message string.
// Labels are used by the trace analyzer for fast categorical filtering.
func classifyErrLabels(msg string) []string {
	labels := []string{"error"}
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		labels = append(labels, "timeout")
	}
	if strings.Contains(lower, "permission denied") || strings.Contains(lower, "not allowed") ||
		strings.Contains(lower, "permission error") {
		labels = append(labels, "permission")
	}
	if strings.Contains(lower, "not found") || strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "does not exist") {
		labels = append(labels, "not_found")
	}
	if strings.Contains(lower, "context length") || strings.Contains(lower, "context window") ||
		strings.Contains(lower, "token limit") || strings.Contains(lower, "max_tokens") ||
		strings.Contains(lower, "too long") || strings.Contains(lower, "context overflow") {
		labels = append(labels, "context_overflow")
	}
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") ||
		strings.Contains(lower, "quota exceeded") {
		labels = append(labels, "rate_limit")
	}
	if strings.Contains(lower, "hallucin") || strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no such tool") {
		labels = append(labels, "hallucination")
	}
	return labels
}
