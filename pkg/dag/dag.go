// Package dag implements a Directed Acyclic Graph (DAG) task execution engine
// for the Gorkbot Orchestrator. It provides parallel scheduling, dependency
// resolution, self-healing via AI Root Cause Analysis, atomic rollback, binary
// state persistence, and real-time Bubble Tea TUI integration.
//
// Design goals:
//   - Outperform Python agentic frameworks (LangChain / Deep Agents) by using
//     native Go goroutines for true parallelism and gob for zero-copy serialization.
//   - Platform-agnostic: runtime detection switches between Termux, Linux, macOS
//     and Windows execution backends without user configuration.
//   - Self-healing: every tool failure triggers an AI Root Cause Analysis (RCA)
//     step before a retry, so the agent understands *why* something broke.
package dag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"log/slog"
)

// ─── Task lifecycle states ────────────────────────────────────────────────────

// TaskStatus is the lifecycle state of a Task within the graph.
type TaskStatus int8

const (
	StatusPending    TaskStatus = iota // Blocked waiting for dependencies
	StatusQueued                       // Dependencies met; waiting for executor slot
	StatusRunning                      // Actively executing in a goroutine
	StatusCompleted                    // Finished successfully
	StatusFailed                       // Finished with error (RCA + retry may follow)
	StatusSkipped                      // A dependency failed; this task was cancelled
	StatusRolledBack                   // Completed then undone via Rollback()
)

// String implements fmt.Stringer.
func (s TaskStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusQueued:
		return "queued"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	case StatusRolledBack:
		return "rolled_back"
	default:
		return "unknown"
	}
}

// Icon returns the single-character TUI glyph for this status.
func (s TaskStatus) Icon() string {
	switch s {
	case StatusPending:
		return "○"
	case StatusQueued:
		return "◎"
	case StatusRunning:
		return "●"
	case StatusCompleted:
		return "✓"
	case StatusFailed:
		return "✗"
	case StatusSkipped:
		return "⊘"
	case StatusRolledBack:
		return "↩"
	default:
		return "?"
	}
}

// ─── Core types ───────────────────────────────────────────────────────────────

// ActionFunc is the work performed by a Task. It receives the running context
// and the shared Environment for cross-task data access and I/O.
// The returned interface{} is stored in Task.Result on success.
type ActionFunc func(ctx context.Context, env *Environment) (interface{}, error)

// RollbackFunc is the optional undo operation for a Task.
// Called by Graph.RollbackTask or automatically on full-graph rollback.
type RollbackFunc func(ctx context.Context, env *Environment) error

// Task is a discrete unit of work within the DAG.
// Fields that begin with a capital letter are serialised by state.go (gob).
type Task struct {
	ID           string        // Unique identifier (must be stable across restarts)
	Description  string        // Human-readable label shown in the TUI
	Dependencies []string      // IDs of tasks that must complete before this one
	Tags         []string      // Arbitrary labels for grouping / filtering
	MaxRetries   int           // 0 = no retries; -1 = infinite
	RetryDelay   time.Duration // Base delay; actual delay = RetryDelay * 2^RetryCount

	// Callbacks — not serialised (re-registered after deserialization).
	Action   ActionFunc   `gob:"-"`
	Rollback RollbackFunc `gob:"-"`

	// Runtime state (serialised).
	Status      TaskStatus
	Result      interface{} // Successful return value from Action
	ErrMsg      string      // Last error string (stored instead of error for gob)
	StartedAt   time.Time
	CompletedAt time.Time
	RetryCount  int
	RCAReport   string  // AI-generated root cause analysis after failure
	Progress    float64 // 0.0–1.0; updated via Environment.SetProgress
}

// Clone returns a shallow copy safe for snapshotting.
func (t *Task) Clone() *Task {
	c := *t
	c.Dependencies = append([]string(nil), t.Dependencies...)
	c.Tags = append([]string(nil), t.Tags...)
	return &c
}

