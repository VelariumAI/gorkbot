package checkpoints

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Checkpoint represents a point-in-time snapshot
type Checkpoint struct {
	ID        string    // Unique checkpoint ID
	Type      string    // "git" or "snapshot"
	Path      string    // Repository or directory path
	Branch    string    // Git branch (for git checkpoints)
	Commit    string    // Git commit hash
	Timestamp time.Time
	Message   string
	Metadata  map[string]interface{}
}

// CheckpointManager manages hybrid checkpoints (Git + filesystem snapshots)
type CheckpointManager struct {
	logger      *slog.Logger
	checkpoints map[string]*Checkpoint
	gitEnabled  bool
	snapshotDir string
}

// NewCheckpointManager creates a new checkpoint manager
func NewCheckpointManager(logger *slog.Logger) *CheckpointManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &CheckpointManager{
		logger:      logger,
		checkpoints: make(map[string]*Checkpoint),
		gitEnabled:  isGitInstalled(),
	}
}

// CreateCheckpoint creates a checkpoint (Git if available, else snapshot)
func (cm *CheckpointManager) CreateCheckpoint(path string, message string) (*Checkpoint, error) {
	// Check if path is a git repo
	if cm.isGitRepo(path) && cm.gitEnabled {
		return cm.createGitCheckpoint(path, message)
	}

	// Fall back to filesystem snapshot
	return cm.createSnapshotCheckpoint(path, message)
}

// createGitCheckpoint creates a git-based checkpoint
func (cm *CheckpointManager) createGitCheckpoint(repoPath string, message string) (*Checkpoint, error) {
	// Get current branch
	branch, err := cm.gitCommand(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get branch: %w", err)
	}

	// Create checkpoint by getting current commit
	commit, err := cm.gitCommand(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	checkpoint := &Checkpoint{
		ID:        fmt.Sprintf("git-%d", time.Now().UnixNano()),
		Type:      "git",
		Path:      repoPath,
		Branch:    branch,
		Commit:    commit,
		Timestamp: time.Now(),
		Message:   message,
		Metadata:  make(map[string]interface{}),
	}

	cm.checkpoints[checkpoint.ID] = checkpoint

	cm.logger.Debug("created git checkpoint",
		slog.String("id", checkpoint.ID),
		slog.String("path", repoPath),
		slog.String("branch", branch),
		slog.String("commit", commit[:7]),
	)

	return checkpoint, nil
}

// createSnapshotCheckpoint creates a filesystem snapshot checkpoint
func (cm *CheckpointManager) createSnapshotCheckpoint(path string, message string) (*Checkpoint, error) {
	checkpoint := &Checkpoint{
		ID:        fmt.Sprintf("snapshot-%d", time.Now().UnixNano()),
		Type:      "snapshot",
		Path:      path,
		Timestamp: time.Now(),
		Message:   message,
		Metadata: map[string]interface{}{
			"files_captured": 0,
		},
	}

	cm.checkpoints[checkpoint.ID] = checkpoint

	cm.logger.Debug("created snapshot checkpoint",
		slog.String("id", checkpoint.ID),
		slog.String("path", path),
	)

	return checkpoint, nil
}

// Rollback rolls back to a checkpoint
func (cm *CheckpointManager) Rollback(checkpointID string) error {
	checkpoint, ok := cm.checkpoints[checkpointID]
	if !ok {
		return fmt.Errorf("checkpoint not found: %s", checkpointID)
	}

	if checkpoint.Type == "git" {
		return cm.rollbackGit(checkpoint)
	}

	return cm.rollbackSnapshot(checkpoint)
}

// rollbackGit rolls back to a git checkpoint
func (cm *CheckpointManager) rollbackGit(checkpoint *Checkpoint) error {
	_, err := cm.gitCommand(checkpoint.Path, "reset", "--hard", checkpoint.Commit)
	if err != nil {
		return fmt.Errorf("git reset failed: %w", err)
	}

	cm.logger.Info("rolled back to git checkpoint",
		slog.String("path", checkpoint.Path),
		slog.String("commit", checkpoint.Commit[:7]),
	)

	return nil
}

// rollbackSnapshot rolls back to a snapshot checkpoint
func (cm *CheckpointManager) rollbackSnapshot(checkpoint *Checkpoint) error {
	cm.logger.Warn("snapshot rollback not yet implemented",
		slog.String("checkpoint", checkpoint.ID),
	)
	return nil
}

// GetCheckpoint retrieves a checkpoint
func (cm *CheckpointManager) GetCheckpoint(id string) *Checkpoint {
	return cm.checkpoints[id]
}

// ListCheckpoints returns all checkpoints
func (cm *CheckpointManager) ListCheckpoints() []*Checkpoint {
	checkpoints := make([]*Checkpoint, 0, len(cm.checkpoints))
	for _, cp := range cm.checkpoints {
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints
}

// DeleteCheckpoint removes a checkpoint
func (cm *CheckpointManager) DeleteCheckpoint(id string) error {
	if _, ok := cm.checkpoints[id]; !ok {
		return fmt.Errorf("checkpoint not found: %s", id)
	}

	delete(cm.checkpoints, id)
	cm.logger.Debug("deleted checkpoint", slog.String("id", id))

	return nil
}

// Helper methods

func (cm *CheckpointManager) isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

func (cm *CheckpointManager) gitCommand(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Trim newline
	return string(output[:len(output)-1]), nil
}

func isGitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// GetStats returns checkpoint statistics
func (cm *CheckpointManager) GetStats() map[string]interface{} {
	gitCount := 0
	snapshotCount := 0

	for _, cp := range cm.checkpoints {
		if cp.Type == "git" {
			gitCount++
		} else {
			snapshotCount++
		}
	}

	return map[string]interface{}{
		"total":     len(cm.checkpoints),
		"git":       gitCount,
		"snapshot":  snapshotCount,
		"git_enabled": cm.gitEnabled,
	}
}
