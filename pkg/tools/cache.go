package tools

import (
	"container/list"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// cachePruneInterval controls how often expired SQLite rows are deleted.
const cachePruneInterval = 1 * time.Hour

// cacheEntry stores an in-memory cached tool result.
type cacheEntry struct {
	key       string
	result    *ToolResult
	expiresAt time.Time
}

// ToolCache provides a 3-tier bounded caching layer for tool results:
// Tier 1: In-process bounded LRU
// Tier 2: SQLite backing store (if configured)
// Tier 3: Execution
type ToolCache struct {
	mu       sync.RWMutex
	maxItems int
	ll       *list.List
	entries  map[string]*list.Element
	ttls     map[string]time.Duration
	db       *sql.DB
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

// NewToolCache creates a bounded ToolCache (LRU) with an optional SQLite backing store.
func NewToolCache(maxItems int, db *sql.DB) *ToolCache {
	if maxItems <= 0 {
		maxItems = 1000 // Default LRU size
	}
	return &ToolCache{
		maxItems: maxItems,
		ll:       list.New(),
		entries:  make(map[string]*list.Element),
		ttls:     DefaultCacheTTLs,
		db:       db,
	}
}

// SetDB sets the SQLite backing store after creation.
func (c *ToolCache) SetDB(db *sql.DB) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = db
}

// cacheKey builds a deterministic cache key from tool name + parameters.
func cacheKey(toolName string, params map[string]interface{}) string {
	pathSuffix := ""
	if params != nil {
		if path, ok := params["file_path"].(string); ok {
			pathSuffix = "|path:" + path
		} else if path, ok := params["path"].(string); ok {
			pathSuffix = "|path:" + path
		} else if path, ok := params["dir_path"].(string); ok {
			pathSuffix = "|path:" + path
		}
	}

	h := sha256.New()
	h.Write([]byte(toolName))
	if params != nil {
		b, _ := json.Marshal(params)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)) + pathSuffix
}

// Get retrieves a cached result. First checks Memory LRU, then SQLite cache.
func (c *ToolCache) Get(toolName string, params map[string]interface{}) (*ToolResult, bool) {
	if _, ok := c.ttls[toolName]; !ok {
		return nil, false
	}
	key := cacheKey(toolName, params)

	c.mu.Lock()
	if ele, ok := c.entries[key]; ok {
		entry := ele.Value.(*cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			c.ll.MoveToFront(ele)
			c.mu.Unlock()
			return entry.result, true
		}
		// Expired memory entry
		c.ll.Remove(ele)
		delete(c.entries, key)
	}
	c.mu.Unlock()

	// Check SQLite Cache if missed in memory
	if c.db != nil {
		var resultData string
		var createdAt time.Time
		var ttl int
		err := c.db.QueryRow(`
			SELECT result_data, created_at, ttl_seconds 
			FROM cache_tool_results 
			WHERE request_hash = ?
		`, key).Scan(&resultData, &createdAt, &ttl)

		if err == nil {
			if time.Since(createdAt).Seconds() <= float64(ttl) {
				// Cache hit from SQLite! Restore to memory
				var res ToolResult
				if json.Unmarshal([]byte(resultData), &res) == nil {
					c.SetMemoryOnly(key, &res, time.Duration(ttl)*time.Second)
					return &res, true
				}
			}
		}
	}

	return nil, false
}

// SetMemoryOnly adds an item to the in-memory LRU without persisting to SQLite.
func (c *ToolCache) SetMemoryOnly(key string, result *ToolResult, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.entries[key]; ok {
		c.ll.MoveToFront(ele)
		ele.Value.(*cacheEntry).result = result
		ele.Value.(*cacheEntry).expiresAt = time.Now().Add(ttl)
		return
	}

	ent := &cacheEntry{key: key, result: result, expiresAt: time.Now().Add(ttl)}
	ele := c.ll.PushFront(ent)
	c.entries[key] = ele

	if c.ll.Len() > c.maxItems {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.entries, oldest.Value.(*cacheEntry).key)
		}
	}
}

// Set stores a result in the memory cache and SQLite cache for the tool's configured TTL.
func (c *ToolCache) Set(toolName string, params map[string]interface{}, result *ToolResult) {
	ttl, ok := c.ttls[toolName]
	if !ok {
		return
	}
	key := cacheKey(toolName, params)
	c.SetMemoryOnly(key, result, ttl)

	if c.db != nil {
		if b, err := json.Marshal(result); err == nil {
			resultData := string(b)
			ttlSeconds := int(ttl.Seconds())
			// Ignore errors in background cache writing, just fire and forget
			go func() {
				_, _ = c.db.Exec(`
					INSERT INTO cache_tool_results (request_hash, result_data, created_at, ttl_seconds)
					VALUES (?, ?, CURRENT_TIMESTAMP, ?)
					ON CONFLICT(request_hash) DO UPDATE SET
						result_data = excluded.result_data,
						created_at = CURRENT_TIMESTAMP,
						ttl_seconds = excluded.ttl_seconds;
				`, key, resultData, ttlSeconds)
			}()
		}
	}
}

// InvalidateOnMutation flushes the cache or path-scoped entries.
func (c *ToolCache) InvalidateOnMutation(toolName string, params map[string]interface{}) {
	if MutationInvalidates[toolName] {
		pathFound := false
		if params != nil {
			if path, ok := params["file_path"].(string); ok && path != "" {
				c.invalidatePath(path)
				pathFound = true
			} else if path, ok := params["path"].(string); ok && path != "" {
				c.invalidatePath(path)
				pathFound = true
			}
		}
		if !pathFound || toolName == "bash" || toolName == "git_commit" || toolName == "git_pull" {
			// Commands like bash or git could alter anything, flush everything.
			c.Flush()
		}
	}
}

// invalidatePath safely flushes memory and SQLite for a specific file path.
func (c *ToolCache) invalidatePath(path string) {
	suffix := "|path:" + path
	c.mu.Lock()
	for key, ele := range c.entries {
		if strings.HasSuffix(key, suffix) {
			c.ll.Remove(ele)
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()

	if c.db != nil {
		go func() {
			_, _ = c.db.Exec(`DELETE FROM cache_tool_results WHERE request_hash LIKE ?`, "%"+suffix)
		}()
	}
}

// Flush removes all entries from the cache.
func (c *ToolCache) Flush() {
	c.mu.Lock()
	c.ll = list.New()
	c.entries = make(map[string]*list.Element)
	c.mu.Unlock()

	if c.db != nil {
		go func() {
			_, _ = c.db.Exec(`DELETE FROM cache_tool_results`)
		}()
	}
}

// StartCachePruner starts a background goroutine that periodically deletes
// expired rows from the cache_tool_results SQLite table.  It exits when ctx
// is cancelled.  Safe to call with a nil db — becomes a no-op.
func (c *ToolCache) StartCachePruner(ctx context.Context) {
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()
	if db == nil {
		return
	}
	go func() {
		pruneExpired := func() {
			_, err := db.ExecContext(ctx,
				`DELETE FROM cache_tool_results
				 WHERE datetime(created_at, '+' || ttl_seconds || ' seconds') < datetime('now')`)
			if err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "cache_pruner: %v\n", err)
			}
		}
		pruneExpired() // run immediately on startup
		ticker := time.NewTicker(cachePruneInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pruneExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Size returns the number of live (non-expired) cache entries in memory.
func (c *ToolCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	count := 0
	for e := c.ll.Front(); e != nil; e = e.Next() {
		if now.Before(e.Value.(*cacheEntry).expiresAt) {
			count++
		}
	}
	return count
}