// elapsed returns how long the task has been running (or ran).
func (t *Task) elapsed() time.Duration {
	if t.StartedAt.IsZero() {
		return 0
	}
	if !t.CompletedAt.IsZero() {
		return t.CompletedAt.Sub(t.StartedAt)
	}
	return time.Since(t.StartedAt)
}

// ─── Events ───────────────────────────────────────────────────────────────────

// Event is emitted on every Task state change and consumed by the TUI and
// any external observers attached via Graph.Events().
type Event struct {
	GraphID  string
	TaskID   string
	Status   TaskStatus
	Progress float64
	Message  string
	ErrMsg   string
	At       time.Time
}

// ─── RCA Provider ─────────────────────────────────────────────────────────────

// RCAProvider is satisfied by any AI backend capable of root cause analysis.
// The Orchestrator's primary AI provider can be adapted to this interface.
type RCAProvider interface {
	// Analyze describes *why* taskDesc failed with the given output and
	// returns a concise (≤ 200 word) root-cause report.
	Analyze(ctx context.Context, taskDesc, failureOutput string) (string, error)
}

// ─── Environment ──────────────────────────────────────────────────────────────

// Environment is the shared execution context passed to every ActionFunc.
// All methods are goroutine-safe.
type Environment struct {
	// Platform metadata (read-only after construction).
	OS        string // runtime.GOOS
	Arch      string // runtime.GOARCH
	IsTermux  bool
	IsWindows bool
	IsSBC     bool // Raspberry Pi / Orange Pi / SBC

	// Working directory for file I/O tools.
	Workspace string

	// Pluggable sub-systems.
	Logger   *slog.Logger
	Executor Executor       // Platform-specific command runner
	Store    *RollbackStore // Atomic rollback cache (.gorkbot/tmp)
	Pruner   *Pruner        // Smart output compression
	AI       RCAProvider    // Optional AI for root cause analysis (nil = disabled)

	data       map[string]interface{}
	mu         sync.RWMutex
	progressCh chan progressUpdate
}

type progressUpdate struct {
	taskID   string
	progress float64
}

// NewEnvironment constructs a production Environment with automatic platform
// detection. cacheDir should be a writable config directory.
func NewEnvironment(workspace, cacheDir string, logger *slog.Logger, ai RCAProvider) *Environment {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	_, isTermux := os.LookupEnv("TERMUX_VERSION")

	// Detect SBC via /proc/cpuinfo keywords.
	isSBC := false
	if cpuInfo, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		info := string(cpuInfo)
		for _, kw := range []string{"Raspberry Pi", "OrangePi", "RockPi", "Allwinner", "Amlogic"} {
			if contains(info, kw) {
				isSBC = true
				break
			}
		}
	}

	var exec Executor
	if goos == "windows" {
		exec = &WindowsExecutor{}
	} else {
		exec = &UnixExecutor{IsTermux: isTermux}
	}

	storeDir := filepath.Join(cacheDir, ".gorkbot", "tmp")
	store, _ := NewRollbackStore(storeDir)

	return &Environment{
		OS:         goos,
		Arch:       goarch,
		IsTermux:   isTermux,
		IsWindows:  goos == "windows",
		IsSBC:      isSBC,
		Workspace:  workspace,
		Logger:     logger,
		Executor:   exec,
		Store:      store,
		Pruner:     NewPruner(DefaultMaxLines, DefaultMaxChars),
		AI:         ai,
		data:       make(map[string]interface{}),
		progressCh: make(chan progressUpdate, 256),
	}
}

// Set stores a value in the shared data map.
func (e *Environment) Set(key string, value interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.data[key] = value
}

// Get retrieves a value from the shared data map.
func (e *Environment) Get(key string) (interface{}, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v, ok := e.data[key]
	return v, ok
}

