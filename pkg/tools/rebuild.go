package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RebuildTool rebuilds Gorkbot from source
type RebuildTool struct {
	BaseTool
}

// NewRebuildTool creates a new rebuild tool
func NewRebuildTool() *RebuildTool {
	return &RebuildTool{
		BaseTool: NewBaseTool(
			"rebuild_gorkbot",
			"Rebuild Gorkbot from source (go build) and prepare for hot-restart. Use after editing source files to apply changes. The binary will be rebuilt at bin/gorkbot.",
			CategoryMeta,
			true,
			PermissionSession,
		),
	}
}

// Parameters returns the JSON schema for the rebuild tool
func (t *RebuildTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"restart_after": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, write restart_pending.json so TUI prompts user to restart (default: true)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Optional message to display after restart",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

// Execute runs the rebuild
func (t *RebuildTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	// Default restart_after to true
	restartAfter := true
	if ra, ok := params["restart_after"].(bool); ok {
		restartAfter = ra
	}

	message := ""
	if msg, ok := params["message"].(string); ok {
		message = msg
	}

	// Find project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("cannot determine project root: %v", err),
		}, nil
	}

	// Find go binary
	goBin, err := exec.LookPath("go")
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   "go binary not found in PATH. Install Go to rebuild.",
		}, nil
	}

	outputBin := filepath.Join(projectRoot, "bin", "gorkbot")

	// Ensure bin/ directory exists
	if err := os.MkdirAll(filepath.Dir(outputBin), 0755); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("cannot create bin/ directory: %v", err),
		}, nil
	}

	// Run go build with 5-minute timeout
	buildCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(buildCtx, goBin, "build", "-o", outputBin, "./cmd/gorkbot/")
	cmd.Dir = projectRoot

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	buildErr := cmd.Run()
	buildOutput := combined.String()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Project: %s | Binary: %s]\n\n", projectRoot, outputBin))

	if buildOutput != "" {
		sb.WriteString("=== Build Output ===\n")
		sb.WriteString(buildOutput)
		if !strings.HasSuffix(buildOutput, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	if buildErr != nil || exitCode != 0 {
		sb.WriteString("Build failed. Check errors above.")
		return &ToolResult{
			Success: false,
			Output:  sb.String(),
			Error:   fmt.Sprintf("build exited with code %d", exitCode),
		}, nil
	}

	sb.WriteString("Rebuild successful! Restart Gorkbot to apply changes.")

	// Write restart_pending.json if requested
	if restartAfter {
		if pendingErr := writeRestartPending(message); pendingErr != nil {
			sb.WriteString(fmt.Sprintf("\n\nNote: could not write restart_pending.json: %v", pendingErr))
		} else {
			sb.WriteString("\nrestart_pending.json written — TUI will prompt for restart.")
		}
	}

	return &ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"project_root":  projectRoot,
			"binary_path":   outputBin,
			"restart_after": restartAfter,
			"build_output":  buildOutput,
		},
	}, nil
}

// findProjectRoot locates the Gorkbot project root directory
func findProjectRoot() (string, error) {
	// 1. Explicit env var
	if dir := os.Getenv("GORKBOT_PROJECT_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
	}

	// 2. Walk up from the executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Walk up from cwd
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return "", fmt.Errorf("go.mod not found; set GORKBOT_PROJECT_DIR env var")
}

// writeRestartPending writes the restart pending marker file
func writeRestartPending(message string) error {
	// Determine config dir: use GORKBOT_CONFIG_DIR, XDG_CONFIG_HOME, or ~/.config/gorkbot
	configDir := os.Getenv("GORKBOT_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home dir: %w", err)
		}
		configDir = filepath.Join(home, ".config", "gorkbot")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("cannot create config dir: %w", err)
	}

	pendingPath := filepath.Join(configDir, "restart_pending.json")

	payload := map[string]interface{}{
		"rebuild_at": time.Now().Unix(),
		"message":    message,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal restart payload: %w", err)
	}

	return os.WriteFile(pendingPath, data, 0644)
}
