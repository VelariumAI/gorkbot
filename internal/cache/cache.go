package cache

import (
	"crypto/md5"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CacheEntry represents a cached prompt or response
type CacheEntry struct {
	Key            string
	Content        string
	Provider       string
	InputTokens    int
	OutputTokens   int
	CreatedAt      time.Time
	ExpiresAt      time.Time
	AccessCount    int
	LastAccessedAt time.Time
	Cost           float64
	Hash           string
	Metadata       map[string]interface{}
}

// CacheStrategy defines how to cache for a provider
type CacheStrategy interface {
	Name() string
	CanCache(provider string, content string) bool
	GetCacheKey(provider string, content string) string
	GetTTL(provider string) time.Duration
	GetMaxSize(provider string) int // max entries
	UpdateOnAccess(entry *CacheEntry) error
	EvictionPolicy(entries []*CacheEntry) *CacheEntry // which to evict
}

// CacheManager manages caching across providers
type CacheManager struct {
	entries    map[string]*CacheEntry
	strategies map[string]CacheStrategy
	logger     *slog.Logger
	mu         sync.RWMutex

	// Stats
	hits       int
	misses     int
	evictions  int
	totalSaved float64 // Total cost saved by cache hits
}

// NewCacheManager creates a new cache manager
func NewCacheManager(logger *slog.Logger) *CacheManager {
	if logger == nil {
		logger = slog.Default()
	}

	cm := &CacheManager{
		entries:    make(map[string]*CacheEntry),
		strategies: make(map[string]CacheStrategy),
		logger:     logger,
	}

	// Register default strategies
	cm.registerStrategies()

	return cm
}

// registerStrategies registers caching strategies for providers
func (cm *CacheManager) registerStrategies() {
	cm.strategies["claude"] = NewClaudeCache()
	cm.strategies["openai"] = NewOpenAICache()
	cm.strategies["default"] = NewDefaultCache()

	cm.logger.Debug("registered cache strategies", slog.Int("count", len(cm.strategies)))
}

// Get retrieves a cached entry if available and not expired
func (cm *CacheManager) Get(provider string, content string) (*CacheEntry, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	strategy := cm.getStrategy(provider)
	cacheKey := strategy.GetCacheKey(provider, content)

	entry, ok := cm.entries[cacheKey]
	if !ok {
		cm.misses++
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		delete(cm.entries, cacheKey)
		cm.misses++
		return nil, false
	}

	// Entry is valid
	cm.hits++
	if err := strategy.UpdateOnAccess(entry); err != nil {
		cm.logger.Warn("cache UpdateOnAccess failed", slog.String("key", cacheKey), slog.String("provider", provider), slog.String("error", err.Error()))
	}

	return entry, true
}

// Set caches an entry
func (cm *CacheManager) Set(provider string, content string, entry *CacheEntry) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	strategy := cm.getStrategy(provider)

	// Check if caching is allowed
	if !strategy.CanCache(provider, content) {
		return nil
	}

	// Generate cache key
	cacheKey := strategy.GetCacheKey(provider, content)
	entry.Key = cacheKey
	entry.Provider = provider
	entry.CreatedAt = time.Now()
	entry.ExpiresAt = time.Now().Add(strategy.GetTTL(provider))

	// Check size limits
	maxSize := strategy.GetMaxSize(provider)
	if len(cm.entries) >= maxSize {
		// Evict least valuable entry
		if toEvict := strategy.EvictionPolicy(cm.entriesList()); toEvict != nil {
			delete(cm.entries, toEvict.Key)
			cm.evictions++
		}
	}

	// Cache the entry
	copied := *entry
	cm.entries[cacheKey] = &copied

	cm.logger.Debug("cached entry",
		slog.String("provider", provider),
		slog.String("key", cacheKey),
		slog.Int("output_tokens", entry.OutputTokens),
		slog.Float64("cost", entry.Cost),
	)

	return nil
}

// Invalidate removes an entry from cache
func (cm *CacheManager) Invalidate(provider string, content string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	strategy := cm.getStrategy(provider)
	cacheKey := strategy.GetCacheKey(provider, content)

	if _, ok := cm.entries[cacheKey]; ok {
		delete(cm.entries, cacheKey)
		cm.logger.Debug("invalidated cache entry", slog.String("key", cacheKey))
	}
}

// InvalidateByPattern removes all entries matching a pattern
func (cm *CacheManager) InvalidateByPattern(pattern string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	count := 0
	for key := range cm.entries {
		if matchPattern(key, pattern) {
			delete(cm.entries, key)
			count++
		}
	}

	cm.logger.Debug("invalidated cache entries by pattern",
		slog.String("pattern", pattern),
		slog.Int("count", count),
	)

	return count
}

// Clear removes all entries
func (cm *CacheManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.entries = make(map[string]*CacheEntry)
	cm.logger.Debug("cleared all cache entries")
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	totalRequests := cm.hits + cm.misses
	var hitRate float64
	if totalRequests > 0 {
		hitRate = float64(cm.hits) / float64(totalRequests) * 100
	}

	return map[string]interface{}{
		"hits":        cm.hits,
		"misses":      cm.misses,
		"hit_rate":    hitRate,
		"evictions":   cm.evictions,
		"total_saved": cm.totalSaved,
		"entries":     len(cm.entries),
	}
}

// Prune removes expired entries
func (cm *CacheManager) Prune() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	count := 0
	now := time.Now()
	for key, entry := range cm.entries {
		if now.After(entry.ExpiresAt) {
			delete(cm.entries, key)
			count++
		}
	}

	if count > 0 {
		cm.logger.Debug("pruned expired cache entries", slog.Int("count", count))
	}

	return count
}

// entriesList returns list of cache entries
func (cm *CacheManager) entriesList() []*CacheEntry {
	entries := make([]*CacheEntry, 0, len(cm.entries))
	for _, entry := range cm.entries {
		entries = append(entries, entry)
	}
	return entries
}

// getStrategy returns strategy for provider or default
func (cm *CacheManager) getStrategy(provider string) CacheStrategy {
	if strategy, ok := cm.strategies[provider]; ok {
		return strategy
	}
	return cm.strategies["default"]
}

// matchPattern matches simple glob patterns
func matchPattern(key string, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) == 0 {
		return false
	}

	// Simple substring match for now
	return len(key) >= len(pattern) && key[:len(pattern)] == pattern
}

// ComputeHash computes MD5 hash of content
func ComputeHash(content string) string {
	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// CacheStats tracks detailed cache statistics
type CacheStats struct {
	Provider      string
	Hits          int
	Misses        int
	HitRate       float64
	AvgCostSaved  float64
	AvgAccessTime time.Duration
	Entries       int
	SizeBytes     int
}

// GetProviderStats returns statistics for a specific provider
func (cm *CacheManager) GetProviderStats(provider string) *CacheStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := &CacheStats{
		Provider: provider,
	}

	totalCost := 0.0
	entryCount := 0

	for _, entry := range cm.entries {
		if entry.Provider == provider {
			stats.Hits += entry.AccessCount
			stats.SizeBytes += len(entry.Content)
			totalCost += entry.Cost
			entryCount++
		}
	}

	if entryCount > 0 {
		stats.AvgCostSaved = totalCost / float64(entryCount)
		stats.Entries = entryCount
	}

	totalRequests := stats.Hits + stats.Misses
	if totalRequests > 0 {
		stats.HitRate = float64(stats.Hits) / float64(totalRequests) * 100
	}

	return stats
}
