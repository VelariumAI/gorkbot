package webui

import (
	"sync"
	"time"
)

// ToolRecord tracks execution of a single tool within a run.
type ToolRecord struct {
	Name      string
	Status    string     // "running", "complete", "error"
	StartTime time.Time
	EndTime   *time.Time
}

// RunRecord represents a single execution run with its metadata and tool usage.
type RunRecord struct {
	ID         string
	Prompt     string
	Status     string      // "running", "complete", "error"
	Model      string
	Provider   string
	StartTime  time.Time
	EndTime    *time.Time
	LatencyMS  int64
	TokensUsed int
	Tools      []ToolRecord
	ErrorMsg   string
}

// RunStore provides thread-safe storage of run records for live entity tracking.
type RunStore struct {
	mu   sync.RWMutex
	runs map[string]*RunRecord
	ids  []string // insertion order for List()
}

// NewRunStore creates a new empty run store.
func NewRunStore() *RunStore {
	return &RunStore{
		runs: make(map[string]*RunRecord),
		ids:  []string{},
	}
}

// Create initializes a new run record with the given parameters.
func (s *RunStore) Create(id, prompt, model, provider string) *RunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	run := &RunRecord{
		ID:        id,
		Prompt:    prompt,
		Status:    "running",
		Model:     model,
		Provider:  provider,
		StartTime: now,
		Tools:     []ToolRecord{},
	}

	s.runs[id] = run
	s.ids = append(s.ids, id)
	return run
}

// Get retrieves a run by ID. Returns (nil, false) if not found.
func (s *RunStore) Get(id string) (*RunRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[id]
	return run, ok
}

// List returns the N most recent runs (up to limit). Uses insertion order.
func (s *RunStore) List(limit int) []*RunRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*RunRecord
	start := len(s.ids) - limit
	if start < 0 {
		start = 0
	}

	// Reverse order: most recent first
	for i := len(s.ids) - 1; i >= start; i-- {
		if id := s.ids[i]; id != "" {
			if run, ok := s.runs[id]; ok {
				out = append(out, run)
			}
		}
	}
	return out
}

// ToolStart marks a tool as running within a run.
func (s *RunStore) ToolStart(runID, toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if run, ok := s.runs[runID]; ok {
		now := time.Now()
		run.Tools = append(run.Tools, ToolRecord{
			Name:      toolName,
			Status:    "running",
			StartTime: now,
		})
	}
}

// ToolDone marks a tool as complete within a run.
func (s *RunStore) ToolDone(runID, toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if run, ok := s.runs[runID]; ok {
		now := time.Now()
		for i := range run.Tools {
			if run.Tools[i].Name == toolName && run.Tools[i].Status == "running" {
				run.Tools[i].Status = "complete"
				run.Tools[i].EndTime = &now
				break
			}
		}
	}
}

// Complete marks a run as finished successfully with token count.
func (s *RunStore) Complete(runID string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if run, ok := s.runs[runID]; ok {
		now := time.Now()
		run.Status = "complete"
		run.EndTime = &now
		run.TokensUsed = tokens
		run.LatencyMS = now.Sub(run.StartTime).Milliseconds()
	}
}

// Fail marks a run as failed with an error message.
func (s *RunStore) Fail(runID, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if run, ok := s.runs[runID]; ok {
		now := time.Now()
		run.Status = "error"
		run.EndTime = &now
		run.ErrorMsg = errMsg
		run.LatencyMS = now.Sub(run.StartTime).Milliseconds()
	}
}
