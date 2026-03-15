package dag

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// DefaultMaxLines is the maximum number of lines returned after compression.
	// 10 lines gives the LLM enough context without blowing the token budget.
	DefaultMaxLines = 10

	// DefaultMaxChars is the hard character ceiling after compression.
	DefaultMaxChars = 2000
)

// Pruner compresses verbose tool output before it is passed to the LLM or
// stored in the RCA report. This is the "Context Pruning" capability that
// distinguishes Gorkbot from Python frameworks which dump raw logs into every
// prompt, burning tokens and degrading response quality.
//
// Compression strategy (priority order):
//  1. Always include lines containing error/warning/fatal keywords.
//  2. Always include the last N lines (the "tail") — most recent context.
//  3. If the budget is still not exhausted, fill from the head.
//  4. Hard-truncate at MaxChars regardless.
type Pruner struct {
	MaxLines int
	MaxChars int
}

// NewPruner constructs a Pruner with explicit limits.
func NewPruner(maxLines, maxChars int) *Pruner {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	return &Pruner{MaxLines: maxLines, MaxChars: maxChars}
}

// errorLineRe matches lines that likely contain actionable failure information.
var errorLineRe = regexp.MustCompile(`(?i)\b(error|err:|fatal|panic|failed|exception|denied|timeout|refused|traceback|cannot|could not|no such|permission|unauthorized|invalid|unexpected)\b`)

// Compress reduces output to at most p.MaxLines lines and p.MaxChars characters,
// prioritising error/warning lines and the tail of the output.
//
// If the input is already within budget it is returned unchanged so there is
// zero overhead on short tool outputs.
func (p *Pruner) Compress(output string) string {
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")

	// Fast path: already within budget.
	if len(lines) <= p.MaxLines && len(output) <= p.MaxChars {
		return output
	}

	// Pass 1: collect high-signal lines (errors, warnings, fatals).
	var signalIdx []int
	for i, l := range lines {
		if errorLineRe.MatchString(l) {
			signalIdx = append(signalIdx, i)
		}
	}

	// Pass 2: always include the last tailCount lines.
	tailCount := p.MaxLines / 2
	if tailCount < 1 {
		tailCount = 1
	}
	tail := make(map[int]bool)
	start := len(lines) - tailCount
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		tail[i] = true
	}

	// Union: signal lines + tail lines, deduplicated and ordered.
	selected := make(map[int]bool)
	for _, i := range signalIdx {
		selected[i] = true
	}
	for i := range tail {
		selected[i] = true
	}

	// Fill remaining budget from the head.
	for i := 0; i < len(lines) && len(selected) < p.MaxLines; i++ {
		selected[i] = true
	}

	// Build result in original order, noting omissions.
	var kept []string
	lastKept := -1
	for i, l := range lines {
		if !selected[i] {
			continue
		}
		if lastKept >= 0 && i > lastKept+1 {
			kept = append(kept, fmt.Sprintf("  [... %d lines omitted ...]", i-lastKept-1))
		}
		kept = append(kept, l)
		lastKept = i
	}

	result := strings.Join(kept, "\n")

	// Hard character ceiling.
	if len(result) > p.MaxChars {
		result = result[:p.MaxChars-3] + "..."
	}
	return result
}

// CompressWithHeader wraps Compress with a summary header that tells the LLM
// how many lines were omitted, so it understands why context is truncated.
func (p *Pruner) CompressWithHeader(label, output string) string {
	total := strings.Count(output, "\n") + 1
	compressed := p.Compress(output)
	shown := strings.Count(compressed, "\n") + 1

	if shown >= total {
		return compressed
	}
	header := fmt.Sprintf("[%s — showing %d of %d lines]\n", label, shown, total)
	return header + compressed
}
