package memory

// goals.go — Prospective Memory: Goal Ledger.
//
// Persists open goals across sessions. At session start, any open goals are
// injected into the system prompt so the AI remembers what it was working on.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GoalStatus represents the lifecycle state of a goal.
type GoalStatus string

const (
	GoalOpen     GoalStatus = "open"
	GoalDone     GoalStatus = "done"
	GoalDeferred GoalStatus = "deferred"
)

// Goal is a persisted intent that survives across sessions.
type Goal struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      GoalStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Notes       string     `json:"notes,omitempty"`
}

// GoalLedger persists goals to disk and surfaces open ones at session start.
type GoalLedger struct {
	mu    sync.RWMutex
	path  string
	Goals []*Goal `json:"goals"`
}

// NewGoalLedger opens (or creates) the goal ledger at configDir/goals.json.
func NewGoalLedger(configDir string) (*GoalLedger, error) {
	path := filepath.Join(configDir, "goals.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	gl := &GoalLedger{path: path}
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, gl)
	}
	if gl.Goals == nil {
		gl.Goals = make([]*Goal, 0)
	}
	return gl, nil
}

// AddGoal adds a new open goal and persists it. Returns the new goal's ID.
func (gl *GoalLedger) AddGoal(description string) string {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	g := &Goal{
		ID:          uuid.New().String()[:8],
		Description: description,
		Status:      GoalOpen,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	gl.Goals = append(gl.Goals, g)
	gl.persist()
	return g.ID
}

// CloseGoal marks a goal as done by ID.
func (gl *GoalLedger) CloseGoal(id string) bool {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	for _, g := range gl.Goals {
		if g.ID == id {
			g.Status = GoalDone
			g.UpdatedAt = time.Now()
			gl.persist()
			return true
		}
	}
	return false
}

// DeferGoal marks a goal as deferred by ID.
func (gl *GoalLedger) DeferGoal(id string) bool {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	for _, g := range gl.Goals {
		if g.ID == id {
			g.Status = GoalDeferred
			g.UpdatedAt = time.Now()
			gl.persist()
			return true
		}
	}
	return false
}

// OpenGoals returns all goals with status "open".
func (gl *GoalLedger) OpenGoals() []*Goal {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	var out []*Goal
	for _, g := range gl.Goals {
		if g.Status == GoalOpen {
			out = append(out, g)
		}
	}
	return out
}

// AllGoals returns all goals regardless of status.
func (gl *GoalLedger) AllGoals() []*Goal {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	out := make([]*Goal, len(gl.Goals))
	copy(out, gl.Goals)
	return out
}

// FormatBrief returns a compact Markdown block of open goals for prompt injection.
// Returns "" if there are no open goals.
func (gl *GoalLedger) FormatBrief() string {
	open := gl.OpenGoals()
	if len(open) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Open Goals from Prior Sessions:\n")
	for _, g := range open {
		age := time.Since(g.CreatedAt)
		days := int(age.Hours() / 24)
		if days == 0 {
			sb.WriteString(fmt.Sprintf("- [%s] %s (since today)\n", g.ID, g.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- [%s] %s (%d days ago)\n", g.ID, g.Description, days))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// persist writes the ledger to disk (must be called with write lock held).
func (gl *GoalLedger) persist() {
	data, err := json.MarshalIndent(gl, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(gl.path, data, 0600)
}
