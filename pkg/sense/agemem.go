// Package sense implements the SENSE (Self-Evolving Neural Stabilization Engine)
// framework components adapted for Gorkbot's API-based dual-tier AI architecture.
// This is a faithful Go port of the SENSE v4.1 memory, reward, and stabilization
// concepts; the evolutionary GRPO trainer is intentionally omitted because Gorkbot
// uses cloud-hosted models (Grok / Gemini) whose weights cannot be fine-tuned.
package sense

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Memory tier constants ────────────────────────────────────────────────────

// MemoryTier indicates how a memory entry is retained.
type MemoryTier string

const (
	TierHot  MemoryTier = "hot"  // In active working window (STM)
	TierWarm MemoryTier = "warm" // Recently accessed, still in STM
	TierCold MemoryTier = "cold" // Persisted in LTM only
)

// ─── Memory entry ─────────────────────────────────────────────────────────────

// MemoryEntry is a single piece of stored knowledge.
type MemoryEntry struct {
	Key         string                 `json:"key"`
	Content     string                 `json:"content"`
	Priority    float64                `json:"priority"`
	Tier        MemoryTier             `json:"tier"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	AccessedAt  time.Time              `json:"accessed_at"`
	AccessCount int                    `json:"access_count"`
}

func (e *MemoryEntry) score() float64 {
	// Combine priority with recency (linear decay over 24 h)
	age := time.Since(e.AccessedAt).Hours()
	decay := 1.0 - (age/24.0)*0.5
	if decay < 0.1 {
		decay = 0.1
	}
	return e.Priority * decay
}

// ─── Short-Term Memory ────────────────────────────────────────────────────────

// STM is a token-bounded ring-buffer of recent entries.
// When it exceeds 80 % of capacity it calls a prune callback so high-priority
// items can be consolidated to LTM before they are evicted.
type STM struct {
	mu            sync.Mutex
	entries       []*MemoryEntry
	maxTokens     int
	currentTokens int
	pruneCallback func(freed, remaining int)
}

func newSTM(maxTokens int) *STM {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	return &STM{maxTokens: maxTokens}
}

func (s *STM) estimateTokens(content string) int {
	// Rough 4 chars ≈ 1 token heuristic.
	return (len(content) + 3) / 4
}

// Store adds an entry to STM, triggering pruning when capacity is near.
func (s *STM) Store(key, content string, priority float64, metadata map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokens := s.estimateTokens(content)

	// Update existing if key already present.
	for _, e := range s.entries {
		if e.Key == key {
			s.currentTokens -= s.estimateTokens(e.Content)
			e.Content = content
			e.Priority = priority
			e.AccessedAt = time.Now()
			e.AccessCount++
			if metadata != nil {
				e.Metadata = metadata
			}
			s.currentTokens += tokens
			return
		}
	}

	entry := &MemoryEntry{
		Key:        key,
		Content:    content,
		Priority:   priority,
		Tier:       TierHot,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}
	s.entries = append(s.entries, entry)
	s.currentTokens += tokens

	// Prune if we're over 80 % capacity.
	if s.currentTokens > int(float64(s.maxTokens)*0.8) {
		s.prune()
	}
}

// Retrieve looks up an entry by key.
func (s *STM) Retrieve(key string) (*MemoryEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.Key == key {
			e.AccessedAt = time.Now()
			e.AccessCount++
			return e, true
		}
	}
	return nil, false
}

// Search returns the top-k entries that contain any of the query words.
func (s *STM) Search(query string, k int) []*MemoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		entry *MemoryEntry
		score float64
	}
	var results []scored

	for _, e := range s.entries {
		lower := strings.ToLower(e.Content + " " + e.Key)
		overlap := 0
		for _, w := range words {
			if strings.Contains(lower, w) {
				overlap++
			}
		}
		if overlap > 0 {
			results = append(results, scored{e, float64(overlap) * e.score()})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	out := make([]*MemoryEntry, 0, k)
	for i, r := range results {
		if i >= k {
			break
		}
		out = append(out, r.entry)
	}
	return out
}

// GetContext returns a formatted string of the top entries (by score) capped at maxTokens.
func (s *STM) GetContext(maxTokens int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	sorted := make([]*MemoryEntry, len(s.entries))
	copy(sorted, s.entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score() > sorted[j].score() })

	var sb strings.Builder
	used := 0
	for _, e := range sorted {
		t := s.estimateTokens(e.Content)
		if used+t > maxTokens {
			break
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", e.Key, e.Content))
		used += t
	}
	return sb.String()
}

// prune evicts the lowest-scored entries down to 60 % of capacity (caller holds lock).
func (s *STM) prune() {
	target := int(float64(s.maxTokens) * 0.6)
	sort.Slice(s.entries, func(i, j int) bool {
		return s.entries[i].score() > s.entries[j].score()
	})
	freed := 0
	for s.currentTokens > target && len(s.entries) > 0 {
		last := s.entries[len(s.entries)-1]
		freed += s.estimateTokens(last.Content)
		s.currentTokens -= s.estimateTokens(last.Content)
		s.entries = s.entries[:len(s.entries)-1]
	}
	if s.pruneCallback != nil {
		s.pruneCallback(freed, s.currentTokens)
	}
}

// Usage returns (currentTokens, maxTokens).
func (s *STM) Usage() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTokens, s.maxTokens
}

// ShouldPrune returns true when STM is over 80 % full.
func (s *STM) ShouldPrune() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTokens > int(float64(s.maxTokens)*0.8)
}

// All returns a snapshot of all STM entries (for consolidation).
func (s *STM) All() []*MemoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*MemoryEntry, len(s.entries))
	copy(cp, s.entries)
	return cp
}

// ─── Long-Term Memory ─────────────────────────────────────────────────────────

// LTM persists entries as a JSON file on disk. Semantic search uses keyword
// matching (exact vector embeddings are not available without a local model).
type LTM struct {
	mu      sync.RWMutex
	path    string
	entries map[string]*MemoryEntry
}

func newLTM(dir string) (*LTM, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("ltm: mkdir: %w", err)
	}
	l := &LTM{
		path:    filepath.Join(dir, "ltm.json"),
		entries: make(map[string]*MemoryEntry),
	}
	_ = l.load()
	return l, nil
}

// Store upserts an entry in LTM.
func (l *LTM) Store(key, content string, priority float64, metadata map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := &MemoryEntry{
		Key:        key,
		Content:    content,
		Priority:   priority,
		Tier:       TierCold,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}
	if existing, ok := l.entries[key]; ok {
		e.CreatedAt = existing.CreatedAt
		e.AccessCount = existing.AccessCount
	}
	l.entries[key] = e
	if err := l.save(); err != nil {
		slog.Warn("AgeMem LTM: failed to persist store", "key", key, "error", err)
	}
}

// Retrieve fetches a single entry.
func (l *LTM) Retrieve(key string) (*MemoryEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if ok {
		e.AccessedAt = time.Now()
		e.AccessCount++
		if err := l.save(); err != nil {
			slog.Warn("AgeMem LTM: failed to persist access stats", "key", key, "error", err)
		}
	}
	return e, ok
}

// Search returns top-k entries by keyword overlap + priority.
func (l *LTM) Search(query string, k int) []*MemoryEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		e     *MemoryEntry
		score float64
	}
	var results []scored
	for _, e := range l.entries {
		lower := strings.ToLower(e.Content + " " + e.Key)
		overlap := 0
		for _, w := range words {
			if strings.Contains(lower, w) {
				overlap++
			}
		}
		if overlap > 0 {
			results = append(results, scored{e, float64(overlap) * e.Priority})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	out := make([]*MemoryEntry, 0, k)
	for i, r := range results {
		if i >= k {
			break
		}
		out = append(out, r.e)
	}
	return out
}

// touchBatch updates AccessedAt and AccessCount for any of the given entries
// that live in LTM, then flushes once. Called after a Search so that
// frequently-retrieved cold memories accumulate access weight.
func (l *LTM) touchBatch(entries []*MemoryEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for _, e := range entries {
		if stored, ok := l.entries[e.Key]; ok {
			stored.AccessedAt = time.Now()
			stored.AccessCount++
			changed = true
		}
	}
	if changed {
		_ = l.save()
	}
}

func (l *LTM) save() error {
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.path, data, 0600)
}

// maxLTMFileBytes caps the LTM file size to protect against OOM from a
// corrupted or adversarially large file (10 MB practical ceiling).
const maxLTMFileBytes = 10 * 1024 * 1024

func (l *LTM) load() error {
	info, err := os.Stat(l.path)
	if err != nil {
		return err
	}
	if info.Size() > maxLTMFileBytes {
		return fmt.Errorf("ltm file too large (%d bytes, max %d): refusing to load", info.Size(), maxLTMFileBytes)
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &l.entries)
}

// ─── AgeMem: unified two-tier memory ─────────────────────────────────────────

// AgeMem is the SENSE "Filing Cabinet" — a unified API over STM and LTM.
// Per SENSE architecture:
//   - Short-Term Memory: bounded token window, keyword search, auto-prune
//   - Long-Term Memory: persistent JSON store, promoted from STM on prune
//   - Consolidation: high-priority STM entries are written to LTM when pruning
type AgeMem struct {
	stm *STM
	ltm *LTM
}

// NewAgeMem constructs an AgeMem backed by files in dataDir.
// stmMaxTokens controls the STM capacity; 0 uses a sensible default (8000).
func NewAgeMem(dataDir string, stmMaxTokens int) (*AgeMem, error) {
	ltmDir := filepath.Join(dataDir, "agemem")
	ltm, err := newLTM(ltmDir)
	if err != nil {
		return nil, fmt.Errorf("agemem: ltm init: %w", err)
	}
	stm := newSTM(stmMaxTokens)
	am := &AgeMem{stm: stm, ltm: ltm}

	// On STM prune, consolidate high-priority entries into LTM.
	stm.pruneCallback = func(freed, _ int) {
		am.consolidate()
	}
	return am, nil
}

// Store saves content with the given priority (0–1, higher = more important).
// If persist is true or priority ≥ 0.9, the entry is also written to LTM.
func (am *AgeMem) Store(key, content string, priority float64, metadata map[string]interface{}, persist bool) {
	am.stm.Store(key, content, priority, metadata)
	if persist || priority >= 0.9 {
		am.ltm.Store(key, content, priority, metadata)
	}
}

// Retrieve checks STM first, then LTM.
func (am *AgeMem) Retrieve(key string) (*MemoryEntry, bool) {
	if e, ok := am.stm.Retrieve(key); ok {
		return e, true
	}
	return am.ltm.Retrieve(key)
}

// Search queries both tiers and returns deduplicated results (STM preferred).
func (am *AgeMem) Search(query string, k int) []*MemoryEntry {
	stmResults := am.stm.Search(query, k)
	ltmResults := am.ltm.Search(query, k)

	seen := make(map[string]bool)
	var merged []*MemoryEntry
	for _, e := range stmResults {
		seen[e.Key] = true
		merged = append(merged, e)
	}
	for _, e := range ltmResults {
		if !seen[e.Key] {
			merged = append(merged, e)
		}
	}
	if len(merged) > k {
		merged = merged[:k]
	}
	return merged
}

// GetContextForPrompt returns formatted STM context capped at maxTokens.
// Deprecated: prefer FormatRelevant which queries both tiers with relevance ranking.
func (am *AgeMem) GetContextForPrompt(maxTokens int) string {
	return am.stm.GetContext(maxTokens)
}

// FormatRelevant returns a token-capped, formatted block of the most relevant
// memories for query, searching BOTH STM (hot/warm) and LTM (cold/persistent).
// Results are ranked by keyword overlap × priority × recency.
// Tier labels are included so the AI knows how fresh each memory is.
// LTM access stats are updated (touchBatch) so frequently-surfaced cold
// memories accumulate weight and survive future eviction cycles.
// Returns "" when nothing relevant is found.
func (am *AgeMem) FormatRelevant(query string, maxTokens int) string {
	if query == "" {
		return am.stm.GetContext(maxTokens)
	}
	results := am.Search(query, 15) // Search already deduplicates STM+LTM
	if len(results) == 0 {
		return ""
	}

	// Touch LTM entries to update access stats.
	am.ltm.touchBatch(results)

	// Format, capping at maxTokens (rough 4 chars/token estimate).
	var sb strings.Builder
	used := 0
	for _, e := range results {
		line := fmt.Sprintf("[%s|%s]: %s\n", e.Tier, e.Key, e.Content)
		t := (len(line) + 3) / 4
		if used+t > maxTokens {
			break
		}
		sb.WriteString(line)
		used += t
	}
	return sb.String()
}

// ShouldPrune returns true when STM exceeds 80 % of its token budget.
func (am *AgeMem) ShouldPrune() bool {
	return am.stm.ShouldPrune()
}

// consolidate promotes high-priority STM entries (priority ≥ 0.8) to LTM.
func (am *AgeMem) consolidate() {
	for _, e := range am.stm.All() {
		if e.Priority >= 0.8 {
			am.ltm.Store(e.Key, e.Content, e.Priority, e.Metadata)
		}
	}
}

// UsageStats returns token usage info for monitoring.
func (am *AgeMem) UsageStats() map[string]interface{} {
	cur, max := am.stm.Usage()
	return map[string]interface{}{
		"stm_tokens": cur,
		"stm_max":    max,
		"stm_pct":    float64(cur) / float64(max) * 100,
	}
}
