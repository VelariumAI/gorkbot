package sense

// trace_analyzer.go — SENSE Autonomous Trace Analyzer
//
// Implements /self check: reads all JSONL trace files from the traces
// directory and classifies failure events into three canonical categories
// defined by Poehnelt (2026):
//
//   1. NeuralHallucination — the agent fabricated a tool name, a file path,
//      or a factual claim that was demonstrably false.
//
//   2. ToolFailure — a tool returned success=false or an error that was NOT
//      caused by a context overflow.
//
//   3. ContextOverflow — the model's context window was exceeded, causing
//      truncation or rejection by the AI provider.
//
// The analyzer also records SanitizerRejects (stabilization middleware hits)
// as a fourth category for operational monitoring.
//
// Usage:
//
//   a := sense.NewTraceAnalyzer(traceDir)
//   report, err := a.Analyze()
//
// The returned AnalysisReport is safe for JSON serialization and is consumed
// directly by the SkillEvolver to generate SKILL.md invariant files.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FailureCategory is the semantic classification of a detected failure.
type FailureCategory string

const (
	// CatHallucination — AI referenced something that does not exist.
	CatHallucination FailureCategory = "NeuralHallucination"
	// CatToolFailure — tool returned an error unrelated to context overflow.
	CatToolFailure FailureCategory = "ToolFailure"
	// CatContextOverflow — context window exhausted.
	CatContextOverflow FailureCategory = "ContextOverflow"
	// CatSanitizerReject — stabilization middleware rejected an input.
	CatSanitizerReject FailureCategory = "SanitizerReject"
	// CatProviderError — transient AI provider outage or rate limit.
	CatProviderError FailureCategory = "ProviderError"
)

// FailureEvent is a single classified failure extracted from the trace log.
type FailureEvent struct {
	// Timestamp of the original trace event.
	Timestamp time.Time `json:"timestamp"`
	// Category is the semantic classification.
	Category FailureCategory `json:"category"`
	// Tool is the tool name involved (empty for provider errors).
	Tool string `json:"tool,omitempty"`
	// SourceFile is the JSONL trace file the event was read from.
	SourceFile string `json:"source_file"`
	// Error is the error string from the trace event.
	Error string `json:"error,omitempty"`
	// Labels are the raw labels from the trace event.
	Labels []string `json:"labels,omitempty"`
	// DurationMS is the execution duration (for tool failures).
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// ToolFailurePattern groups repeated failures of the same tool.
type ToolFailurePattern struct {
	// Tool is the tool name.
	Tool string `json:"tool"`
	// Count is the number of failures.
	Count int `json:"count"`
	// Category is the dominant failure category for this tool.
	Category FailureCategory `json:"category"`
	// CommonErrors are the most frequent error substrings (up to 5).
	CommonErrors []string `json:"common_errors"`
	// FirstSeen is the earliest failure timestamp.
	FirstSeen time.Time `json:"first_seen"`
	// LastSeen is the most recent failure timestamp.
	LastSeen time.Time `json:"last_seen"`
}

// AnalysisReport is the structured output of a trace analysis run.
type AnalysisReport struct {
	// AnalyzedAt is when the analysis was run.
	AnalyzedAt time.Time `json:"analyzed_at"`
	// TraceDir is the directory that was scanned.
	TraceDir string `json:"trace_dir"`
	// FilesScanned is the number of JSONL files read.
	FilesScanned int `json:"files_scanned"`
	// TotalEvents is the total number of trace events parsed.
	TotalEvents int `json:"total_events"`
	// FailureEvents is the list of all classified failure events.
	FailureEvents []FailureEvent `json:"failure_events"`
	// Patterns is a summary of repeated tool failure patterns.
	Patterns []ToolFailurePattern `json:"patterns"`
	// Summary is a human-readable markdown summary.
	Summary string `json:"summary"`
	// Counts is a per-category count of failure events.
	Counts map[FailureCategory]int `json:"counts"`
}

// TraceAnalyzer reads and classifies SENSE trace events.
type TraceAnalyzer struct {
	traceDir string
}

// NewTraceAnalyzer creates an analyzer that reads from traceDir.
func NewTraceAnalyzer(traceDir string) *TraceAnalyzer {
	return &TraceAnalyzer{traceDir: traceDir}
}

// Analyze scans all *.jsonl files in the trace directory, classifies each
// failure event, and returns an AnalysisReport.
func (a *TraceAnalyzer) Analyze() (*AnalysisReport, error) {
	report := &AnalysisReport{
		AnalyzedAt: time.Now().UTC(),
		TraceDir:   a.traceDir,
		Counts:     make(map[FailureCategory]int),
	}

	// Collect JSONL files using filepath.WalkDir (platform-agnostic).
	var files []string
	walkErr := filepath.WalkDir(a.traceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Tolerate individual access errors; continue scanning.
			return nil
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, fmt.Errorf("SENSE analyzer: walk %q failed: %w", a.traceDir, walkErr)
	}
	if os.IsNotExist(walkErr) || len(files) == 0 {
		report.Summary = "No trace files found. Run gorkbot with tool calls to generate traces."
		return report, nil
	}

	sort.Strings(files) // chronological order (date-named files sort correctly)

	// Parse each file.
	for _, fpath := range files {
		n, events, err := a.parseFile(fpath)
		if err != nil {
			// Skip unreadable files without aborting the whole run.
			continue
		}
		report.FilesScanned++
		report.TotalEvents += n
		report.FailureEvents = append(report.FailureEvents, events...)
	}

	// Classify and count.
	for _, ev := range report.FailureEvents {
		report.Counts[ev.Category]++
	}

	// Build tool-level failure patterns.
	report.Patterns = buildPatterns(report.FailureEvents)

	// Render a human-readable markdown summary.
	report.Summary = renderSummary(report)

	return report, nil
}

// parseFile reads one JSONL file and returns (total events scanned, failure events).
func (a *TraceAnalyzer) parseFile(path string) (int, []FailureEvent, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	baseName := filepath.Base(path)
	var failures []FailureEvent
	total := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024) // 256 KB per line
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		total++

		var ev SENSETrace
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // malformed line — skip
		}

		cat, ok := classifyEvent(&ev)
		if !ok {
			continue // not a failure event
		}

		ts, _ := time.Parse(time.RFC3339Nano, ev.Timestamp)
		failures = append(failures, FailureEvent{
			Timestamp:  ts,
			Category:   cat,
			Tool:       ev.ToolName,
			SourceFile: baseName,
			Error:      ev.Error,
			Labels:     ev.Labels,
			DurationMS: ev.DurationMS,
		})
	}

	return total, failures, scanner.Err()
}

