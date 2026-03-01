// Package engine — ralph.go
//
// Ralph Loop: self-referential retry system.
//
// Inspired by oh-my-opencode's "ralph-loop" hook which was described as:
// "Agent gets stuck? Ralph loop delegates back to itself with:
//   - Previous attempt context
//   - What failed + why
//   - New approach guidance"
//
// In Gorkbot's context this means: when the AI's tool execution loop reaches a
// state of repeated failure (same tool failing, no progress, error loops), the
// Ralph loop intervenes and re-frames the conversation with a meta-prompt that
// summarises what has been tried, what went wrong, and asks the AI to approach
// the problem differently.
//
// This prevents the AI from getting locked in a failure cycle while still
// allowing autonomous recovery without requiring human intervention.
package engine

import (
	"fmt"
	"strings"
	"time"
)

// RalphConfig controls Ralph Loop behaviour.
type RalphConfig struct {
	// MaxIterations is the maximum number of retry cycles (default 3).
	MaxIterations int

	// FailureThreshold is the number of consecutive tool failures that trigger
	// a Ralph iteration (default 2).
	FailureThreshold int

	// Enabled controls whether the Ralph Loop is active (default true).
	Enabled bool
}

// DefaultRalphConfig returns sensible defaults.
func DefaultRalphConfig() RalphConfig {
	return RalphConfig{
		MaxIterations:    3,
		FailureThreshold: 2,
		Enabled:          true,
	}
}

// attemptRecord captures what was tried during a single Ralph iteration.
type attemptRecord struct {
	Iteration   int
	ToolsFailed []string
	Errors      []string
	Timestamp   time.Time
}

// RalphLoop tracks attempt history and builds retry meta-prompts.
type RalphLoop struct {
	cfg      RalphConfig
	attempts []attemptRecord
	current  *attemptRecord
}

// NewRalphLoop creates a RalphLoop with the given config.
func NewRalphLoop(cfg RalphConfig) *RalphLoop {
	return &RalphLoop{cfg: cfg}
}

// Begin starts tracking a new Ralph iteration cycle for a given user task.
// Call this at the start of each ExecuteTaskWithStreaming call.
func (r *RalphLoop) Begin() {
	r.current = &attemptRecord{
		Iteration: len(r.attempts) + 1,
		Timestamp: time.Now(),
	}
}

// RecordFailure notes a tool failure during the current attempt.
func (r *RalphLoop) RecordFailure(toolName, errMsg string) {
	if r.current == nil {
		return
	}
	r.current.ToolsFailed = append(r.current.ToolsFailed, toolName)
	r.current.Errors = append(r.current.Errors, fmt.Sprintf("%s: %s", toolName, errMsg))
}

// ShouldTrigger returns true when the Ralph Loop should intervene:
//   - enabled AND
//   - current iteration has ≥ FailureThreshold failures AND
//   - we haven't exhausted MaxIterations
func (r *RalphLoop) ShouldTrigger() bool {
	if !r.cfg.Enabled {
		return false
	}
	if len(r.attempts) >= r.cfg.MaxIterations {
		return false
	}
	if r.current == nil {
		return false
	}
	return len(r.current.ToolsFailed) >= r.cfg.FailureThreshold
}

// Commit finalises the current attempt record and saves it to history.
// Call this at the end of an execution loop regardless of success/failure.
func (r *RalphLoop) Commit() {
	if r.current != nil {
		r.attempts = append(r.attempts, *r.current)
		r.current = nil
	}
}

// IterationsUsed returns how many Ralph iterations have been committed.
func (r *RalphLoop) IterationsUsed() int { return len(r.attempts) }

// MaxIterations returns the configured limit.
func (r *RalphLoop) MaxIterations() int { return r.cfg.MaxIterations }

// BuildRetryPrompt constructs a meta-prompt that summarises past failures and
// instructs the AI to approach the problem with a fresh strategy.
// The returned prompt is prepended to the user's original message for the
// next streaming call.
func (r *RalphLoop) BuildRetryPrompt(originalPrompt string) string {
	if len(r.attempts) == 0 {
		return originalPrompt
	}

	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════════════════════\n")
	sb.WriteString("  RALPH LOOP — AUTONOMOUS RETRY\n")
	sb.WriteString(fmt.Sprintf("  Iteration %d / %d\n", len(r.attempts)+1, r.cfg.MaxIterations))
	sb.WriteString("═══════════════════════════════════════════════════════\n\n")

	sb.WriteString("Previous approach(es) did not succeed. Here is what was tried:\n\n")

	for _, attempt := range r.attempts {
		sb.WriteString(fmt.Sprintf("▸ Attempt %d", attempt.Iteration))
		if len(attempt.ToolsFailed) > 0 {
			sb.WriteString(fmt.Sprintf(" — %d failure(s):\n", len(attempt.ToolsFailed)))
			for _, e := range attempt.Errors {
				sb.WriteString(fmt.Sprintf("    • %s\n", e))
			}
		} else {
			sb.WriteString(" — completed without tool failures but did not resolve the task.\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString("═══════════════════════════════════════════════════════\n")
	sb.WriteString("INSTRUCTIONS FOR THIS RETRY:\n")
	sb.WriteString("1. Do NOT repeat the same approach that failed above.\n")
	sb.WriteString("2. Analyse WHY the previous attempt(s) failed before acting.\n")
	sb.WriteString("3. Choose a different strategy, tool sequence, or decomposition.\n")
	sb.WriteString("4. If the task is fundamentally ambiguous, ask a clarifying question.\n")
	sb.WriteString("5. If you identify that the task is impossible as stated, explain why clearly.\n")
	sb.WriteString("═══════════════════════════════════════════════════════\n\n")

	sb.WriteString("ORIGINAL TASK:\n")
	sb.WriteString(originalPrompt)

	return sb.String()
}

// Reset clears all attempt history. Call between unrelated user tasks.
func (r *RalphLoop) Reset() {
	r.attempts = nil
	r.current = nil
}

// Summary returns a one-line description of the Ralph Loop state for logging.
func (r *RalphLoop) Summary() string {
	if !r.cfg.Enabled {
		return "ralph:disabled"
	}
	total := 0
	for _, a := range r.attempts {
		total += len(a.ToolsFailed)
	}
	return fmt.Sprintf("ralph:iter=%d/%d failures=%d",
		len(r.attempts), r.cfg.MaxIterations, total)
}
