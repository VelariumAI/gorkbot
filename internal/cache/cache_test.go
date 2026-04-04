package cache

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestCacheManager(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent := strings.Repeat("x", 600)
	entry := &CacheEntry{
		Content:      "test prompt",
		OutputTokens: 100,
		Cost:         0.001,
	}

	// Test Set
	err := cm.Set("claude", longContent, entry)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get - hit
	got, ok := cm.Get("claude", longContent)
	if !ok {
		t.Error("Get failed: expected hit")
	}

	if got == nil || got.Content != "test prompt" {
		t.Error("Get returned wrong entry")
	}

	// Test Get - miss
	_, ok = cm.Get("claude", "unknown content")
	if ok {
		t.Error("Get succeeded on miss: expected miss")
	}

	// Test stats
	stats := cm.GetStats()
	if stats["hits"].(int) != 1 {
		t.Errorf("Expected 1 hit, got %d", stats["hits"].(int))
	}
}

func TestCacheExpiration(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent := strings.Repeat("x", 150)
	entry := &CacheEntry{
		Content:      "test",
		OutputTokens: 50,
		Cost:         0.0005,
	}

	// Set with default strategy (1 hour TTL)
	cm.Set("default", longContent, entry)

	// Manually expire the entry
	if cached, ok := cm.entries["default:"+ComputeHash(longContent)]; ok {
		cached.ExpiresAt = time.Now().Add(-1 * time.Second)
	}

	// Get should miss on expired entry
	_, ok := cm.Get("default", longContent)
	if ok {
		t.Error("Get succeeded on expired entry: expected miss")
	}
}

func TestClaudeCacheStrategy(t *testing.T) {
	cc := NewClaudeCache()

	// Test CanCache
	if !cc.CanCache("claude", "a"+string(make([]byte, 500))) {
		t.Error("CanCache failed for valid content")
	}

	// Test TTL
	ttl := cc.GetTTL("claude")
	if ttl != 5*time.Minute {
		t.Errorf("Expected 5 minute TTL, got %v", ttl)
	}

	// Test max size
	if cc.GetMaxSize("claude") != 1000 {
		t.Error("Claude max size should be 1000")
	}
}

func TestOpenAICacheStrategy(t *testing.T) {
	oc := NewOpenAICache()

	// Test CanCache
	if !oc.CanCache("openai", "b"+string(make([]byte, 100))) {
		t.Error("CanCache failed for valid content")
	}

	// Test TTL
	ttl := oc.GetTTL("openai")
	if ttl != 24*time.Hour {
		t.Errorf("Expected 24 hour TTL, got %v", ttl)
	}

	// Test max size
	if oc.GetMaxSize("openai") != 5000 {
		t.Error("OpenAI max size should be 5000")
	}
}

func TestCacheInvalidation(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent1 := strings.Repeat("a", 150)
	longContent2 := strings.Repeat("b", 150)
	entry := &CacheEntry{
		Content: "test",
	}

	cm.Set("default", longContent1, entry)
	cm.Set("default", longContent2, entry)

	// Invalidate one
	cm.Invalidate("default", longContent1)

	if _, ok := cm.Get("default", longContent1); ok {
		t.Error("Invalidated entry should be missing")
	}

	if _, ok := cm.Get("default", longContent2); !ok {
		t.Error("Other entries should remain")
	}
}

func TestCachePrune(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent1 := strings.Repeat("a", 150)
	longContent2 := strings.Repeat("b", 150)
	entry := &CacheEntry{
		Content: "test",
	}

	cm.Set("default", longContent1, entry)
	cm.Set("default", longContent2, entry)

	// Manually expire one entry
	if cached, ok := cm.entries["default:"+ComputeHash(longContent1)]; ok {
		cached.ExpiresAt = time.Now().Add(-1 * time.Second)
	}

	pruned := cm.Prune()
	if pruned != 1 {
		t.Errorf("Expected 1 pruned entry, got %d", pruned)
	}

	if _, ok := cm.Get("default", longContent1); ok {
		t.Error("Pruned entry should be missing")
	}
}

func TestCacheClear(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent1 := strings.Repeat("a", 150)
	longContent2 := strings.Repeat("b", 150)
	entry := &CacheEntry{Content: "test"}
	cm.Set("default", longContent1, entry)
	cm.Set("default", longContent2, entry)

	cm.Clear()

	if len(cm.entries) != 0 {
		t.Error("Clear should remove all entries")
	}
}

func TestEvictionPolicies(t *testing.T) {
	// Test LRU eviction (DefaultCache)
	dc := NewDefaultCache()
	entries := []*CacheEntry{
		{
			Key:            "key1",
			LastAccessedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			Key:            "key2",
			LastAccessedAt: time.Now(),
		},
	}

	toEvict := dc.EvictionPolicy(entries)
	if toEvict.Key != "key1" {
		t.Error("LRU should evict least recently accessed")
	}

	// Test LFU eviction (ClaudeCache)
	cc := NewClaudeCache()
	entries = []*CacheEntry{
		{
			Key:         "key1",
			AccessCount: 1,
		},
		{
			Key:         "key2",
			AccessCount: 10,
		},
	}

	toEvict = cc.EvictionPolicy(entries)
	if toEvict.Key != "key1" {
		t.Error("LFU should evict least frequently used")
	}

	// Test cost-based eviction (OpenAICache)
	oc := NewOpenAICache()
	entries = []*CacheEntry{
		{
			Key:  "key1",
			Cost: 0.001,
		},
		{
			Key:  "key2",
			Cost: 0.1,
		},
	}

	toEvict = oc.EvictionPolicy(entries)
	if toEvict.Key != "key1" {
		t.Error("Cost-based should evict lowest-cost entries")
	}
}

func TestProviderStats(t *testing.T) {
	logger := slog.Default()
	cm := NewCacheManager(logger)

	longContent1 := strings.Repeat("a", 600)
	longContent2 := strings.Repeat("b", 600)
	entry := &CacheEntry{
		Content: "test",
		Cost:    0.001,
	}

	cm.Set("claude", longContent1, entry)
	cm.Set("claude", longContent2, entry)

	stats := cm.GetProviderStats("claude")

	if stats.Provider != "claude" {
		t.Error("Provider mismatch in stats")
	}

	if stats.Entries != 2 {
		t.Errorf("Expected 2 entries, got %d", stats.Entries)
	}
}

func TestComputeHash(t *testing.T) {
	hash1 := ComputeHash("test content")
	hash2 := ComputeHash("test content")
	hash3 := ComputeHash("different content")

	if hash1 != hash2 {
		t.Error("Same content should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("Different content should produce different hash")
	}

	if len(hash1) == 0 {
		t.Error("Hash should not be empty")
	}
}
