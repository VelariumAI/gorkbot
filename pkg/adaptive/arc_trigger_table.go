package adaptive

import "strings"

// TriggerEntry maps a file-path prefix pattern to a Tier 2 specialist domain name.
// When the orchestrator detects that a task involves files matching the pattern,
// it loads the corresponding specialist persona from the CCI system.
type TriggerEntry struct {
	// PathPattern is a prefix or substring of a file path (case-insensitive).
	PathPattern string
	// Domain is the CCI Tier 2 specialist domain identifier.
	Domain string
	// Priority is the specificity score; higher values win when multiple entries match.
	Priority int
}

// TriggerTable defines the Orchestration Trigger Table —
// the authoritative mapping of code locations to specialist domains.
// Entries are evaluated in order; the highest-priority match wins.
var TriggerTable = []TriggerEntry{
	// Highly specific subsystems (priority 10)
	{PathPattern: "internal/tui/", Domain: "tui", Priority: 10},
	{PathPattern: "internal/arc/", Domain: "arc-mel", Priority: 10},
	{PathPattern: "internal/mel/", Domain: "arc-mel", Priority: 10},
	{PathPattern: "pkg/cci/", Domain: "cci", Priority: 10},
	{PathPattern: "pkg/mcp/", Domain: "mcp-integration", Priority: 10},
	{PathPattern: "pkg/sense/", Domain: "sense", Priority: 10},
	{PathPattern: "pkg/memory/", Domain: "memory", Priority: 10},
	{PathPattern: "pkg/subagents/", Domain: "subagents", Priority: 10},
	{PathPattern: "pkg/session/", Domain: "session", Priority: 10},
	{PathPattern: "pkg/providers/", Domain: "providers", Priority: 10},
	{PathPattern: "pkg/commands/", Domain: "commands", Priority: 10},

	// Broad subsystems (priority 8)
	{PathPattern: "internal/engine/", Domain: "orchestrator", Priority: 8},
	{PathPattern: "cmd/gorkbot/", Domain: "orchestrator", Priority: 8},
	{PathPattern: "pkg/ai/", Domain: "ai-providers", Priority: 8},
	{PathPattern: "pkg/tools/", Domain: "tool-system", Priority: 8},

	// Security tooling (priority 9 — more specific within pkg/tools)
	{PathPattern: "pkg/tools/security", Domain: "security", Priority: 9},
	{PathPattern: "pkg/security/", Domain: "security", Priority: 9},
}

// MatchTrigger returns the specialist domain that best matches the given prompt
// or file path. It scans all trigger table entries and returns the domain of the
// highest-priority match. Returns "" if no entry matches.
func MatchTrigger(text string) string {
	lower := strings.ToLower(text)
	best := ""
	bestPri := -1

	for _, entry := range TriggerTable {
		if strings.Contains(lower, strings.ToLower(entry.PathPattern)) {
			if entry.Priority > bestPri {
				bestPri = entry.Priority
				best = entry.Domain
			}
		}
	}
	return best
}

// MatchTriggerAll returns all matching specialist domains for the given text,
// de-duplicated and ordered by priority (highest first).
func MatchTriggerAll(text string) []string {
	lower := strings.ToLower(text)
	seen := make(map[string]int) // domain → max priority

	for _, entry := range TriggerTable {
		if strings.Contains(lower, strings.ToLower(entry.PathPattern)) {
			if p, ok := seen[entry.Domain]; !ok || entry.Priority > p {
				seen[entry.Domain] = entry.Priority
			}
		}
	}

	// Build result sorted by priority descending.
	type dp struct {
		domain   string
		priority int
	}
	var results []dp
	for d, p := range seen {
		results = append(results, dp{d, p})
	}
	// Simple insertion sort (table is small).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].priority > results[j-1].priority; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	domains := make([]string, 0, len(results))
	for _, r := range results {
		domains = append(domains, r.domain)
	}
	return domains
}