// GetString is a convenience wrapper that coerces the result to string.
func (e *Environment) GetString(key string) string {
	v, ok := e.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// SetProgress reports this task's progress (0.0–1.0) to the TUI.
// Safe to call from any goroutine; excess updates are dropped silently.
func (e *Environment) SetProgress(taskID string, pct float64) {
	select {
	case e.progressCh <- progressUpdate{taskID, clamp(pct, 0, 1)}:
	default:
	}
}

// ─── Graph ────────────────────────────────────────────────────────────────────

// Graph is the central thread-safe store for a DAG execution session.
type Graph struct {
	ID      string
	tasks   map[string]*Task
	mu      sync.RWMutex
	eventCh chan Event
}

// NewGraph creates an empty named execution graph.
func NewGraph(id string) *Graph {
	return &Graph{
		ID:      id,
		tasks:   make(map[string]*Task),
		eventCh: make(chan Event, 512),
	}
}

// Add registers a Task. Returns an error if the ID is already taken.
func (g *Graph) Add(t *Task) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.tasks[t.ID]; exists {
		return fmt.Errorf("dag: task %q already registered", t.ID)
	}
	t.Status = StatusPending
	g.tasks[t.ID] = t
	return nil
}

// Get returns a task by ID (nil if not found).
func (g *Graph) Get(id string) *Task {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.tasks[id]
}

// All returns all tasks in stable alphabetical order.
func (g *Graph) All() []*Task {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := make([]string, 0, len(g.tasks))
	for id := range g.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Task, len(ids))
	for i, id := range ids {
		out[i] = g.tasks[id]
	}
	return out
}

// Events returns the read-only event channel. The TUI consumes from this.
func (g *Graph) Events() <-chan Event { return g.eventCh }

// Snapshot returns deep copies of all tasks (thread-safe, for serialisation).
func (g *Graph) Snapshot() []*Task {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Task, 0, len(g.tasks))
	for _, t := range g.tasks {
		out = append(out, t.Clone())
	}
	return out
}

// setStatus mutates a task's status and emits the corresponding Event.
// Must be called with g.mu NOT held.
func (g *Graph) setStatus(id string, st TaskStatus, msg, errMsg string) {
	g.mu.Lock()
	t, ok := g.tasks[id]
	if ok {
		t.Status = st
		switch st {
		case StatusRunning:
			t.StartedAt = time.Now()
		case StatusCompleted, StatusFailed, StatusSkipped, StatusRolledBack:
			t.CompletedAt = time.Now()
		}
		if errMsg != "" {
			t.ErrMsg = errMsg
		}
	}
	g.mu.Unlock()

	if ok {
		g.emit(Event{
			GraphID: g.ID,
			TaskID:  id,
			Status:  st,
			Message: msg,
			ErrMsg:  errMsg,
		})
	}
}

// emit sends an Event to the channel, dropping if the consumer is slow.
func (g *Graph) emit(ev Event) {
	ev.At = time.Now()
	select {
	case g.eventCh <- ev:
	default:
	}
}

// TopologicalOrder returns tasks sorted so dependencies always precede
// dependents, enabling correct parallel scheduling. Returns an error on
// unknown dependency references or cycle detection.
func (g *Graph) TopologicalOrder() ([]*Task, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	inDegree := make(map[string]int, len(g.tasks))
	for id := range g.tasks {
		inDegree[id] = 0
	}
	for _, t := range g.tasks {
		for _, dep := range t.Dependencies {
			if _, ok := g.tasks[dep]; !ok {
				return nil, fmt.Errorf("dag: task %q depends on unknown task %q", t.ID, dep)
			}
			inDegree[t.ID]++
		}
	}

	// Kahn's algorithm for topological sort.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // Deterministic ordering for reproducible graphs.

	var order []*Task
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, g.tasks[cur])
		for _, t := range g.tasks {
			for _, dep := range t.Dependencies {
				if dep == cur {
					inDegree[t.ID]--
					if inDegree[t.ID] == 0 {
						queue = append(queue, t.ID)
						sort.Strings(queue)
					}
				}
			}
		}
	}

	if len(order) != len(g.tasks) {
		return nil, fmt.Errorf("dag: cycle detected — %d tasks unreachable", len(g.tasks)-len(order))
	}
	return order, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
