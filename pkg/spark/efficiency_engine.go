package spark

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// ─── EfficiencyEngine (TII) ────────────────────────────────────────────────

// EfficiencyEngine implements the Tool Intelligence Index (TII).
// It maintains per-tool EWMA success rates and latencies.
type EfficiencyEngine struct {
	alpha   float64
	mu      sync.RWMutex
	entries map[string]*TIIEntry
	dataDir string
}

// NewEfficiencyEngine creates a TII engine and loads persisted data.
func NewEfficiencyEngine(alpha float64, dataDir string) *EfficiencyEngine {
	e := &EfficiencyEngine{
		alpha:   alpha,
		entries: make(map[string]*TIIEntry),
		dataDir: dataDir,
	}
	_ = e.load()
	return e
}

// RecordSuccess updates TII for a successful tool call.
func (e *EfficiencyEngine) RecordSuccess(toolName string, latencyMS int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := e.getOrCreate(toolName)
	entry.Invocations++
	entry.SuccessRate = e.alpha*1.0 + (1-e.alpha)*entry.SuccessRate
	entry.LatencyEWMA = e.alpha*float64(latencyMS) + (1-e.alpha)*entry.LatencyEWMA
	entry.LastUsed = time.Now()
}

// RecordFailure updates TII for a failed tool call.
func (e *EfficiencyEngine) RecordFailure(toolName string, latencyMS int64, errMsg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := e.getOrCreate(toolName)
	entry.Invocations++
	entry.SuccessRate = e.alpha*0.0 + (1-e.alpha)*entry.SuccessRate
	entry.LatencyEWMA = e.alpha*float64(latencyMS) + (1-e.alpha)*entry.LatencyEWMA
	if errMsg != "" {
		entry.LastError = errMsg
	}
	entry.LastUsed = time.Now()
}

// GetEntry returns a copy of the TII entry for a tool (nil if not seen yet).
func (e *EfficiencyEngine) GetEntry(toolName string) *TIIEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if raw, ok := e.entries[toolName]; ok {
		cp := *raw
		return &cp
	}
	return nil
}

// Snapshot returns a full copy of all TII entries.
func (e *EfficiencyEngine) Snapshot() []TIIEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]TIIEntry, 0, len(e.entries))
	for _, v := range e.entries {
		out = append(out, *v)
	}
	return out
}

// GetContextBlock formats the top-N tools by invocation count for prompt injection.
func (e *EfficiencyEngine) GetContextBlock(maxTools int) string {
	entries := e.Snapshot()
	sortTIIEntriesByUsage(entries)
	if len(entries) > maxTools {
		entries = entries[:maxTools]
	}
	if len(entries) == 0 {
		return ""
	}
	var sb []byte
	for _, en := range entries {
		line := fmt.Sprintf("  %-28s  calls=%-5d  sr=%.2f  lat=%.0fms\n",
			en.ToolName, en.Invocations, en.SuccessRate, en.LatencyEWMA)
		sb = append(sb, line...)
	}
	return string(sb)
}

// Persist saves the TII to disk.
func (e *EfficiencyEngine) Persist() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}
	b, err := json.Marshal(e.entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.dataDir, "tii.json"), b, 0644)
}

func (e *EfficiencyEngine) load() error {
	path := filepath.Join(e.dataDir, "tii.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &e.entries)
}

// getOrCreate returns the TII entry, creating with optimistic defaults if new.
// Caller must hold e.mu (write).
func (e *EfficiencyEngine) getOrCreate(toolName string) *TIIEntry {
	if entry, ok := e.entries[toolName]; ok {
		return entry
	}
	entry := &TIIEntry{
		ToolName:    toolName,
		SuccessRate: 1.0, // optimistic start
	}
	e.entries[toolName] = entry
	return entry
}

func sortTIIEntriesByUsage(entries []TIIEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Invocations > entries[j].Invocations
	})
}

// ─── ImprovementDebtLedger (IDL) ──────────────────────────────────────────

