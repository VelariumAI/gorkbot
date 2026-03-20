package cache

import (
	"github.com/velariumai/gorkbot/pkg/ai"
)

// OpenAICacheMinTokens is the minimum prompt length at which OpenAI's
// automatic prompt caching activates. Below this threshold caching is
// simply unavailable regardless of prompt structure.
const OpenAICacheMinTokens = 1024

// OptimiseForOpenAICache reorders msgs so that static content (system
// messages) always precedes dynamic content (user/assistant turns).
//
// OpenAI's automatic prompt caching uses prefix matching — the server caches
// the longest prefix seen in previous requests. Static content at the front
// maximises the cached prefix length. The function is a no-op when the
// messages are already correctly ordered.
//
// Rules applied (from OpenAI docs):
//  1. system messages → first
//  2. user / assistant / tool messages → follow in original order
//  3. The very last user message (the current prompt) → always last
//
// Returns a reordered copy; the original slice is not mutated.
func OptimiseForOpenAICache(msgs []ai.ConversationMessage) []ai.ConversationMessage {
	if len(msgs) == 0 {
		return msgs
	}

	system := make([]ai.ConversationMessage, 0, 4)
	rest := make([]ai.ConversationMessage, 0, len(msgs))

	for _, m := range msgs {
		if m.Role == "system" {
			system = append(system, m)
		} else {
			rest = append(rest, m)
		}
	}

	out := make([]ai.ConversationMessage, 0, len(msgs))
	out = append(out, system...)
	out = append(out, rest...)
	return out
}

// IsOpenAICacheWorthy returns true when the estimated token count of
// systemPrompt clears the automatic caching threshold.
func IsOpenAICacheWorthy(systemPrompt string) bool {
	return estimateTokens(systemPrompt) >= OpenAICacheMinTokens
}
