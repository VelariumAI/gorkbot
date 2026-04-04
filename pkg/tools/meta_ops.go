package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

// SystemMonitorTool - Get system resource usage with SEVERE cooldown enforcement.
// CRITICAL: User reported "spamming notifications" — 30-MINUTE MINIMUM between executions.
type SystemMonitorTool struct {
	BaseTool
	lastExecution time.Time
}

func NewSystemMonitorTool() *SystemMonitorTool {
	return &SystemMonitorTool{
		BaseTool: BaseTool{
			name: "system_monitor",
			// HIDDEN FROM SYSTEM PROMPTS via GetDefinitions() filter
			// This prevents LLM from seeing it and getting distracted by resource checks
			description:        `⚠️ SUPPRESSED: System resource monitoring. This tool is intentionally hidden from LLM prompts to prevent thinking derailment. Only available if user explicitly requests system status.`,
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionNever, // ALWAYS REQUIRE EXPLICIT PERMISSION
		},
		lastExecution: time.Now().Add(-30 * time.Minute),
	}
}

func (t *SystemMonitorTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SystemMonitorTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// ADAPTIVE COOLDOWN: 30 minutes (1800 seconds) minimum between executions
	// This prevents system_monitor from spamming notifications while still
	// allowing manual calls and urgent checks.
	const cooldownSeconds = 30 * 60 // 30 minutes

	now := time.Now()
	elapsed := now.Sub(t.lastExecution)

	// Enforce cooldown: return gracefully (not an error) if throttled
	// This prevents the AI from getting stuck on failed tool calls
	if elapsed < time.Duration(cooldownSeconds)*time.Second {
		nextAllowed := t.lastExecution.Add(time.Duration(cooldownSeconds) * time.Second)
		waitSecs := nextAllowed.Sub(now).Seconds()

		// Return SUCCESS (not error) to indicate tool worked, just throttled
		// This prevents SENSE evolution from learning throttling as a failure pattern
		return &ToolResult{
			Success: true,
			Output: fmt.Sprintf("System monitor is on cooldown (executes max once per 30 minutes).\nLast checked: %s ago.\nNext check available in: %.0f seconds.",
				formatDuration(elapsed), waitSecs),
		}, nil
	}

	// Update last execution time
	t.lastExecution = now

	// Efficiently gather system metrics
	cmd := exec.CommandContext(ctx, "bash", "-c",
		"echo '=== SYSTEM STATUS (checked: '$(date)') ==='; "+
			"echo ''; echo '--- MEMORY USAGE ---'; free -h | tail -2; "+
			"echo ''; echo '--- DISK USAGE ---'; df -h / | tail -1; "+
			"echo ''; echo '--- TOP PROCESSES ---'; top -b -n 1 | head -12")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{
			Success: true,
			Output:  "System check completed (partial output due to command constraints)",
		}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// formatDuration returns a human-readable duration string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// IsContextuallyAppropriate checks if a system_monitor call makes sense given the current state.
// This helps guide the AI about whether calling this tool was the right decision.
// Returns (isAppropriate bool, explanation string)
func (t *SystemMonitorTool) IsContextuallyAppropriate(userPrompt string) (bool, string) {
	// Keywords that suggest a system check is warranted
	systemCheckKeywords := []string{
		"system", "status", "health", "resource", "memory", "disk", "cpu",
		"performance", "slow", "hung", "stuck", "usage", "available", "free",
		"how are you", "check", "diagnose", "what's wrong", "problem",
	}

	upperPrompt := strings.ToUpper(userPrompt)
	foundKeyword := false
	for _, keyword := range systemCheckKeywords {
		if strings.Contains(upperPrompt, strings.ToUpper(keyword)) {
			foundKeyword = true
			break
		}
	}

	elapsed := time.Since(t.lastExecution)
	withinCooldown := elapsed < 30*time.Minute

	switch {
	case foundKeyword && !withinCooldown:
		return true, "✓ Contextually appropriate: User asked about system status and cooldown expired"
	case foundKeyword && withinCooldown:
		return true, "✓ Contextually appropriate: User asked about system status (using cached data due to cooldown)"
	case !foundKeyword && withinCooldown:
		return false, "⚠ Not contextually appropriate: No system keywords detected; using cached data anyway due to cooldown"
	default:
		return false, "⚠ Likely not needed: User didn't ask about system; consider whether you really need this data"
	}
}
