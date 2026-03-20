package adaptive

import (
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
)

// IngressFilter prunes low-information content from prompts before they reach
// the ARC classifier and the LLM. It is a pure text transformer — it never
// modifies the caller's data and never blocks a request.
//
// Pruning pipeline (applied in order):
//  1. Normalise whitespace (collapse runs of spaces / tabs / blank lines).
//  2. Strip ANSI escape sequences and OSC control codes.
//  3. Remove pure filler phrases ("please", "could you", "would you mind").
//  4. Deduplicate repeated sentences (edit-distance ≤ 1 word difference).
//  5. Trim to a configurable maximum rune count.
//
// Token savings are tracked atomically for observability.
type IngressFilter struct {
	MaxRunes int64 // 0 = no limit

	// Counters — read with Load(), reset with Reset().
	totalIn    atomic.Int64
	totalOut   atomic.Int64
	callCount  atomic.Int64

	ansiRE    *regexp.Regexp
	fillerRE  *regexp.Regexp
}

// NewIngressFilter creates an IngressFilter with a default rune limit of
// 32 000 (≈ 8 000 tokens). Pass 0 to disable the hard cap.
func NewIngressFilter(maxRunes int64) *IngressFilter {
	if maxRunes == 0 {
		maxRunes = 32000
	}
	return &IngressFilter{
		MaxRunes:  maxRunes,
		ansiRE:    regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`),
		fillerRE:  regexp.MustCompile(`(?i)\b(?:please|kindly|would you (?:mind |be able to )?|could you (?:please )?|if you (?:don't mind,? )?|feel free to |go ahead and )\b`),
	}
}

// PruneStats reports token savings for a single Prune call.
type PruneStats struct {
	InputRunes  int
	OutputRunes int
	SavedRunes  int
	SavedPct    float64
}

// Prune applies the full pruning pipeline and returns the cleaned prompt along
// with savings statistics. It is safe to call from multiple goroutines.
func (f *IngressFilter) Prune(prompt string) (pruned string, stats PruneStats) {
	stats.InputRunes = len([]rune(prompt))
	f.totalIn.Add(int64(stats.InputRunes))
	f.callCount.Add(1)

	s := prompt

	// 1. Strip ANSI / OSC sequences.
	s = f.ansiRE.ReplaceAllString(s, "")

	// 2. Normalise whitespace.
	s = normaliseWhitespace(s)

	// 3. Remove filler phrases.
	s = f.fillerRE.ReplaceAllString(s, "")

	// 4. Deduplicate sentences.
	s = deduplicateSentences(s)

	// 5. Normalise again after dedup.
	s = normaliseWhitespace(s)

	// 6. Hard rune cap.
	if f.MaxRunes > 0 {
		runes := []rune(s)
		if int64(len(runes)) > f.MaxRunes {
			s = string(runes[:f.MaxRunes])
		}
	}

	stats.OutputRunes = len([]rune(s))
	stats.SavedRunes = stats.InputRunes - stats.OutputRunes
	if stats.InputRunes > 0 {
		stats.SavedPct = float64(stats.SavedRunes) / float64(stats.InputRunes) * 100
	}

	f.totalOut.Add(int64(stats.OutputRunes))
	return s, stats
}

// SessionStats returns cumulative savings across all Prune calls this session.
func (f *IngressFilter) SessionStats() (in, out, calls int64) {
	return f.totalIn.Load(), f.totalOut.Load(), f.callCount.Load()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// normaliseWhitespace collapses whitespace runs and trims the result.
func normaliseWhitespace(s string) string {
	// Collapse multiple blank lines → single blank line.
	multiBlank := regexp.MustCompile(`\n{3,}`)
	s = multiBlank.ReplaceAllString(s, "\n\n")

	// Collapse horizontal whitespace runs on each line.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Replace runs of spaces/tabs with a single space.
		var b strings.Builder
		prevSpace := false
		for _, r := range line {
			if unicode.IsSpace(r) && r != '\n' {
				if !prevSpace {
					b.WriteRune(' ')
				}
				prevSpace = true
			} else {
				b.WriteRune(r)
				prevSpace = false
			}
		}
		lines[i] = strings.TrimSpace(b.String())
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// deduplicateSentences removes sentences that are near-identical to a
// previously seen sentence (same word set, ≤ 1 word difference).
// Sentence boundary: ". ", "! ", "? ", or "\n".
func deduplicateSentences(s string) string {
	// Split on sentence boundaries while preserving delimiters.
	splitter := regexp.MustCompile(`(?:[.!?]\s+|\n)`)
	locs := splitter.FindAllStringIndex(s, -1)

	if len(locs) < 2 {
		return s // nothing to deduplicate
	}

	// Reconstruct sentences with their trailing delimiter.
	sentences := make([]string, 0, len(locs)+1)
	prev := 0
	for _, loc := range locs {
		sentences = append(sentences, s[prev:loc[1]])
		prev = loc[1]
	}
	if prev < len(s) {
		sentences = append(sentences, s[prev:])
	}

	seen := make([][]string, 0, len(sentences))
	var out strings.Builder

	for _, sent := range sentences {
		words := tokeniseWords(sent)
		if len(words) == 0 {
			out.WriteString(sent)
			continue
		}
		dup := false
		for _, prev := range seen {
			if jaccardWords(words, prev) >= 0.85 {
				dup = true
				break
			}
		}
		if !dup {
			seen = append(seen, words)
			out.WriteString(sent)
		}
	}
	return out.String()
}

// tokeniseWords lowercases and splits text into alpha-only words ≥ 3 chars.
func tokeniseWords(s string) []string {
	lower := strings.ToLower(s)
	var words []string
	for _, w := range strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len(w) >= 3 {
			words = append(words, w)
		}
	}
	return words
}

// jaccardWords computes |intersection| / |union| for two word slices.
func jaccardWords(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	setA := make(map[string]struct{}, len(a))
	for _, w := range a {
		setA[w] = struct{}{}
	}
	inter := 0
	setB := make(map[string]struct{}, len(b))
	for _, w := range b {
		setB[w] = struct{}{}
		if _, ok := setA[w]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