// classifyEvent maps a SENSETrace event to a FailureCategory.
// Returns (category, true) for failure events, ("", false) for success events.
func classifyEvent(ev *SENSETrace) (FailureCategory, bool) {
	switch ev.Kind {
	case KindHallucination:
		return CatHallucination, true

	case KindContextOverflow:
		return CatContextOverflow, true

	case KindSanitizerReject:
		return CatSanitizerReject, true

	case KindProviderError:
		return CatProviderError, true

	case KindToolFailure, KindParamError:
		// Distinguish context overflow embedded in tool failures.
		if hasLabel(ev.Labels, "context_overflow") || isContextOverflowMsg(ev.Error) {
			return CatContextOverflow, true
		}
		if hasLabel(ev.Labels, "hallucination") || isHallucinationMsg(ev.Error, ev.ToolName) {
			return CatHallucination, true
		}
		return CatToolFailure, true

	default:
		return "", false
	}
}

// ── Pattern aggregation ────────────────────────────────────────────────────

// buildPatterns groups FailureEvents by tool and produces ToolFailurePatterns
// with dominant category, common errors, and time range.
func buildPatterns(events []FailureEvent) []ToolFailurePattern {
	type bucket struct {
		category FailureCategory
		errors   []string
		first    time.Time
		last     time.Time
		counts   map[FailureCategory]int
	}

	byTool := make(map[string]*bucket)
	for _, ev := range events {
		key := ev.Tool
		if key == "" {
			key = "__provider__"
		}
		b, ok := byTool[key]
		if !ok {
			b = &bucket{
				first:  ev.Timestamp,
				last:   ev.Timestamp,
				counts: make(map[FailureCategory]int),
			}
			byTool[key] = b
		}
		b.counts[ev.Category]++
		if ev.Error != "" {
			b.errors = append(b.errors, ev.Error)
		}
		if ev.Timestamp.Before(b.first) {
			b.first = ev.Timestamp
		}
		if ev.Timestamp.After(b.last) {
			b.last = ev.Timestamp
		}
	}

	var patterns []ToolFailurePattern
	for tool, b := range byTool {
		// Determine dominant category.
		dom := dominantCategory(b.counts)
		total := 0
		for _, c := range b.counts {
			total += c
		}

		patterns = append(patterns, ToolFailurePattern{
			Tool:         tool,
			Count:        total,
			Category:     dom,
			CommonErrors: topErrors(b.errors, 5),
			FirstSeen:    b.first,
			LastSeen:     b.last,
		})
	}

	// Sort by count descending for prominence.
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})
	return patterns
}

