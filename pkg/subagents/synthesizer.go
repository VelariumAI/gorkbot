package subagents

import (
	"context"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

// Conflict represents a contradiction between two agent results.
type Conflict struct {
	Claim   string // assertion present in one result
	Counter string // contradicting statement from another result
	SourceA string // source label of first result
	SourceB string // source label of second result
}

// SynthesisResult is the output of the Synthesizer.
type SynthesisResult struct {
	Consensus  string     // agreed facts and consolidated summary
	Conflicts  []Conflict // detected contradictions between agents
	Confidence float64    // 0-1 aggregate confidence score
	Sources    []string   // labels of contributing agent results
}

// SourcedResult is a single agent result with a label for attribution.
type SourcedResult struct {
	Label  string // human-readable label (e.g. "primary", "verifier", "redteam-recon")
	Output string // raw agent output
}

// ─────────────────────────────────────────────────────────────────────────────
// Synthesizer
// ─────────────────────────────────────────────────────────────────────────────

// Synthesizer combines multiple agent results into a coherent output.
// It uses a lightweight AI pass when a provider is available, and falls back
// to heuristic majority-vote when not.
type Synthesizer struct {
	provider ai.AIProvider // lightweight model for the synthesis pass (may be nil)
}

// NewSynthesizer creates a Synthesizer. provider may be nil; synthesis will
// be purely heuristic in that case.
func NewSynthesizer(provider ai.AIProvider) *Synthesizer {
	return &Synthesizer{provider: provider}
}

// Synthesize merges multiple SourcedResults into a SynthesisResult.
// When provider is set, it runs an AI summarisation pass on top of the
// heuristic conflict/consensus detection.
func (s *Synthesizer) Synthesize(ctx context.Context, results []SourcedResult) (*SynthesisResult, error) {
	if len(results) == 0 {
		return &SynthesisResult{Consensus: "No agent results to synthesize."}, nil
	}
	if len(results) == 1 {
		return &SynthesisResult{
			Consensus:  results[0].Output,
			Confidence: 1.0,
			Sources:    []string{results[0].Label},
		}, nil
	}

	// Extract source labels.
	sources := make([]string, len(results))
	for i, r := range results {
		sources[i] = r.Label
	}

	// Heuristic conflict detection.
	conflicts := detectConflicts(results)

	// Compute confidence based on agreement rate.
	confidence := computeConfidence(results, conflicts)

	// Build consensus via AI pass or heuristic fallback.
	var consensus string
	var err error
	if s.provider != nil {
		consensus, err = s.aiSynthesisPass(ctx, results, conflicts)
		if err != nil {
			// Degrade gracefully to heuristic.
			consensus = heuristicConsensus(results)
		}
	} else {
		consensus = heuristicConsensus(results)
	}

	return &SynthesisResult{
		Consensus:  consensus,
		Conflicts:  conflicts,
		Confidence: confidence,
		Sources:    sources,
	}, nil
}

// Format returns a human-readable synthesis report.
func (sr *SynthesisResult) Format() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Synthesis Report (confidence: %.0f%%)\n\n", sr.Confidence*100))
	sb.WriteString("### Consensus\n")
	sb.WriteString(sr.Consensus)
	sb.WriteString("\n")

	if len(sr.Conflicts) > 0 {
		sb.WriteString(fmt.Sprintf("\n### Conflicts Detected (%d)\n", len(sr.Conflicts)))
		for i, c := range sr.Conflicts {
			sb.WriteString(fmt.Sprintf("\n**Conflict %d** (%s vs %s)\n", i+1, c.SourceA, c.SourceB))
			sb.WriteString(fmt.Sprintf("- Claim: %s\n", c.Claim))
			sb.WriteString(fmt.Sprintf("- Counter: %s\n", c.Counter))
		}
	}

	sb.WriteString(fmt.Sprintf("\n*Sources: %s*\n", strings.Join(sr.Sources, ", ")))
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// aiSynthesisPass uses the lightweight provider to produce a final consensus.
func (s *Synthesizer) aiSynthesisPass(ctx context.Context, results []SourcedResult, conflicts []Conflict) (string, error) {
	var prompt strings.Builder
	prompt.WriteString("You are synthesizing outputs from multiple AI agents into a single coherent consensus.\n\n")

	for _, r := range results {
		prompt.WriteString(fmt.Sprintf("=== Agent: %s ===\n%s\n\n", r.Label, r.Output))
	}

	if len(conflicts) > 0 {
		prompt.WriteString("=== Detected Conflicts ===\n")
		for _, c := range conflicts {
			prompt.WriteString(fmt.Sprintf("- %s says: %q but %s says: %q\n", c.SourceA, c.Claim, c.SourceB, c.Counter))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("Write a concise synthesis: state the agreed facts, note any unresolved contradictions, and provide an overall assessment. Be factual and brief.")

	return s.provider.Generate(ctx, prompt.String())
}

// heuristicConsensus picks the longest/most detailed result as the baseline
// and prepends a brief note about the number of contributing agents.
func heuristicConsensus(results []SourcedResult) string {
	longest := results[0]
	for _, r := range results[1:] {
		if len(r.Output) > len(longest.Output) {
			longest = r
		}
	}

	if len(results) == 1 {
		return longest.Output
	}

	// Combine: use the longest as primary, append unique sentences from others.
	combined := longestFirstMerge(results)
	return combined
}

// longestFirstMerge uses the longest result as baseline and appends unique
// sentences from other results that don't appear in the baseline.
func longestFirstMerge(results []SourcedResult) string {
	// Find longest.
	bestIdx := 0
	for i, r := range results {
		if len(r.Output) > len(results[bestIdx].Output) {
			bestIdx = i
		}
	}
	base := results[bestIdx].Output

	// Collect unique additions from other results.
	var additions []string
	for i, r := range results {
		if i == bestIdx {
			continue
		}
		sentences := splitSentences(r.Output)
		for _, s := range sentences {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			// Only add if not already present in base.
			if !strings.Contains(strings.ToLower(base), strings.ToLower(s[:min(len(s), 40)])) {
				additions = append(additions, s)
			}
		}
	}

	if len(additions) == 0 {
		return base
	}
	return base + "\n\n**Additional findings:**\n" + strings.Join(additions, "\n")
}

// detectConflicts looks for simple negation patterns across result pairs.
// This is a heuristic — it detects obvious contradictions, not semantic ones.
func detectConflicts(results []SourcedResult) []Conflict {
	var conflicts []Conflict

	negationPairs := [][2]string{
		{"vulnerable", "not vulnerable"},
		{"found", "not found"},
		{"exists", "does not exist"},
		{"enabled", "disabled"},
		{"present", "absent"},
		{"success", "failed"},
		{"accessible", "not accessible"},
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			aLower := strings.ToLower(results[i].Output)
			bLower := strings.ToLower(results[j].Output)

			for _, pair := range negationPairs {
				aHasPos := strings.Contains(aLower, pair[0]) && !strings.Contains(aLower, pair[1])
				aHasNeg := strings.Contains(aLower, pair[1])
				bHasPos := strings.Contains(bLower, pair[0]) && !strings.Contains(bLower, pair[1])
				bHasNeg := strings.Contains(bLower, pair[1])

				if (aHasPos && bHasNeg) || (aHasNeg && bHasPos) {
					conflicts = append(conflicts, Conflict{
						Claim:   pair[0],
						Counter: pair[1],
						SourceA: results[i].Label,
						SourceB: results[j].Label,
					})
					break // one conflict per pair of results is enough
				}
			}
		}
	}

	return conflicts
}

// computeConfidence returns a score 0–1 based on agreement between results.
// High agreement → high confidence; many conflicts → lower confidence.
func computeConfidence(results []SourcedResult, conflicts []Conflict) float64 {
	if len(results) <= 1 {
		return 1.0
	}
	maxPairs := len(results) * (len(results) - 1) / 2
	if maxPairs == 0 {
		return 1.0
	}
	conflictRate := float64(len(conflicts)) / float64(maxPairs)
	return 1.0 - conflictRate*0.5 // never below 0.5 for non-empty results
}

// splitSentences splits text on sentence boundaries.
func splitSentences(text string) []string {
	// Simple split on ". ", "! ", "? ", "\n"
	text = strings.ReplaceAll(text, "! ", ".\n")
	text = strings.ReplaceAll(text, "? ", ".\n")
	text = strings.ReplaceAll(text, ". ", ".\n")
	return strings.Split(text, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
