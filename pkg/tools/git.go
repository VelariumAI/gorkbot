package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitStatusTool shows git repository status
type GitStatusTool struct {
	BaseTool
}

func NewGitStatusTool() *GitStatusTool {
	return &GitStatusTool{
		BaseTool: BaseTool{
			name:               "git_status",
			description:        "Show the working tree status of a git repository",
			category:           CategoryGit,
			requiresPermission: false, // Safe read-only operation
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *GitStatusTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"short": map[string]interface{}{
				"type":        "boolean",
				"description": "Show short format output (default: false)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitStatusTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	short := false
	if s, ok := params["short"].(bool); ok {
		short = s
	}

	flags := ""
	if short {
		flags = "-s"
	}

	command := fmt.Sprintf("git -C %s status %s", shellescape(path), flags)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GitDiffTool shows changes in repository
type GitDiffTool struct {
	BaseTool
}

func NewGitDiffTool() *GitDiffTool {
	return &GitDiffTool{
		BaseTool: BaseTool{
			name:               "git_diff",
			description:        "Show changes between commits, commit and working tree, etc",
			category:           CategoryGit,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *GitDiffTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"cached": map[string]interface{}{
				"type":        "boolean",
				"description": "Show staged changes (default: false)",
			},
			"file": map[string]interface{}{
				"type":        "string",
				"description": "Specific file to diff (optional)",
			},
			"commit": map[string]interface{}{
				"type":        "string",
				"description": "Compare against specific commit (optional)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitDiffTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	flags := ""
	if cached, ok := params["cached"].(bool); ok && cached {
		flags += " --cached"
	}

	target := ""
	if commit, ok := params["commit"].(string); ok {
		target = shellescape(commit)
	}

	file := ""
	if f, ok := params["file"].(string); ok {
		file = " -- " + shellescape(f)
	}

	command := fmt.Sprintf("git -C %s diff%s %s%s", shellescape(path), flags, target, file)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GitLogTool shows commit history
type GitLogTool struct {
	BaseTool
}

func NewGitLogTool() *GitLogTool {
	return &GitLogTool{
		BaseTool: BaseTool{
			name:               "git_log",
			description:        "Show commit logs",
			category:           CategoryGit,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *GitLogTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Number of commits to show (default: 10)",
			},
			"oneline": map[string]interface{}{
				"type":        "boolean",
				"description": "Show one line per commit (default: true)",
			},
			"graph": map[string]interface{}{
				"type":        "boolean",
				"description": "Show ASCII graph of branch/merge history (default: false)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitLogTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	limit := 10
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	oneline := true
	if o, ok := params["oneline"].(bool); ok {
		oneline = o
	}

	graph := false
	if g, ok := params["graph"].(bool); ok {
		graph = g
	}

	flags := fmt.Sprintf("-n %d", limit)
	if oneline {
		flags += " --oneline"
	}
	if graph {
		flags += " --graph"
	}

	command := fmt.Sprintf("git -C %s log %s", shellescape(path), flags)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GitCommitTool creates a commit
type GitCommitTool struct {
	BaseTool
}

func NewGitCommitTool() *GitCommitTool {
	return &GitCommitTool{
		BaseTool: BaseTool{
			name:               "git_commit",
			description:        "Record changes to the repository",
			category:           CategoryGit,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *GitCommitTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Commit message",
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Automatically stage modified/deleted files (default: false)",
			},
			"files": map[string]interface{}{
				"type":        "array",
				"description": "Specific files to add before committing",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"message"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitCommitTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	message, ok := params["message"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "message is required"}, fmt.Errorf("message required")
	}

	all := false
	if a, ok := params["all"].(bool); ok {
		all = a
	}

	// Build git add command if files specified
	addCommand := ""
	if files, ok := params["files"].([]interface{}); ok && len(files) > 0 {
		for _, f := range files {
			if file, ok := f.(string); ok {
				addCommand += fmt.Sprintf("git -C %s add %s && ", shellescape(path), shellescape(file))
			}
		}
	}

	flags := ""
	if all {
		flags = "-a"
	}

	// Escape message for shell
	escapedMsg := shellescape(message)

	command := fmt.Sprintf("%sgit -C %s commit %s -m %s", addCommand, shellescape(path), flags, escapedMsg)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GitPushTool pushes commits to remote
type GitPushTool struct {
	BaseTool
}

func NewGitPushTool() *GitPushTool {
	return &GitPushTool{
		BaseTool: BaseTool{
			name:               "git_push",
			description:        "Update remote refs along with associated objects",
			category:           CategoryGit,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *GitPushTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"remote": map[string]interface{}{
				"type":        "string",
				"description": "Remote name (default: origin)",
			},
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Branch to push (default: current branch)",
			},
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "Force push (default: false) - USE WITH CAUTION!",
			},
			"set_upstream": map[string]interface{}{
				"type":        "boolean",
				"description": "Set upstream for the branch (default: false)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitPushTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	remote := "origin"
	if r, ok := params["remote"].(string); ok {
		remote = r
	}

	branch := ""
	if b, ok := params["branch"].(string); ok {
		branch = b
	}

	force := false
	if f, ok := params["force"].(bool); ok {
		force = f
	}

	setUpstream := false
	if u, ok := params["set_upstream"].(bool); ok {
		setUpstream = u
	}

	flags := ""
	if force {
		flags += " --force"
	}
	if setUpstream {
		flags += " -u"
	}

	target := remote
	if branch != "" {
		target = fmt.Sprintf("%s %s", remote, branch)
	}

	command := fmt.Sprintf("git -C %s push%s %s", shellescape(path), flags, target)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GitPullTool fetches and integrates changes
type GitPullTool struct {
	BaseTool
}

func NewGitPullTool() *GitPullTool {
	return &GitPullTool{
		BaseTool: BaseTool{
			name:               "git_pull",
			description:        "Fetch from and integrate with another repository or local branch",
			category:           CategoryGit,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *GitPullTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Repository path (default: current directory)",
			},
			"remote": map[string]interface{}{
				"type":        "string",
				"description": "Remote name (default: origin)",
			},
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Branch to pull (optional)",
			},
			"rebase": map[string]interface{}{
				"type":        "boolean",
				"description": "Rebase instead of merge (default: false)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitPullTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	remote := "origin"
	if r, ok := params["remote"].(string); ok {
		remote = r
	}

	branch := ""
	if b, ok := params["branch"].(string); ok {
		branch = b
	}

	rebase := false
	if r, ok := params["rebase"].(bool); ok {
		rebase = r
	}

	flags := ""
	if rebase {
		flags = "--rebase"
	}

	target := remote
	if branch != "" {
		target = fmt.Sprintf("%s %s", remote, branch)
	}

	command := fmt.Sprintf("git -C %s pull %s %s", shellescape(path), flags, target)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
