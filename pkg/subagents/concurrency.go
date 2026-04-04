// Package subagents — concurrency.go
//
// ConcurrencyManager provides per-category semaphores that limit how many
// sub-agents of each category may run simultaneously. Ported from the
// build-your-own-openclaw Step 16 (concurrency-control) asyncio.Semaphore
// pattern, reimplemented using Go channels as semaphores.
//
// Each concurrency slot is keyed by the spawn category string
// ("reasoning", "speed", "coding", "general", or any custom value).
// When all slots for a category are occupied the call blocks until ctx is
// cancelled or a slot is released — whichever happens first.
//
// Default limits (overridable via SubagentConfig):
//
//	reasoning: 2   (heavy LLM calls; keep low)
//	speed:     4
//	coding:    3
//	general:   4
//	*:         2   (fallback for unknown categories)
//
// Usage:
//
//	cfg := subagents.DefaultSubagentConfig()
//	cfg.MaxConcurrentByCategory["coding"] = 6
//	cfg.MaxDepth = 3
//
//	cm := subagents.NewConcurrencyManager(cfg)
//
//	// In spawn logic:
//	if err := cm.Acquire(ctx, category); err != nil {
//	    return nil, err   // ctx cancelled — caller should abort
//	}
//	defer cm.Release(category)
//	// ... run sub-agent ...
package subagents

import (
	"context"
	"fmt"
	"sync"
)

// SubagentConfig is the user-facing configuration for sub-agent constraints.
// It can be loaded from GORKBOT.md YAML front-matter or set programmatically.
type SubagentConfig struct {
	// MaxDepth is the maximum recursive delegation depth.
	// Replaces the hard-coded MaxDepth constant in delegate.go.
	MaxDepth int `json:"max_depth" yaml:"max_depth"`

	// MaxConcurrentByCategory maps spawn category to max concurrent slots.
	// Missing categories fall back to DefaultMaxConcurrent.
	MaxConcurrentByCategory map[string]int `json:"max_concurrent" yaml:"max_concurrent"`

	// DefaultMaxConcurrent is the fallback for categories not in the map.
	DefaultMaxConcurrent int `json:"default_max_concurrent" yaml:"default_max_concurrent"`
}

// DefaultSubagentConfig returns sensible defaults.
func DefaultSubagentConfig() SubagentConfig {
	return SubagentConfig{
		MaxDepth: MaxDepth, // keeps backward compat with the const
		MaxConcurrentByCategory: map[string]int{
			"reasoning": 2,
			"speed":     4,
			"coding":    3,
			"general":   4,
		},
		DefaultMaxConcurrent: 2,
	}
}

// semaphore is a counting semaphore backed by a buffered channel.
type semaphore chan struct{}

func newSemaphore(n int) semaphore {
	if n <= 0 {
		n = 1
	}
	return make(semaphore, n)
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (s semaphore) Acquire(ctx context.Context) error {
	select {
	case s <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees one slot. Must be called exactly once after each successful Acquire.
func (s semaphore) Release() {
	select {
	case <-s:
	default:
		// Should never happen in correct usage; protect against double-release.
	}
}

// Len returns the number of currently occupied slots.
func (s semaphore) Len() int { return len(s) }

// ConcurrencyManager tracks per-category semaphores and the configured depth limit.
type ConcurrencyManager struct {
	cfg SubagentConfig

	mu   sync.Mutex
	sems map[string]semaphore
}

// NewConcurrencyManager creates a manager from the given config.
func NewConcurrencyManager(cfg SubagentConfig) *ConcurrencyManager {
	return &ConcurrencyManager{
		cfg:  cfg,
		sems: make(map[string]semaphore),
	}
}

// Acquire claims a concurrency slot for category, blocking until one is free
// or ctx is cancelled.
// Returns an error only when ctx is cancelled (caller should abort the task).
func (cm *ConcurrencyManager) Acquire(ctx context.Context, category string) error {
	sem := cm.semaphoreFor(category)
	return sem.Acquire(ctx)
}

// Release frees the slot previously acquired for category. Must be called
// exactly once after each successful Acquire (typically via defer).
func (cm *ConcurrencyManager) Release(category string) {
	cm.mu.Lock()
	sem, ok := cm.sems[category]
	cm.mu.Unlock()
	if ok {
		sem.Release()
	}
}

// MaxDepth returns the configured maximum delegation depth.
func (cm *ConcurrencyManager) MaxDepth() int {
	return cm.cfg.MaxDepth
}

// StatusReport returns a human-readable summary of current slot usage for the
// /concurrency debug command.
func (cm *ConcurrencyManager) StatusReport() string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(cm.sems) == 0 {
		return "No active concurrency semaphores.\n"
	}

	var sb fmt.Stringer
	_ = sb
	lines := fmt.Sprintf("Max depth: %d\n\nCategory      | Used | Limit\n", cm.cfg.MaxDepth)
	lines += "--------------+------+------\n"
	for cat, sem := range cm.sems {
		limit := cm.limitFor(cat)
		lines += fmt.Sprintf("%-14s| %4d | %5d\n", cat, sem.Len(), limit)
	}
	return lines
}

// semaphoreFor returns (creating if necessary) the semaphore for category.
func (cm *ConcurrencyManager) semaphoreFor(category string) semaphore {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if s, ok := cm.sems[category]; ok {
		return s
	}
	s := newSemaphore(cm.limitFor(category))
	cm.sems[category] = s
	return s
}

// limitFor returns the configured concurrency limit for category.
func (cm *ConcurrencyManager) limitFor(category string) int {
	if n, ok := cm.cfg.MaxConcurrentByCategory[category]; ok && n > 0 {
		return n
	}
	if cm.cfg.DefaultMaxConcurrent > 0 {
		return cm.cfg.DefaultMaxConcurrent
	}
	return 2
}
