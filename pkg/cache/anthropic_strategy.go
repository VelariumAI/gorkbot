package cache

import (
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// anthropicMinTokens returns the minimum cacheable token count for a given
// Claude model. Prompts shorter than this floor cannot be cached even when
// marked with cache_control — the API silently ignores the breakpoint.
// Values are sourced from the Anthropic prompt-caching documentation (2026).
func anthropicMinTokens(model string) int {
	m := strings.ToLower(model)
	switch {
	// Claude Sonnet 4.6 — 2 048
	case strings.Contains(m, "sonnet-4-6"), strings.Contains(m, "sonnet-4.6"):
		return 2048
	// Claude Opus 4.x — 4 096
	case strings.Contains(m, "opus-4"):
		return 4096
	// Claude Haiku 4.5 — 4 096
	case strings.Contains(m, "haiku-4-5"), strings.Contains(m, "haiku-4.5"):
		return 4096
	// Claude Haiku 3.5 / 3 — 2 048
	case strings.Contains(m, "haiku-3"):
		return 2048
	// Claude Sonnet 3.x / 4.x (older) — 1 024
	case strings.Contains(m, "sonnet-3"), strings.Contains(m, "sonnet-4"):
		return 1024
	// Claude Opus 3 — 1 024
	case strings.Contains(m, "opus-3"):
		return 1024
	default:
		// Conservative default: use the lowest common floor.
		return 1024
	}
}

// estimateTokens is a lightweight token estimator (~4 chars per token).
// The full EstimateTokens on ConversationHistory is heavier; this is used
// only to check whether a content block clears the caching floor.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// anthropicBreakpoints computes the message indices that should receive a
// cache_control breakpoint for Anthropic-compatible providers.
//
// Strategy (derived from Anthropic best-practice docs):
//  1. Always attempt to cache the system prompt (index 0 of the messages
//     array as built inside the provider; signalled by index -1 here so the
//     caller knows to mark the system block).
//  2. Mark up to 2 recent user messages if they individually clear the floor.
//  3. Cap total breakpoints at 4 (MiniMax hard limit; Anthropic allows more
//     but 4 is the safe cross-provider maximum).
//
// Returns a slice of indices into msgs (not the full wire array). The caller
// is responsible for mapping these to the actual request payload.
func anthropicBreakpoints(model, systemPrompt string, msgs []ai.ConversationMessage) []int {
	floor := anthropicMinTokens(model)
	var breakpoints []int

	// Index -1 is the sentinel for "mark the system prompt block".
	if estimateTokens(systemPrompt) >= floor {
		breakpoints = append(breakpoints, -1)
	}

	// Walk backwards through msgs to find recent user turns worth caching.
	marked := 0
	for i := len(msgs) - 1; i >= 0 && marked < 2 && len(breakpoints) < 4; i-- {
		m := msgs[i]
		if m.Role != "user" {
			continue
		}
		if estimateTokens(m.Content) >= floor {
			breakpoints = append(breakpoints, i)
			marked++
		}
	}

	return breakpoints
}
