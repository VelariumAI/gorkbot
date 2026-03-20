// Package adaptive — routing_table.go
//
// RoutingTable provides regex-based source-to-agent binding with three-tier
// specificity, ported from the build-your-own-openclaw Step 11 pattern into
// idiomatic Go.
//
// The three tiers (lowest index wins) are:
//
//   Tier 0 — Exact match   : pattern contains no regex meta-characters.
//   Tier 1 — Specific regex: pattern has regex meta-characters but NOT ".*".
//   Tier 2 — Wildcard      : pattern contains ".*" (catchall).
//
// On each Route call, all bindings are evaluated. The match with the lowest
// tier wins; within the same tier, the first registered binding wins
// (registration order matters for tie-breaking).
//
// RoutingTable is designed to sit in front of the ARC Router as a reliable,
// zero-learning-required fallback that still fires even when ARC has not
// accumulated enough routing history.
//
// Usage:
//
//	rt := adaptive.NewRoutingTable()
//	rt.Add("telegram:12345678",  "personal-agent")   // Tier 0 — exact
//	rt.Add("discord:staff-.*",   "admin-agent")       // Tier 1 — specific
//	rt.Add(".*",                 "default-agent")      // Tier 2 — wildcard
//
//	agentID, ok := rt.Route("telegram:12345678") // → "personal-agent", true
//	agentID, ok  = rt.Route("discord:staff-ops") // → "admin-agent",    true
//	agentID, ok  = rt.Route("discord:general")   // → "default-agent",  true
package adaptive

import (
	"regexp"
	"strings"
	"sync"
)

// routingTier classifies the specificity of a pattern.
type routingTier int

const (
	tierExact    routingTier = 0 // no regex meta-characters
	tierSpecific routingTier = 1 // regex, but no ".*"
	tierWildcard routingTier = 2 // contains ".*"
)

// Binding is a single source-pattern → agent mapping.
type Binding struct {
	Pattern string
	AgentID string
	Tier    routingTier
	re      *regexp.Regexp
}

// RoutingEntry is the flat config representation (e.g. from GORKBOT.md or YAML).
type RoutingEntry struct {
	Pattern string `json:"pattern" yaml:"pattern"`
	AgentID string `json:"agent"   yaml:"agent"`
}

// RoutingTable maps source identifiers to agent IDs using ordered,
// tiered regex matching. Safe for concurrent use after all bindings are
// registered (the internal RWMutex allows concurrent reads).
type RoutingTable struct {
	mu       sync.RWMutex
	bindings []Binding // ordered by registration, sorted by tier on Add
}

// NewRoutingTable creates an empty RoutingTable.
func NewRoutingTable() *RoutingTable {
	return &RoutingTable{}
}

// Add registers a pattern → agentID binding.
// pattern is a full Go regexp (anchored with ^ and $ automatically).
// Duplicate patterns are silently ignored (first registration wins).
func (rt *RoutingTable) Add(pattern, agentID string) error {
	tier := classifyTier(pattern)

	// Anchor the pattern so "staff-.*" doesn't match "prefix-staff-foo".
	anchored := "^(?:" + pattern + ")$"
	re, err := regexp.Compile(anchored)
	if err != nil {
		return err
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Prevent duplicate patterns.
	for _, b := range rt.bindings {
		if b.Pattern == pattern {
			return nil // idempotent
		}
	}

	rt.bindings = append(rt.bindings, Binding{
		Pattern: pattern,
		AgentID: agentID,
		Tier:    tier,
		re:      re,
	})
	return nil
}

// AddFromConfig bulk-loads bindings from a slice of RoutingEntry (e.g. parsed
// from GORKBOT.md YAML front-matter or a JSON config file).
// Returns the first compilation error encountered, if any.
func (rt *RoutingTable) AddFromConfig(entries []RoutingEntry) error {
	for _, e := range entries {
		if err := rt.Add(e.Pattern, e.AgentID); err != nil {
			return err
		}
	}
	return nil
}

// Route returns the best-matching agentID for the given source string.
// "Best" means lowest tier (Tier 0 > Tier 1 > Tier 2); within a tier the
// first registered binding wins. Returns ("", false) if no binding matches.
func (rt *RoutingTable) Route(source string) (agentID string, ok bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	bestTier := routingTier(99)
	bestAgent := ""
	found := false

	for _, b := range rt.bindings {
		if b.Tier > bestTier {
			continue // already have a better match
		}
		if b.re.MatchString(source) {
			if !found || b.Tier < bestTier {
				bestTier = b.Tier
				bestAgent = b.AgentID
				found = true
			}
		}
	}

	return bestAgent, found
}

// Bindings returns a snapshot of all registered bindings (for /bindings debug command).
func (rt *RoutingTable) Bindings() []Binding {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]Binding, len(rt.bindings))
	copy(out, rt.bindings)
	return out
}

// Len returns the number of registered bindings.
func (rt *RoutingTable) Len() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.bindings)
}

// Remove deletes all bindings for the given pattern. Idempotent.
func (rt *RoutingTable) Remove(pattern string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := rt.bindings[:0]
	for _, b := range rt.bindings {
		if b.Pattern != pattern {
			out = append(out, b)
		}
	}
	rt.bindings = out
}

// classifyTier determines the tier of a pattern string.
func classifyTier(pattern string) routingTier {
	// Wildcard: contains ".*"
	if strings.Contains(pattern, ".*") {
		return tierWildcard
	}
	// Check for any regex meta-characters.
	metaChars := `\.+*?^${}[]|()`
	for _, c := range metaChars {
		if strings.ContainsRune(pattern, c) {
			return tierSpecific
		}
	}
	return tierExact
}
