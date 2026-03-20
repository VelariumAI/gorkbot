package cache

// GrokCacheHeaders returns the HTTP headers to inject into an xAI/Grok request
// to maximise automatic prompt-cache hit rates.
//
// xAI's caching is fully automatic — the server caches prompt prefix KV states
// without any explicit API call. The only client-side knob is x-grok-conv-id:
// providing a stable UUID4 for the session routes all requests to the same
// backend server, boosting hit rates above 90% for multi-turn conversations.
//
// Cache hits appear in the response usage object:
//
//	usage.prompt_tokens_details.cached_tokens
//
// Best practices (per xAI docs):
//   - Never modify earlier messages — only append new ones.
//   - Any edit, removal, or reorder of prior messages breaks the cache.
//   - Standardise system prompts across turns (avoid per-turn dynamic content
//     in the system prompt; use user messages for dynamic context instead).
func GrokCacheHeaders(convID string) map[string]string {
	if convID == "" {
		return nil
	}
	return map[string]string{
		"x-grok-conv-id": convID,
	}
}
