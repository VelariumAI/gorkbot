package engine

import (
	"crypto/md5"
	"strings"
)

// maxMemoryTokens is the total token budget for the [[MEMORY]] block.
const maxMemoryTokens = 1200

// pruneMemoryParts deduplicates and token-caps memory parts.
// Parts are included in input order until the budget is exhausted.
// Identical parts (MD5 match) are skipped regardless of budget.
// Returns a subset of parts safe to join into the [[MEMORY]] block.
func pruneMemoryParts(parts []string, maxTokens int) []string {
	const charsPerToken = 4
	seen := make(map[[16]byte]bool)
	var result []string
	used := 0

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		h := md5.Sum([]byte(p))
		if seen[h] {
			continue
		}
		seen[h] = true
		cost := (len(p) + charsPerToken - 1) / charsPerToken // ceiling div
		if used+cost > maxTokens {
			continue // skip over-budget entries (don't truncate mid-sentence)
		}
		result = append(result, p)
		used += cost
	}
	return result
}
