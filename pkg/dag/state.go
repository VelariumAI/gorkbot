package dag

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func init() {
	// Register concrete types stored in interface{} fields so gob can encode
	// and decode them. Add more types here as Task.Result variants are introduced.
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register("")
	gob.Register(int64(0))
	gob.Register(float64(0))
	gob.Register(true)
	gob.Register(time.Time{})
}

// PersistedState is the gob-serialisable snapshot of a Graph execution.
// ActionFuncs and RollbackFuncs are not serialised (they are functions);
// callers must re-register them on the restored Graph via RestoreActions.
type PersistedState struct {
	SchemaVersion uint8 // Bumped when the format changes (current: 1)
	GraphID       string
	SavedAt       time.Time
	EnvData       map[string]interface{} // Environment.data snapshot
	Tasks         []*Task                // Deep copies — see Task.Clone()
}

// SaveState serialises the Graph and Environment data to a binary gob file.
// The file is written atomically (temp → rename) to protect against partial
// writes on Ctrl+C or force-quit.
//
// path should be a stable filename such as ~/.config/gorkbot/dag_<graphID>.gob
func SaveState(path string, g *Graph, env *Environment) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("dag state: mkdir: %w", err)
	}

	env.mu.RLock()
	envData := make(map[string]interface{}, len(env.data))
	for k, v := range env.data {
		envData[k] = v
	}
	env.mu.RUnlock()

	state := &PersistedState{
		SchemaVersion: 1,
		GraphID:       g.ID,
		SavedAt:       time.Now(),
		EnvData:       envData,
		Tasks:         g.Snapshot(),
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("dag state: encode: %w", err)
	}

	// Atomic write: temp file → rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("dag state: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("dag state: rename: %w", err)
	}
	return nil
}

// LoadState deserialises a PersistedState from path.
// After loading, callers must:
//  1. Reconstruct a Graph from state.Tasks via RestoreGraph.
//  2. Re-register ActionFuncs and RollbackFuncs (they are not serialised).
func LoadState(path string) (*PersistedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dag state: read: %w", err)
	}

	var state PersistedState
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&state); err != nil {
		return nil, fmt.Errorf("dag state: decode: %w", err)
	}
	if state.SchemaVersion != 1 {
		return nil, fmt.Errorf("dag state: unsupported schema version %d", state.SchemaVersion)
	}
	return &state, nil
}

// RestoreGraph reconstructs a Graph from a PersistedState.
// Task actions must be re-registered separately — see RegisterActions.
func RestoreGraph(state *PersistedState) *Graph {
	g := NewGraph(state.GraphID)
	for _, t := range state.Tasks {
		// Reset runtime-only fields to their pending defaults so the resolver
		// can re-drive tasks that were interrupted mid-execution.
		if t.Status == StatusRunning || t.Status == StatusQueued {
			t.Status = StatusPending
			t.StartedAt = time.Time{}
			t.CompletedAt = time.Time{}
		}
		g.mu.Lock()
		g.tasks[t.ID] = t
		g.mu.Unlock()
	}
	return g
}

// RestoreEnv repopulates an Environment's data map from a PersistedState.
func RestoreEnv(state *PersistedState, env *Environment) {
	env.mu.Lock()
	defer env.mu.Unlock()
	for k, v := range state.EnvData {
		env.data[k] = v
	}
}

// ActionRegistry maps Task IDs to their ActionFunc and RollbackFunc.
// Used to re-wire callbacks after deserialization.
type ActionRegistry map[string]struct {
	Action   ActionFunc
	Rollback RollbackFunc
}

// RegisterActions wires the ActionFuncs and RollbackFuncs from reg back into
// the tasks of g. Tasks whose IDs are not in reg keep nil callbacks (they will
// fail with "no Action defined" when the resolver reaches them).
func RegisterActions(g *Graph, reg ActionRegistry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, fns := range reg {
		if t, ok := g.tasks[id]; ok {
			t.Action = fns.Action
			t.Rollback = fns.Rollback
		}
	}
}

// DefaultStatePath returns the conventional path for a graph's state file.
// configDir is the application config directory (e.g. ~/.config/gorkbot).
func DefaultStatePath(configDir, graphID string) string {
	safe := filepath.Base(graphID) // strip any path traversal
	return filepath.Join(configDir, "dag", safe+".gob")
}
