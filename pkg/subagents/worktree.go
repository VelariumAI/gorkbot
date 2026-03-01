package subagents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeManager creates and manages temporary git worktrees for isolated agent execution.
// Each agent gets its own worktree so file mutations do not affect the main working tree.
type WorktreeManager struct {
	// BaseDir is the root of the git repository (where .git lives).
	BaseDir string
}

// NewWorktreeManager creates a WorktreeManager rooted at baseDir.
func NewWorktreeManager(baseDir string) *WorktreeManager {
	return &WorktreeManager{BaseDir: baseDir}
}

// Create adds a git worktree for agentID and returns its filesystem path.
// The branch agent/<agentID> is created from HEAD.
func (wm *WorktreeManager) Create(agentID string) (string, error) {
	path := filepath.Join(wm.BaseDir, ".agent-worktrees", agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("mkdir worktree parent: %w", err)
	}
	branch := "agent/" + agentID
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path, "HEAD")
	cmd.Dir = wm.BaseDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return path, nil
}

// Remove tears down the worktree and branch for agentID.
func (wm *WorktreeManager) Remove(agentID string) error {
	path := filepath.Join(wm.BaseDir, ".agent-worktrees", agentID)
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = wm.BaseDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// Best-effort branch cleanup — ignore errors (branch may not exist).
	_ = exec.Command("git", "-C", wm.BaseDir, "branch", "-D", "agent/"+agentID).Run()
	return nil
}

// List returns a porcelain listing of all worktrees in the repo.
func (wm *WorktreeManager) List() (string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = wm.BaseDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}
	return string(out), nil
}
