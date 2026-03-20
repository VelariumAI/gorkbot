package cache

import (
	"sync"
	"time"
)

const (
	appCacheDefaultTTL     = 5 * time.Minute
	appCacheMaxEntries     = 256
	appCacheCleanupEvery   = 100 // evict expired entries after this many Sets
)

// appCacheEntry is a single cached response with an expiry time.
type appCacheEntry struct {
	value     string
	expiresAt time.Time
}

// AppCache is a bounded, TTL-aware in-memory response cache used as the
// universal Tier-3 fallback for providers without native prompt-caching APIs.
//
// Keys are typically: "<providerID>:<model>:<system_hash>".
// Eviction strategy: LRU on overflow + time-based expiry on access.
type AppCache struct {
	mu        sync.RWMutex
	entries   map[string]appCacheEntry
	order     []string // insertion-order for LRU eviction (oldest first)
	ttl       time.Duration
	setCount  int
}

// NewAppCache creates an AppCache. configDir is accepted for future on-disk
// persistence but is currently unused (in-memory only for low RAM targets).
func NewAppCache(_ string) *AppCache {
	return &AppCache{
		entries: make(map[string]appCacheEntry, appCacheMaxEntries),
		order:   make([]string, 0, appCacheMaxEntries),
		ttl:     appCacheDefaultTTL,
	}
}

// Get returns the cached response for key, and true when the entry is present
// and has not expired. Returns ("", false) on miss or expiry.
func (c *AppCache) Get(key string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		// Lazy delete — expired entry is logically absent.
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return "", false
	}
	return entry.value, true
}

// Set stores a response under key with the default TTL.
// When the cache is at capacity, the oldest entry is evicted.
func (c *AppCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict LRU if at capacity.
	if _, exists := c.entries[key]; !exists {
		for len(c.entries) >= appCacheMaxEntries && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}

	c.entries[key] = appCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}

	// Periodic expired-entry sweep to avoid memory creep.
	c.setCount++
	if c.setCount%appCacheCleanupEvery == 0 {
		c.sweepLocked()
	}
}

// sweepLocked removes all expired entries. Must be called with c.mu held.
func (c *AppCache) sweepLocked() {
	now := time.Now()
	alive := c.order[:0]
	for _, k := range c.order {
		if e, ok := c.entries[k]; ok && now.Before(e.expiresAt) {
			alive = append(alive, k)
		} else {
			delete(c.entries, k)
		}
	}
	c.order = alive
}

// Len returns the number of live (non-expired) entries.
func (c *AppCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := 0
	now := time.Now()
	for _, e := range c.entries {
		if now.Before(e.expiresAt) {
			count++
		}
	}
	return count
}
