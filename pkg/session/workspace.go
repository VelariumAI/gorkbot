package session

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WorkspaceManager manages git-based workspace checkpoints for fearless rollbacks.
type WorkspaceManager struct {
	rootDir string
}

// NewWorkspaceManager creates a new workspace manager for the given directory.
func NewWorkspaceManager(dir string) *WorkspaceManager {
	return &WorkspaceManager{rootDir: dir}
}

// RootDir returns the root directory this manager was initialized with.
func (wm *WorkspaceManager) RootDir() string { return wm.rootDir }

// IsGitRepo checks if the current directory is within a git repository.
func (wm *WorkspaceManager) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = wm.rootDir
	return cmd.Run() == nil
}

// CreateCheckpoint creates a hidden git stash commit of the current state and returns its hash.
func (wm *WorkspaceManager) CreateCheckpoint() (string, error) {
	if !wm.IsGitRepo() {
		return "", fmt.Errorf("not a git repository")
	}

	// Use git stash create to create a dangling commit with the current working tree state
	// It doesn't modify the working tree or the stash list.
	// Note: 'git stash create' only stashes tracked files. We may want to add untracked?
	// But 'git stash store' is needed if we want it in the reflog. Let's just create a commit.

	// A simpler and safer approach that includes untracked files:
	// We'll just run git stash push -u -m "gorkbot_checkpoint" and immediately apply it so it stays in stash list for easy rollback.
	// Actually, the requirement is "create a hidden git stash or temporary branch."

	msg := fmt.Sprintf("gorkbot_checkpoint_%d", time.Now().Unix())

	// 'git stash create' doesn't include untracked.
	// Let's use 'git stash push --include-untracked -m "..."' and then 'git stash apply'
	// Wait, 'git stash push' modifies the working tree (it resets it). Then 'apply' restores it.
	// But what if apply has conflicts? It shouldn't, because we just stashed it.

	cmdPush := exec.Command("git", "stash", "push", "--include-untracked", "-m", msg)
	cmdPush.Dir = wm.rootDir
	if out, err := cmdPush.CombinedOutput(); err != nil {
		// If there are no local changes, stash push returns an error or "No local changes to save"
		if strings.Contains(string(out), "No local changes") {
			return "NO_CHANGES", nil
		}
		return "", fmt.Errorf("git stash push failed: %s %v", string(out), err)
	}

	// Now apply it back immediately so the user/agent can keep working.
	cmdApply := exec.Command("git", "stash", "apply", "stash@{0}")
	cmdApply.Dir = wm.rootDir
	if out, err := cmdApply.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git stash apply failed: %s %v", string(out), err)
	}

	// Find the stash commit hash
	cmdRev := exec.Command("git", "rev-parse", "stash@{0}")
	cmdRev.Dir = wm.rootDir
	revOut, err := cmdRev.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v", err)
	}

	return strings.TrimSpace(string(revOut)), nil
}

// RestoreCheckpoint restores the workspace to a specific git stash hash.
func (wm *WorkspaceManager) RestoreCheckpoint(hash string) error {
	if !wm.IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}
	if hash == "NO_CHANGES" || hash == "" {
		return nil
	}

	// Reset working tree to HEAD (warning: destructive to current uncommitted)
	cmdReset := exec.Command("git", "reset", "--hard", "HEAD")
	cmdReset.Dir = wm.rootDir
	if err := cmdReset.Run(); err != nil {
		return fmt.Errorf("git reset failed: %v", err)
	}

	// Clean untracked files
	cmdClean := exec.Command("git", "clean", "-fd")
	cmdClean.Dir = wm.rootDir
	if err := cmdClean.Run(); err != nil {
		return fmt.Errorf("git clean failed: %v", err)
	}

	// Apply the stash hash
	cmdApply := exec.Command("git", "stash", "apply", hash)
	cmdApply.Dir = wm.rootDir
	if out, err := cmdApply.CombinedOutput(); err != nil {
		return fmt.Errorf("git stash apply %s failed: %s %v", hash, string(out), err)
	}

	return nil
}