// ImprovementDebtLedger is a bounded min-heap (on Severity) priority queue of
// actionable improvement debt items.  When full, the lowest-severity item is
// evicted to preserve high-priority debt.
type ImprovementDebtLedger struct {
	mu      sync.Mutex
	items   idlHeap
	maxSize int
	dataDir string
	seen    map[string]int // ID → index in items (for dedup)
}

// NewImprovementDebtLedger creates an IDL and loads persisted state.
func NewImprovementDebtLedger(maxSize int, dataDir string) *ImprovementDebtLedger {
	l := &ImprovementDebtLedger{
		maxSize: maxSize,
		dataDir: dataDir,
		seen:    make(map[string]int),
	}
	heap.Init(&l.items)
	_ = l.load()
	return l
}

// Push adds a new IDL entry, deduplicating by ID.
func (l *ImprovementDebtLedger) Push(entry IDLEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if idx, exists := l.seen[entry.ID]; exists {
		// Duplicate: increment count + update timestamps.
		if idx < len(l.items) {
			l.items[idx].Occurrences++
			l.items[idx].LastSeen = time.Now()
			if entry.Severity > l.items[idx].Severity {
				l.items[idx].Severity = entry.Severity
			}
			heap.Fix(&l.items, idx)
		}
		return
	}

	// Evict lowest-severity item if at capacity.
	if len(l.items) >= l.maxSize {
		removed := heap.Pop(&l.items).(IDLEntry)
		delete(l.seen, removed.ID)
		// Rebuild seen map indices since heap may have rearranged.
		l.rebuildSeen()
	}

	if entry.FirstSeen.IsZero() {
		entry.FirstSeen = time.Now()
	}
	if entry.LastSeen.IsZero() {
		entry.LastSeen = time.Now()
	}
	if entry.Occurrences == 0 {
		entry.Occurrences = 1
	}
	heap.Push(&l.items, entry)
	l.rebuildSeen()
}

// Top returns a copy of the n highest-severity IDL entries.
func (l *ImprovementDebtLedger) Top(n int) []IDLEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]IDLEntry, len(l.items))
	copy(cp, l.items)
	sortIDLBySeverity(cp)
	if n > 0 && len(cp) > n {
		cp = cp[:n]
	}
	return cp
}

// Len returns the current number of IDL entries.
func (l *ImprovementDebtLedger) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.items)
}

// MaxSize returns the maximum IDL capacity.
func (l *ImprovementDebtLedger) MaxSize() int { return l.maxSize }

// Persist saves the IDL to disk.
func (l *ImprovementDebtLedger) Persist() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dataDir, 0755); err != nil {
		return err
	}
	b, err := json.Marshal([]IDLEntry(l.items))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.dataDir, "idl.json"), b, 0644)
}

func (l *ImprovementDebtLedger) load() error {
	path := filepath.Join(l.dataDir, "idl.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var items []IDLEntry
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	for _, item := range items {
		l.items = append(l.items, item)
	}
	heap.Init(&l.items)
	l.rebuildSeen()
	return nil
}

// rebuildSeen reconstructs the ID→index map after heap operations.
// Caller must hold l.mu.
func (l *ImprovementDebtLedger) rebuildSeen() {
	l.seen = make(map[string]int, len(l.items))
	for i, item := range l.items {
		l.seen[item.ID] = i
	}
}

func sortIDLBySeverity(items []IDLEntry) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Severity > items[j].Severity
	})
}

// ─── idlHeap (min-heap on Severity) ──────────────────────────────────────

type idlHeap []IDLEntry

func (h idlHeap) Len() int           { return len(h) }
func (h idlHeap) Less(i, j int) bool { return h[i].Severity < h[j].Severity } // min-heap
func (h idlHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *idlHeap) Push(x interface{}) {
	*h = append(*h, x.(IDLEntry))
}

func (h *idlHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Ensure sense import is used (FailureCategory is a string type from sense).
var _ sense.FailureCategory = sense.CatToolFailure
