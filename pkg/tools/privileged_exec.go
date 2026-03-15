package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

// escalationMethod enumerates the privilege escalation strategies.
type escalationMethod int

const (
	escalationNative   escalationMethod = iota // already root — no wrapper
	escalationSu                               // su -c "command"
	escalationSudo                             // sudo bash -c "command"
	escalationFallback                         // direct exec, no escalation
)

// PrivilegedExecTool routes shell commands through the best available
// privilege escalation mechanism discovered at execution time.
//
// Escalation priority (SENSE Module 2 — EAL):
//   uid==0  → native (already root)
//   su      → su -c
//   sudo    → sudo bash -c
//   default → direct exec + clean error on permission denial
//
// exec.Command is used with separate args throughout — the command string is
// never interpolated into a shell string, so there is no injection surface
// at the Go level. The command is passed verbatim as the -c argument to the
// escalation wrapper's internal shell.
type PrivilegedExecTool struct {
	BaseTool
}

// NewPrivilegedExecTool creates the privileged_execute tool.
func NewPrivilegedExecTool() *PrivilegedExecTool {
	return &PrivilegedExecTool{
		BaseTool: NewBaseTool(
			"privileged_execute",
			"Execute a shell command with automatic privilege escalation. "+
				"Probes for root/su/sudo at runtime and routes accordingly. "+
				"Falls back to direct execution gracefully if no escalation is available. "+
				"Returns a structured JSON envelope with escalation_method, exit_code, success, output, and raw_stderr. "+
				"Use when a command requires elevated privileges (e.g. reading protected /proc paths, "+
				"package install, modifying system config, running logcat on Android).",
			CategorySystem,
			true,
			PermissionOnce,
		),
	}
}

// Parameters returns the JSON schema for privileged_execute.
func (t *PrivilegedExecTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute with elevated privileges",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds (default: 60)",
			},
		},
		"required": []string{"command"},
	}
	data, _ := json.Marshal(schema)
	return data
}

// OutputFormat signals that this tool always returns structured JSON.
func (t *PrivilegedExecTool) OutputFormat() OutputFormat { return FormatJSON }

// privilegedExecResult is the strict response envelope returned to the AI.
type privilegedExecResult struct {
	EscalationMethod string `json:"escalation_method"`
	ExitCode         int    `json:"exit_code"`
	Success          bool   `json:"success"`
	Output           string `json:"output"`
	Stderr           string `json:"raw_stderr,omitempty"`
	Note             string `json:"note,omitempty"`
}

// Execute runs the command through the resolved escalation path.
func (t *PrivilegedExecTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &ToolResult{Success: false, Error: "command parameter required"}, nil
	}

	timeoutSecs := 60.0
	if tv, ok := params["timeout"].(float64); ok && tv > 0 {
		timeoutSecs = tv
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	method, methodName := resolveEscalationMethod(os.Getuid())

	var cmd *exec.Cmd
	switch method {
	case escalationNative:
		cmd = exec.CommandContext(execCtx, "bash", "-c", command)
	case escalationSu:
		// su -c passes command verbatim to the root shell — no extra quoting needed
		// because exec.Command does not go through a shell itself.
		cmd = exec.CommandContext(execCtx, "su", "-c", command)
	case escalationSudo:
		cmd = exec.CommandContext(execCtx, "sudo", "bash", "-c", command)
	default: // escalationFallback
		cmd = exec.CommandContext(execCtx, "bash", "-c", command)
	}

	// Memory-safe output capture: 5 MB stdout, 512 KB stderr.
	// Prevents OOM kills on commands like logcat/dumpsys that can emit 50 MB+.
	var stdout, stderr limitedBuffer
	stdout.limit = 5 * 1024 * 1024
	stderr.limit = 512 * 1024
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// exitCode -1 means the process never started (binary not found, ctx cancelled
	// before exec, etc.). exitCode >= 0 means the shell ran — non-zero exits are
	// not tool failures; the AI reads exit_code to decide what happened.
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	processStarted := cmd.ProcessState != nil

	note := ""
	stderrStr := stderr.String()
	if !processStarted && runErr != nil {
		note = "Process failed to start: " + runErr.Error()
	} else if exitCode != 0 {
		if strings.Contains(stderrStr, "Permission denied") ||
			strings.Contains(stderrStr, "not allowed") ||
			strings.Contains(stderrStr, "password") {
			note = "Escalation may require interactive password or be blocked by policy on this device"
		}
	}

	res := privilegedExecResult{
		EscalationMethod: methodName,
		ExitCode:         exitCode,
		Success:          processStarted && exitCode == 0,
		Output:           stdout.String(),
		Stderr:           stderrStr,
		Note:             note,
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	return &ToolResult{
		Success:      processStarted, // tool itself succeeded if the process ran
		Output:       string(out),
		OutputFormat: FormatJSON,
	}, nil
}

// resolveEscalationMethod selects the best privilege escalation approach for
// the current runtime environment.
func resolveEscalationMethod(uid int) (escalationMethod, string) {
	if uid == 0 {
		return escalationNative, "native (root)"
	}
	if _, err := exec.LookPath("su"); err == nil {
		return escalationSu, "su"
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		return escalationSudo, "sudo"
	}
	return escalationFallback, "direct (no escalation available)"
}
