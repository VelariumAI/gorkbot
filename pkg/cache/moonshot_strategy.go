package cache

// MoonshotCacheClient is a best-effort stub for Moonshot/Kimi context caching.
//
// Moonshot's context caching works as an upload-once / tag-reference model:
// a large static context is uploaded once, assigned a tag/ID, and referenced
// in subsequent calls to avoid re-transmitting the full content. The exact
// REST API format (endpoint, request schema, header names) has not been
// definitively confirmed from Moonshot's public documentation.
//
// This client degrades gracefully to a no-op. When Moonshot's API is
// confirmed, replace the stub methods with real implementations following
// the same pattern as GeminiCacheClient (create / reference / delete
// lifecycle with a resource ID returned by the create call).
//
// Billing note: during public beta, Moonshot charges:
//   - Cache creation: 24 CNY / M tokens
//   - Cache storage:  10 CNY / M tokens / minute
//   - Cache hit:      0.02 CNY / call
type MoonshotCacheClient struct{}

// Create is a no-op until the Moonshot context caching API is confirmed.
// Returns ("", nil) so callers can treat this identically to a cache miss.
func (m *MoonshotCacheClient) Create(systemPrompt string) (cacheID string, err error) {
	return "", nil
}

// Reference returns the cache ID to inject into a Moonshot request, or ""
// when no cache has been created.
func (m *MoonshotCacheClient) Reference() string {
	return ""
}

// Delete is a no-op until the API is confirmed.
func (m *MoonshotCacheClient) Delete() {}
