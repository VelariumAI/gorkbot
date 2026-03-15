package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RollbackStore implements atomic rollback for file-modifying tasks.
//
// Before a task modifies any files, it snapshots the originals into the cache
// directory (.gorkbot/tmp/<taskID>/). If the task fails mid-way, calling
// Rollback restores the originals, leaving the workspace in the state it was in
// before the task ran.
//
// This mirrors git's stash semantics but is 100% self-contained — no git
// repository is required.
type RollbackStore struct {
	cacheDir string // e.g. ~/.config/gorkbot/.gorkbot/tmp
}

// snapshotMeta records what was captured for a given task.
type snapshotMeta struct {
	TaskID    string    `json:"task_id"`
	CreatedAt time.Time `json:"created_at"`
	Files     []string  `json:"files"`
}

// NewRollbackStore creates a RollbackStore backed by cacheDir.
// The directory is created if it does not exist.
func NewRollbackStore(cacheDir string) (*RollbackStore, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("rollback store: mkdir %q: %w", cacheDir, err)
	}
	return &RollbackStore{cacheDir: cacheDir}, nil
}

// taskDir returns the per-task snapshot directory.
func (s *RollbackStore) taskDir(taskID string) string {
	return filepath.Join(s.cacheDir, filepath.Base(taskID))
}

// Snapshot copies the named files into the store before a task modifies them.
// Files that do not exist yet are recorded with a ".new" sentinel so Rollback
// can delete them if the task needs to be undone.
//
// paths should be absolute paths or workspace-relative paths.
func (s *RollbackStore) Snapshot(taskID string, paths []string) error {
	dir := s.taskDir(taskID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("rollback snapshot: mkdir: %w", err)
	}

	meta := snapshotMeta{
		TaskID:    taskID,
		CreatedAt: time.Now(),
		Files:     paths,
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			// File doesn't exist yet — mark it so Rollback can delete it.
			sentinel := filepath.Join(dir, encodeRBName(p)+".new")
			if err2 := os.WriteFile(sentinel, nil, 0o600); err2 != nil {
				return fmt.Errorf("rollback snapshot: sentinel %q: %w", p, err2)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("rollback snapshot: read %q: %w", p, err)
		}
		dest := filepath.Join(dir, encodeRBName(p))
		if err2 := os.WriteFile(dest, data, 0o600); err2 != nil {
			return fmt.Errorf("rollback snapshot: cache %q: %w", p, err2)
		}
	}

	metaData, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), metaData, 0o600)
	return nil
}

// Rollback restores all files captured for taskID to their pre-task state.
// Files that were newly created by the task are deleted.
// The snapshot directory is removed on success.
func (s *RollbackStore) Rollback(_ context.Context, taskID string) error {
	dir := s.taskDir(taskID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("rollback: no snapshot found for task %q", taskID)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("rollback: readdir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "meta.json" {
			continue
		}

		if strings.HasSuffix(name, ".new") {
			// File was created by the task — remove it.
			orig := decodeRBName(strings.TrimSuffix(name, ".new"))
			if err2 := os.Remove(orig); err2 != nil && !os.IsNotExist(err2) {
				return fmt.Errorf("rollback: remove %q: %w", orig, err2)
			}
			continue
		}

		// Restore the original content.
		data, err2 := os.ReadFile(filepath.Join(dir, name))
		if err2 != nil {
			return fmt.Errorf("rollback: read cache: %w", err2)
		}
		orig := decodeRBName(name)
		if err3 := os.MkdirAll(filepath.Dir(orig), 0o755); err3 != nil {
			return fmt.Errorf("rollback: mkdir: %w", err3)
		}
		if err3 := os.WriteFile(orig, data, 0o644); err3 != nil {
			return fmt.Errorf("rollback: restore %q: %w", orig, err3)
		}
	}

	return os.RemoveAll(dir)
}

// Discard removes the snapshot for taskID without restoring files.
// Call after a task completes successfully — the snapshot is no longer needed.
func (s *RollbackStore) Discard(taskID string) error {
	return os.RemoveAll(s.taskDir(taskID))
}

// ListSnapshots returns the IDs of tasks that have live snapshots.
func (s *RollbackStore) ListSnapshots() ([]string, error) {
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// encodeRBName converts an absolute path to a safe single-directory filename
// by replacing OS path separators with a visually distinct Unicode character.
func encodeRBName(path string) string {
	return strings.ReplaceAll(path, string(os.PathSeparator), "⧸")
}

func decodeRBName(name string) string {
	return strings.ReplaceAll(name, "⧸", string(os.PathSeparator))
}
