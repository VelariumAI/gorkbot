// Package config — watcher.go
//
// ConfigWatcher polls the GORKBOT.md file hierarchy for changes and fires
// registered callbacks whenever a file is added, modified, or removed.
//
// Design: polling via os.Stat mtime rather than inotify/fsnotify. This keeps
// zero new dependencies and is fully portable across Linux, Android/Termux,
// macOS, and Windows. Poll interval defaults to 2 seconds — imperceptible to
// humans but fast enough that config edits feel live.
//
// Usage:
//
//	w := config.NewWatcher(loader, 2*time.Second)
//	w.OnChange(func(path string, event config.ChangeEvent) {
//	    newInstructions := loader.LoadInstructions()
//	    orchestrator.RefreshConfigContext(newInstructions)
//	})
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	go w.Start(ctx)
package config

import (
	"context"
	"os"
	"sync"
	"time"
)

// ChangeEvent describes what happened to a watched file.
type ChangeEvent int

const (
	// FileModified — file existed before and its mtime or size changed.
	FileModified ChangeEvent = iota
	// FileAdded — file was not present on the previous poll cycle.
	FileAdded
	// FileRemoved — file was present on the previous poll cycle but is now gone.
	FileRemoved
)

func (e ChangeEvent) String() string {
	switch e {
	case FileModified:
		return "modified"
	case FileAdded:
		return "added"
	case FileRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// ChangeHandler is called on the watcher's goroutine whenever a file changes.
// It receives the absolute path and the type of change.
type ChangeHandler func(path string, event ChangeEvent)

// fileState captures the state of a watched file for comparison.
type fileState struct {
	modTime time.Time
	size    int64
}

// ConfigWatcher polls the Loader's active files and notifies registered
// handlers when any of them change.
type ConfigWatcher struct {
	loader   *Loader
	interval time.Duration

	mu       sync.RWMutex
	handlers []ChangeHandler
	known    map[string]fileState // path → last observed state
}

// NewWatcher creates a ConfigWatcher backed by loader.
// interval controls how often the filesystem is polled (minimum 100ms).
func NewWatcher(loader *Loader, interval time.Duration) *ConfigWatcher {
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	return &ConfigWatcher{
		loader:   loader,
		interval: interval,
		known:    make(map[string]fileState),
	}
}

// OnChange registers a handler that is invoked whenever a watched file changes.
// Multiple handlers can be registered; they are called in registration order.
// Safe to call before or after Start.
func (w *ConfigWatcher) OnChange(h ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, h)
}

// Start begins polling. It runs until ctx is cancelled.
// Designed to be launched as a goroutine.
func (w *ConfigWatcher) Start(ctx context.Context) {
	// Seed initial state so the first poll doesn't fire spurious events.
	w.mu.Lock()
	w.seedLocked()
	w.mu.Unlock()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll checks the current active files against the last known state and fires
// handlers for any differences. Must not be called with w.mu held.
func (w *ConfigWatcher) poll() {
	// Collect current active file set.
	current := w.loader.ActiveFiles()
	currentSet := make(map[string]struct{}, len(current))
	for _, p := range current {
		currentSet[p] = struct{}{}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var events []struct {
		path  string
		event ChangeEvent
	}

	// Check for modifications and additions.
	for _, path := range current {
		info, err := os.Stat(path)
		if err != nil {
			// File was in the active list but can't be stat'd — treat as removed.
			if _, wasKnown := w.known[path]; wasKnown {
				delete(w.known, path)
				events = append(events, struct {
					path  string
					event ChangeEvent
				}{path, FileRemoved})
			}
			continue
		}

		st := fileState{modTime: info.ModTime(), size: info.Size()}

		if prev, known := w.known[path]; !known {
			// New file appeared.
			w.known[path] = st
			events = append(events, struct {
				path  string
				event ChangeEvent
			}{path, FileAdded})
		} else if prev.modTime != st.modTime || prev.size != st.size {
			// Existing file changed.
			w.known[path] = st
			events = append(events, struct {
				path  string
				event ChangeEvent
			}{path, FileModified})
		}
	}

	// Check for removals (was known, no longer in active set).
	for path := range w.known {
		if _, inCurrent := currentSet[path]; !inCurrent {
			delete(w.known, path)
			events = append(events, struct {
				path  string
				event ChangeEvent
			}{path, FileRemoved})
		}
	}

	// Fire handlers outside the hot path — make a snapshot of handlers first.
	if len(events) == 0 {
		return
	}
	handlers := make([]ChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)

	// Unlock so handlers can safely call back into the loader.
	w.mu.Unlock()
	for _, ev := range events {
		for _, h := range handlers {
			h(ev.path, ev.event)
		}
	}
	w.mu.Lock() // re-lock; deferred unlock will fire
}

// seedLocked snapshots the current state of all active files.
// Must be called with w.mu held.
func (w *ConfigWatcher) seedLocked() {
	for _, path := range w.loader.ActiveFiles() {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		w.known[path] = fileState{modTime: info.ModTime(), size: info.Size()}
	}
}

// Snapshot returns a copy of the current known file states (useful for tests
// and diagnostics).
func (w *ConfigWatcher) Snapshot() map[string]fileState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make(map[string]fileState, len(w.known))
	for k, v := range w.known {
		out[k] = v
	}
	return out
}
