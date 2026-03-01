package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// cacheEntry stores a cached tool result with expiry.
type cacheEntry struct {
	result    *ToolResult
	expiresAt time.Time
}

// ToolCache provides TTL-based result memoization for safe, read-only tools.
type ToolCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttls    map[string]time.Duration
}

// DefaultCacheTTLs lists tools that are safe to cache with their TTLs.
var DefaultCacheTTLs = map[string]time.Duration{
	"read_file":      5 * time.Minute,
	"file_info":      5 * time.Minute,
	"list_directory": 30 * time.Second,
	"web_fetch":      15 * time.Minute,
	"system_info":    5 * time.Minute,
	"disk_usage":     60 * time.Second,
	"git_status":     10 * time.Second,
	"git_log":        30 * time.Second,
	"env_var":        5 * time.Minute,
}

// MutationInvalidates maps mutation tools to the read tools they invalidate.
var MutationInvalidates = map[string]bool{
	"write_file":  true,
	"edit_file":   true,
	"delete_file": true,
	"git_commit":  true,
	"git_pull":    true,
	"bash":        true,
}

// NewToolCache creates a ToolCache with the default TTL table.
func NewToolCache() *ToolCache {
	return &ToolCache{
		entries: make(map[string]cacheEntry),
		ttls:    DefaultCacheTTLs,
	}
}

// cacheKey builds a deterministic cache key from tool name + parameters.
func cacheKey(toolName string, params map[string]interface{}) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	if params != nil {
		b, _ := json.Marshal(params)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves a cached result. Returns (result, true) if cache hit.
func (c *ToolCache) Get(toolName string, params map[string]interface{}) (*ToolResult, bool) {
	if _, ok := c.ttls[toolName]; !ok {
		return nil, false
	}
	key := cacheKey(toolName, params)
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()
	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

// Set stores a result in the cache for the tool's configured TTL.
func (c *ToolCache) Set(toolName string, params map[string]interface{}, result *ToolResult) {
	ttl, ok := c.ttls[toolName]
	if !ok {
		return
	}
	key := cacheKey(toolName, params)
	c.mu.Lock()
	c.entries[key] = cacheEntry{result: result, expiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// InvalidateOnMutation flushes the entire cache when a mutation tool runs.
func (c *ToolCache) InvalidateOnMutation(toolName string) {
	if MutationInvalidates[toolName] {
		c.Flush()
	}
}

// Flush removes all entries from the cache.
func (c *ToolCache) Flush() {
	c.mu.Lock()
	c.entries = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// Size returns the number of live (non-expired) cache entries.
func (c *ToolCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	count := 0
	for _, e := range c.entries {
		if now.Before(e.expiresAt) {
			count++
		}
	}
	return count
}