// dominantCategory returns the category with the highest count.
func dominantCategory(counts map[FailureCategory]int) FailureCategory {
	var best FailureCategory
	max := 0
	for cat, n := range counts {
		if n > max {
			max = n
			best = cat
		}
	}
	return best
}

// topErrors returns up to n unique error prefix substrings (first 100 chars).
func topErrors(errs []string, n int) []string {
	seen := make(map[string]bool)
	var out []string
	for _, e := range errs {
		key := e
		if len(key) > 100 {
			key = key[:100]
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

// ── Classifier helpers ─────────────────────────────────────────────────────

func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// isContextOverflowMsg returns true when the error message contains known
// context-overflow signals from major AI providers.
func isContextOverflowMsg(msg string) bool {
	lower := strings.ToLower(msg)
	signals := []string{
		"context length", "context window", "context_length_exceeded",
		"maximum context", "max tokens", "token limit", "too many tokens",
		"input is too long", "tokens exceeds", "reduce your prompt",
		"context_too_long", "request too large",
	}
	for _, s := range signals {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// isHallucinationMsg returns true when error signals indicate the agent
// referenced something non-existent (hallucination pattern).
func isHallucinationMsg(msg, tool string) bool {
	lower := strings.ToLower(msg)
	signals := []string{
		"no such tool", "tool not found", "unknown tool",
		"does not exist", "file not found", "no such file",
		"fabricated", "hallucin",
	}
	for _, s := range signals {
		if strings.Contains(lower, s) {
			return true
		}
	}
	// A "tool not found" error for a non-empty tool name is a hallucination:
	// the agent invented a tool name.
	if tool != "" && strings.Contains(lower, "not found") {
		return true
	}
	return false
}

// ── Summary renderer ───────────────────────────────────────────────────────

// renderSummary generates a human-readable Markdown report of the analysis.
func renderSummary(r *AnalysisReport) string {
	var sb strings.Builder

	sb.WriteString("# SENSE Trace Analysis Report\n\n")
	sb.WriteString(fmt.Sprintf("**Analyzed:** %s  \n", r.AnalyzedAt.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("**Trace Dir:** `%s`  \n", r.TraceDir))
	sb.WriteString(fmt.Sprintf("**Files Scanned:** %d  \n", r.FilesScanned))
	sb.WriteString(fmt.Sprintf("**Total Events:** %d  \n\n", r.TotalEvents))

	totalFailures := len(r.FailureEvents)
	if totalFailures == 0 {
		sb.WriteString("✅ **No failures detected.** The agent is operating within normal parameters.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("## Failure Summary (%d total)\n\n", totalFailures))
	sb.WriteString("| Category | Count |\n")
	sb.WriteString("|----------|-------|\n")

	catOrder := []FailureCategory{
		CatHallucination, CatToolFailure, CatContextOverflow,
		CatSanitizerReject, CatProviderError,
	}
	for _, cat := range catOrder {
		if n := r.Counts[cat]; n > 0 {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", cat, n))
		}
	}
	sb.WriteString("\n")

	if len(r.Patterns) > 0 {
		sb.WriteString("## Top Failure Patterns\n\n")
		limit := 10
		if len(r.Patterns) < limit {
			limit = len(r.Patterns)
		}
		for _, p := range r.Patterns[:limit] {
			toolDisplay := p.Tool
			if toolDisplay == "__provider__" {
				toolDisplay = "(provider)"
			}
			sb.WriteString(fmt.Sprintf("### `%s` — %d × %s\n", toolDisplay, p.Count, p.Category))
			if len(p.CommonErrors) > 0 {
				sb.WriteString("**Sample errors:**\n")
				for _, e := range p.CommonErrors {
					sb.WriteString(fmt.Sprintf("- `%s`\n", e))
				}
			}
			sb.WriteString(fmt.Sprintf("*Range: %s → %s*\n\n",
				p.FirstSeen.Format("2006-01-02"),
				p.LastSeen.Format("2006-01-02"),
			))
		}
	}

	sb.WriteString("---\n")
	sb.WriteString("Run `/self evolve` to generate SKILL.md invariant files for the top patterns.\n")

	return sb.String()
}
