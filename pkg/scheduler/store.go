package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store persists scheduled tasks to disk as JSON.
type Store struct {
	mu       sync.RWMutex
	filePath string
	tasks    map[string]ScheduledTask
}

// NewStore creates or loads the task store from configDir.
func NewStore(configDir string) (*Store, error) {
	s := &Store{
		filePath: filepath.Join(configDir, "scheduled_tasks.json"),
		tasks:    make(map[string]ScheduledTask),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("scheduler store: load: %w", err)
	}
	return s, nil
}

// Add saves a new task to the store.
func (s *Store) Add(task ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return s.save()
}

// Get fetches a task by ID.
func (s *Store) Get(id string) (*ScheduledTask, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	copy := t
	return &copy, true
}

// List returns all tasks.
func (s *Store) List() []ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	return out
}

// Update saves changes to an existing task.
func (s *Store) Update(task ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return s.save()
}

// Delete removes a task by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
	return s.save()
}

// SetStatus is a shorthand to update only the status of a task.
func (s *Store) SetStatus(id string, status TaskStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	t.Status = status
	s.tasks[id] = t
	return s.save()
}

// save writes the task map to disk with 0600 permissions.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// load reads tasks from disk.
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.tasks)
}
