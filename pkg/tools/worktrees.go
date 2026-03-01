package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ─── create_worktree ─────────────────────────────────────────────────────────

type CreateWorktreeTool struct {
	BaseTool
}

func NewCreateWorktreeTool() *CreateWorktreeTool {
	return &CreateWorktreeTool{
		BaseTool: NewBaseTool(
			"create_worktree",
			"Create an isolated git worktree for safe file editing. The worktree gets its own branch so changes never touch the main working tree. Returns the worktree path.",
			CategoryGit,
			false,
			PermissionSession,
		),
	}
}

func (t *CreateWorktreeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Short identifier for the worktree (e.g. 'refactor-auth'). Used as the branch suffix.",
			},
			"base": map[string]interface{}{
				"type":        "string",
				"description": "Git ref to branch from (default: HEAD).",
			},
		},
		"required": []string{"name"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CreateWorktreeTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return &ToolResult{Success: false, Error: "name is required"}, nil
	}
	base, _ := params["base"].(string)
	if base == "" {
		base = "HEAD"
	}

	repoRoot, err := gitRoot()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("not a git repo: %v", err)}, nil
	}

	path := filepath.Join(repoRoot, ".agent-worktrees", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("mkdir: %v", err)}, nil
	}

	branch := "worktree/" + name
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path, base)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("git worktree add: %v\n%s", err, strings.TrimSpace(string(out))),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Worktree created.\nPath:   %s\nBranch: %s\n\nFiles you edit under that path stay isolated from the main tree.", path, branch),
		Data:    map[string]interface{}{"path": path, "branch": branch},
	}, nil
}

// ─── list_worktrees ───────────────────────────────────────────────────────────

type ListWorktreesTool struct {
	BaseTool
}

func NewListWorktreesTool() *ListWorktreesTool {
	return &ListWorktreesTool{
		BaseTool: NewBaseTool(
			"list_worktrees",
			"List all active git worktrees for this repository.",
			CategoryGit,
			false,
			PermissionAlways,
		),
	}
}

func (t *ListWorktreesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *ListWorktreesTool) Execute(_ context.Context, _ map[string]interface{}) (*ToolResult, error) {
	repoRoot, err := gitRoot()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("not a git repo: %v", err)}, nil
	}

	cmd := exec.Command("git", "worktree", "list")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("git worktree list: %v", err)}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}

// ─── remove_worktree ─────────────────────────────────────────────────────────

type RemoveWorktreeTool struct {
	BaseTool
}

func NewRemoveWorktreeTool() *RemoveWorktreeTool {
	return &RemoveWorktreeTool{
		BaseTool: NewBaseTool(
			"remove_worktree",
			"Remove a git worktree created by create_worktree. Also deletes the associated branch.",
			CategoryGit,
			true, // destructive
			PermissionSession,
		),
	}
}

func (t *RemoveWorktreeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name used when creating the worktree.",
			},
		},
		"required": []string{"name"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *RemoveWorktreeTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return &ToolResult{Success: false, Error: "name is required"}, nil
	}

	repoRoot, err := gitRoot()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("not a git repo: %v", err)}, nil
	}

	path := filepath.Join(repoRoot, ".agent-worktrees", name)
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("git worktree remove: %v\n%s", err, strings.TrimSpace(string(out))),
		}, nil
	}

	// Best-effort branch cleanup.
	_ = exec.Command("git", "-C", repoRoot, "branch", "-D", "worktree/"+name).Run()

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Worktree '%s' removed and branch 'worktree/%s' deleted.", name, name),
	}, nil
}

// ─── integrate_worktree ──────────────────────────────────────────────────────

type IntegrateWorktreeTool struct {
	BaseTool
}

func NewIntegrateWorktreeTool() *IntegrateWorktreeTool {
	return &IntegrateWorktreeTool{
		BaseTool: NewBaseTool(
			"integrate_worktree",
			"Safely merge an agent worktree into the main branch. Performs pre-flight checks (build/test), checks for conflicts, and performs a safe merge. Removes the worktree upon success.",
			CategoryGit,
			true,
			PermissionSession,
		),
	}
}

func (t *IntegrateWorktreeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the worktree to integrate.",
			},
			"run_tests": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to run go build/test before merging. Default true.",
			},
		},
		"required": []string{"name"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *IntegrateWorktreeTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return &ToolResult{Success: false, Error: "name is required"}, nil
	}

	runTests := true
	if rt, ok := params["run_tests"].(bool); ok {
		runTests = rt
	}

	repoRoot, err := gitRoot()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("not a git repo: %v", err)}, nil
	}

	worktreePath := filepath.Join(repoRoot, ".agent-worktrees", name)
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return &ToolResult{Success: false, Error: "worktree does not exist: " + name}, nil
	}

	if runTests {
		// Run tests inside worktree
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = worktreePath
		if out, err := cmd.CombinedOutput(); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Pre-flight build failed:\n%s", string(out))}, nil
		}
		cmdTest := exec.Command("go", "test", "./...")
		cmdTest.Dir = worktreePath
		if out, err := cmdTest.CombinedOutput(); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Pre-flight tests failed:\n%s", string(out))}, nil
		}
	}

	branch := "worktree/" + name

	// Perform dry run to check for conflicts
	dryCmd := exec.Command("git", "merge", "--no-commit", "--no-ff", branch)
	dryCmd.Dir = repoRoot
	if out, err := dryCmd.CombinedOutput(); err != nil {
		exec.Command("git", "-C", repoRoot, "merge", "--abort").Run()
		return &ToolResult{Success: false, Error: fmt.Sprintf("Merge conflict detected or merge failed. Aborted.\n%s", string(out))}, nil
	}

	// Commit the merge
	commitCmd := exec.Command("git", "commit", "-m", "Auto-integrated agent worktree: "+name)
	commitCmd.Dir = repoRoot
	if out, err := commitCmd.CombinedOutput(); err != nil {
		exec.Command("git", "-C", repoRoot, "merge", "--abort").Run()
		return &ToolResult{Success: false, Error: fmt.Sprintf("Merge commit failed:\n%s", string(out))}, nil
	}

	// Cleanup
	rmCmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	rmCmd.Dir = repoRoot
	rmCmd.Run()
	exec.Command("git", "-C", repoRoot, "branch", "-D", branch).Run()

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Worktree '%s' successfully integrated and cleaned up.", name),
	}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func gitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
