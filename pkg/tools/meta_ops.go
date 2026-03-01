package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// CronManagerTool manages cron jobs.
type CronManagerTool struct {
	BaseTool
}

func NewCronManagerTool() *CronManagerTool {
	return &CronManagerTool{
		BaseTool: BaseTool{
			name:               "cron_manager",
			description:        "List or edit user cron jobs.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *CronManagerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "add_line", "clear"},
				"description": "Action.",
			},
			"line": map[string]interface{}{
				"type":        "string",
				"description": "Cron line to add (e.g., '* * * * * command').",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CronManagerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	line, _ := args["line"].(string)

	if action == "list" {
		cmd := exec.CommandContext(ctx, "crontab", "-l")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return &ToolResult{Success: true, Output: "No crontab for user."}, nil
		}
		return &ToolResult{Success: true, Output: string(out)}, nil
	} else if action == "add_line" {
		if line == "" {
			return &ToolResult{Success: false, Error: "Line required."}, nil
		}
		script := fmt.Sprintf("(crontab -l 2>/dev/null; echo \"%s\") | crontab -", line)
		cmd := exec.CommandContext(ctx, "bash", "-c", script)
		if err := cmd.Run(); err != nil {
			return &ToolResult{Success: false, Error: "Failed to update crontab."}, nil
		}
		return &ToolResult{Success: true, Output: "Added line to crontab."}, nil
	}
	return &ToolResult{Success: false, Error: "Unknown action."}, nil
}

// BackupRestoreTool uses rclone.
type BackupRestoreTool struct {
	BaseTool
}

func NewBackupRestoreTool() *BackupRestoreTool {
	return &BackupRestoreTool{
		BaseTool: BaseTool{
			name:               "backup_restore",
			description:        "Backup directory to cloud storage using rclone.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *BackupRestoreTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Local source path.",
			},
			"remote": map[string]interface{}{
				"type":        "string",
				"description": "Remote path (e.g., drive:backup).",
			},
		},
		"required": []string{"source", "remote"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *BackupRestoreTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	src, _ := args["source"].(string)
	rem, _ := args["remote"].(string)

	cmd := exec.CommandContext(ctx, "rclone", "copy", src, rem)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Rclone failed (configure rclone config first)."}, nil
	}
	return &ToolResult{Success: true, Output: "Backup complete. " + string(out)}, nil
}

// SystemMonitorTool (htop-like query).
type SystemMonitorTool struct {
	BaseTool
}

func NewSystemMonitorTool() *SystemMonitorTool {
	return &SystemMonitorTool{
		BaseTool: BaseTool{
			name:               "system_monitor",
			description:        "Get system resource usage snapshot (CPU, Mem, Disk).",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SystemMonitorTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SystemMonitorTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", "echo '--- MEMORY ---'; free -h; echo '\n--- DISK ---'; df -h; echo '\n--- TOP ---'; top -b -n 1 | head -20")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Monitor failed."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}
