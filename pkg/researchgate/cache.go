package researchgate

import (
	"sync"
	"time"
)

type Cache interface {
	Get(key string) (ResearchResult, bool)
	Put(key string, value ResearchResult, ttl time.Duration)
}

type memoryCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	value     ResearchResult
	expiresAt time.Time
}

func NewMemoryCache() Cache {
	return &memoryCache{items: make(map[string]cacheEntry)}
}

func (c *memoryCache) Get(key string) (ResearchResult, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return ResearchResult{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return ResearchResult{}, false
	}
	return entry.value, true
}

func (c *memoryCache) Put(key string, value ResearchResult, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.items[key] = cacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}
