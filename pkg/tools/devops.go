package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DockerManagerTool manages Docker containers.
type DockerManagerTool struct {
	BaseTool
}

func NewDockerManagerTool() *DockerManagerTool {
	return &DockerManagerTool{
		BaseTool: BaseTool{
			name:               "docker_manager",
			description:        "Manage Docker containers (list, start, stop, inspect, logs).",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *DockerManagerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "start", "stop", "inspect", "logs", "prune"},
				"description": "Action to perform.",
			},
			"container": map[string]interface{}{
				"type":        "string",
				"description": "Container ID or name (required for start/stop/inspect/logs).",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DockerManagerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	container, _ := args["container"].(string)

	var cmdArgs []string
	switch action {
	case "list":
		cmdArgs = []string{"ps", "-a"}
	case "start":
		cmdArgs = []string{"start", container}
	case "stop":
		cmdArgs = []string{"stop", container}
	case "inspect":
		cmdArgs = []string{"inspect", container}
	case "logs":
		cmdArgs = []string{"logs", "--tail", "50", container}
	case "prune":
		cmdArgs = []string{"system", "prune", "-f"}
	default:
		return &ToolResult{Success: false, Error: "Unknown action"}, nil
	}

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Docker command failed: %v\nOutput: %s", err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}

// K8sKubectlTool executes kubectl commands.
type K8sKubectlTool struct {
	BaseTool
}

func NewK8sKubectlTool() *K8sKubectlTool {
	return &K8sKubectlTool{
		BaseTool: BaseTool{
			name:               "k8s_kubectl",
			description:        "Execute kubectl commands to manage Kubernetes clusters.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *K8sKubectlTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The kubectl command arguments (e.g., 'get pods -n default').",
			},
		},
		"required": []string{"command"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *K8sKubectlTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return &ToolResult{Success: false, Error: "Missing command"}, nil
	}

	parts := strings.Fields(command)
	cmd := exec.CommandContext(ctx, "kubectl", parts...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Kubectl failed: %v\nOutput: %s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// AwsS3SyncTool syncs directories with S3.
type AwsS3SyncTool struct {
	BaseTool
}

func NewAwsS3SyncTool() *AwsS3SyncTool {
	return &AwsS3SyncTool{
		BaseTool: BaseTool{
			name:               "aws_s3_sync",
			description:        "Sync a local directory with an AWS S3 bucket.",
			category:           CategoryNetwork,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *AwsS3SyncTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source directory or S3 path.",
			},
			"dest": map[string]interface{}{
				"type":        "string",
				"description": "Destination S3 path or directory.",
			},
			"delete": map[string]interface{}{
				"type":        "boolean",
				"description": "Delete files in dest that don't exist in source.",
			},
		},
		"required": []string{"source", "dest"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *AwsS3SyncTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	source, _ := args["source"].(string)
	dest, _ := args["dest"].(string)
	del, _ := args["delete"].(bool)

	cmdArgs := []string{"s3", "sync", source, dest}
	if del {
		cmdArgs = append(cmdArgs, "--delete")
	}

	cmd := exec.CommandContext(ctx, "aws", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("AWS sync failed: %v\nOutput: %s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// GitBlameAnalyzeTool analyzes git blame for a file.
type GitBlameAnalyzeTool struct {
	BaseTool
}

func NewGitBlameAnalyzeTool() *GitBlameAnalyzeTool {
	return &GitBlameAnalyzeTool{
		BaseTool: BaseTool{
			name:               "git_blame_analyze",
			description:        "Run git blame on a file to analyze line history.",
			category:           CategoryGit,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *GitBlameAnalyzeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file.",
			},
			"lines": map[string]interface{}{
				"type":        "string",
				"description": "Line range (e.g., '10,20'). Optional.",
			},
		},
		"required": []string{"file"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GitBlameAnalyzeTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	file, _ := args["file"].(string)
	lines, _ := args["lines"].(string)

	cmdArgs := []string{"blame", file}
	if lines != "" {
		cmdArgs = append(cmdArgs, "-L", lines)
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Git blame failed: %v", err)}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// NgrokTunnelTool manages ngrok tunnels.
type NgrokTunnelTool struct {
	BaseTool
}

func NewNgrokTunnelTool() *NgrokTunnelTool {
	return &NgrokTunnelTool{
		BaseTool: BaseTool{
			name:               "ngrok_tunnel",
			description:        "Start an ngrok tunnel to expose a local port.",
			category:           CategoryNetwork,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *NgrokTunnelTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":        "string",
				"description": "Port to expose (e.g., '8080').",
			},
			"proto": map[string]interface{}{
				"type":        "string",
				"description": "Protocol (http, tcp). Default: http.",
			},
		},
		"required": []string{"port"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *NgrokTunnelTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	port, _ := args["port"].(string)
	proto, _ := args["proto"].(string)
	if proto == "" {
		proto = "http"
	}

	// Note: ngrok blocks, so we'd typically need to run it in background or use `start` command via API.
	// For CLI, we can't easily capture output and keep it running in a simple tool call without blocking.
	// We'll assume the user wants to start it in background via nohup or we use a short timeout.
	// BETTER: Use `start_managed_process`?
	// For this tool, let's just try to start it and check status via API, but sticking to CLI:
	// We'll return instructions or try to spawn.
	
	// Actually, `ngrok http 8080 --log=stdout > ngrok.log &`
	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("nohup ngrok %s %s > ngrok.log 2>&1 & echo $!", proto, port))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to start ngrok: %v", err)}, nil
	}
	pid := strings.TrimSpace(string(out))
	return &ToolResult{Success: true, Output: fmt.Sprintf("Ngrok started (PID: %s). Check ngrok.log for URL.", pid)}, nil
}

// CiTriggerTool triggers CI pipelines.
type CiTriggerTool struct {
	BaseTool
}

func NewCiTriggerTool() *CiTriggerTool {
	return &CiTriggerTool{
		BaseTool: BaseTool{
			name:               "ci_trigger",
			description:        "Trigger a CI pipeline (GitHub Actions via gh cli).",
			category:           CategoryGit,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *CiTriggerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workflow": map[string]interface{}{
				"type":        "string",
				"description": "Workflow filename or ID (e.g., 'test.yml').",
			},
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Git branch to run on.",
			},
		},
		"required": []string{"workflow"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CiTriggerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	workflow, _ := args["workflow"].(string)
	branch, _ := args["branch"].(string)

	cmdArgs := []string{"workflow", "run", workflow}
	if branch != "" {
		cmdArgs = append(cmdArgs, "--ref", branch)
	}

	cmd := exec.CommandContext(ctx, "gh", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("CI trigger failed (needs gh cli): %v\n%s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: "Workflow triggered successfully."}, nil
}
