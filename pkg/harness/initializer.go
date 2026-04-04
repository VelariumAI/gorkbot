package harness

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Initializer handles one-time project initialization.
type Initializer struct {
	store  *Store
	logger *slog.Logger
}

// NewInitializer creates a new Initializer.
func NewInitializer(store *Store, logger *slog.Logger) *Initializer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Initializer{store: store, logger: logger}
}

// IsInitialized checks whether the harness has been initialized.
func (i *Initializer) IsInitialized() bool {
	_, err := os.Stat(filepath.Join(i.store.HarnessDir(), "harness_state.json"))
	return err == nil
}

// Initialize sets up the harness for a new project.
func (i *Initializer) Initialize(ctx context.Context, goal string, features []FeatureInput) (*FeatureList, error) {
	if err := i.store.EnsureDir(); err != nil {
		return nil, fmt.Errorf("ensure harness dir: %w", err)
	}

	// Ensure git is initialized
	gitDir := filepath.Join(i.store.projectRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "git", "init")
		cmd.Dir = i.store.projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			i.logger.Warn("git init failed", "error", err, "output", string(out))
		}
	}

	now := time.Now()

	// Build feature list from inputs
	fl := &FeatureList{
		ProjectName: filepath.Base(i.store.projectRoot),
		Goal:        goal,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	for idx, fi := range features {
		feat := Feature{
			ID:          fmt.Sprintf("feat-%03d", idx+1),
			Title:       fi.Title,
			Description: fi.Description,
			Status:      StatusFailing,
			Dependencies: fi.Dependencies,
			Priority:    fi.Priority,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		fl.Features = append(fl.Features, feat)
	}

	// Save feature list
	if err := i.store.SaveFeatureList(fl); err != nil {
		return nil, fmt.Errorf("save feature list: %w", err)
	}

	// Save initial state
	state := &HarnessState{
		ProjectRoot:   i.store.projectRoot,
		Initialized:   true,
		LastBootAt:    now,
		TotalSessions: 1,
	}
	if err := i.store.SaveState(state); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	// Record initial progress
	if err := i.store.AppendProgress(ProgressEntry{
		Timestamp: now,
		Action:    "initialized",
		Details:   fmt.Sprintf("Project initialized with %d features. Goal: %s", len(features), goal),
	}); err != nil {
		i.logger.Warn("failed to write progress", "error", err)
	}

	return fl, nil
}
