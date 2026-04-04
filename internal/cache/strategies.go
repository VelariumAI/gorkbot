package cache

import (
	"time"
)

// DefaultCache is the basic caching strategy
type DefaultCache struct{}

func NewDefaultCache() CacheStrategy {
	return &DefaultCache{}
}

func (dc *DefaultCache) Name() string {
	return "default"
}

func (dc *DefaultCache) CanCache(provider string, content string) bool {
	// Cache if content is reasonably sized
	return len(content) > 100 && len(content) < 1000000
}

func (dc *DefaultCache) GetCacheKey(provider string, content string) string {
	return provider + ":" + ComputeHash(content)
}

func (dc *DefaultCache) GetTTL(provider string) time.Duration {
	// Default: 1 hour
	return 1 * time.Hour
}

func (dc *DefaultCache) GetMaxSize(provider string) int {
	// Default: 1000 entries per provider
	return 1000
}

func (dc *DefaultCache) UpdateOnAccess(entry *CacheEntry) error {
	entry.AccessCount++
	entry.LastAccessedAt = time.Now()
	return nil
}

func (dc *DefaultCache) EvictionPolicy(entries []*CacheEntry) *CacheEntry {
	if len(entries) == 0 {
		return nil
	}

	// LRU: Evict least recently accessed
	lru := entries[0]
	for _, entry := range entries[1:] {
		if entry.LastAccessedAt.Before(lru.LastAccessedAt) {
			lru = entry
		}
	}
	return lru
}

// ClaudeCache is Claude-specific caching strategy
type ClaudeCache struct{}

func NewClaudeCache() CacheStrategy {
	return &ClaudeCache{}
}

func (cc *ClaudeCache) Name() string {
	return "claude"
}

func (cc *ClaudeCache) CanCache(provider string, content string) bool {
	// Claude supports prompt caching for system prompts and long contexts
	// Only cache if reasonably long (Claude's cache kicks in after ~1024 tokens)
	return len(content) > 500 && len(content) < 5000000
}

func (cc *ClaudeCache) GetCacheKey(provider string, content string) string {
	// Claude caches based on exact content, including whitespace
	return "claude:" + ComputeHash(content)
}

func (cc *ClaudeCache) GetTTL(provider string) time.Duration {
	// Claude prompt cache lasts 5 minutes for API access
	return 5 * time.Minute
}

func (cc *ClaudeCache) GetMaxSize(provider string) int {
	// Claude allows up to 1000 cache entries
	return 1000
}

func (cc *ClaudeCache) UpdateOnAccess(entry *CacheEntry) error {
	entry.AccessCount++
	entry.LastAccessedAt = time.Now()
	// Refresh expiration on access (sliding window)
	entry.ExpiresAt = time.Now().Add(cc.GetTTL("claude"))
	return nil
}

func (cc *ClaudeCache) EvictionPolicy(entries []*CacheEntry) *CacheEntry {
	if len(entries) == 0 {
		return nil
	}

	// LFU: Evict least frequently used (lowest access count)
	lfu := entries[0]
	for _, entry := range entries[1:] {
		if entry.AccessCount < lfu.AccessCount {
			lfu = entry
		}
	}
	return lfu
}

// OpenAICache is OpenAI-specific caching strategy
type OpenAICache struct{}

func NewOpenAICache() CacheStrategy {
	return &OpenAICache{}
}

func (oc *OpenAICache) Name() string {
	return "openai"
}

func (oc *OpenAICache) CanCache(provider string, content string) bool {
	// OpenAI caches based on prompt caching (>1024 tokens)
	// Also cache short prompts for cost savings
	return len(content) > 100 && len(content) < 3000000
}

func (oc *OpenAICache) GetCacheKey(provider string, content string) string {
	return "openai:" + ComputeHash(content)
}

func (oc *OpenAICache) GetTTL(provider string) time.Duration {
	// OpenAI cache expires after 1 day of inactivity
	return 24 * time.Hour
}

func (oc *OpenAICache) GetMaxSize(provider string) int {
	// OpenAI allows large cache
	return 5000
}

func (oc *OpenAICache) UpdateOnAccess(entry *CacheEntry) error {
	entry.AccessCount++
	entry.LastAccessedAt = time.Now()
	// Extend expiration on access (OpenAI style)
	entry.ExpiresAt = time.Now().Add(oc.GetTTL("openai"))
	return nil
}

func (oc *OpenAICache) EvictionPolicy(entries []*CacheEntry) *CacheEntry {
	if len(entries) == 0 {
		return nil
	}

	// Cost-based: Evict lowest-cost entries (keep expensive ones)
	minCost := entries[0]
	for _, entry := range entries[1:] {
		if entry.Cost < minCost.Cost {
			minCost = entry
		}
	}
	return minCost
}

// CompressedCache is for compressed content (aggressive caching)
type CompressedCache struct{}

func NewCompressedCache() CacheStrategy {
	return &CompressedCache{}
}

func (cc *CompressedCache) Name() string {
	return "compressed"
}

func (cc *CompressedCache) CanCache(provider string, content string) bool {
	// Always cache compressed content
	return true
}

func (cc *CompressedCache) GetCacheKey(provider string, content string) string {
	return "compressed:" + ComputeHash(content)
}

func (cc *CompressedCache) GetTTL(provider string) time.Duration {
	// Compressed cache lasts longer (2 hours)
	return 2 * time.Hour
}

func (cc *CompressedCache) GetMaxSize(provider string) int {
	return 2000
}

func (cc *CompressedCache) UpdateOnAccess(entry *CacheEntry) error {
	entry.AccessCount++
	entry.LastAccessedAt = time.Now()
	return nil
}

func (cc *CompressedCache) EvictionPolicy(entries []*CacheEntry) *CacheEntry {
	if len(entries) == 0 {
		return nil
	}

	// Age-based: Evict oldest entries
	oldest := entries[0]
	for _, entry := range entries[1:] {
		if entry.CreatedAt.Before(oldest.CreatedAt) {
			oldest = entry
		}
	}
	return oldest
}
