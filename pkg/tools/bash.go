package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes bash commands
type BashTool struct {
	BaseTool
}

// NewBashTool creates a new bash execution tool
func NewBashTool() *BashTool {
	return &BashTool{
		BaseTool: BaseTool{
			name:        "bash",
			description: "Execute bash commands in the terminal. Returns stdout, stderr, and exit code. Use for running shell commands, scripts, or system operations. Always escape user input using shell escaping to prevent injection.",
			category:    CategoryShell,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

// Parameters returns the JSON schema for bash tool parameters
func (t *BashTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds (default: 30)",
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory (optional)",
			},
		},
		"required": []string{"command"},
	}

	data, _ := json.Marshal(schema)
	return data
}

func (t *BashTool) OutputFormat() OutputFormat {
	return FormatText
}

// Execute runs the bash command
func (t *BashTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	// Extract command
	command, ok := params["command"].(string)
	if !ok {
		return &ToolResult{
			Success:      false,
			Error:        "command parameter must be a string",
			OutputFormat: FormatError,
		}, fmt.Errorf("invalid command parameter")
	}

	// Extract timeout (default 30s)
	timeout := 30 * time.Second
	if t, ok := params["timeout"].(float64); ok {
		timeout = time.Duration(t) * time.Second
	}

	// Extract working directory
	workdir := ""
	if wd, ok := params["workdir"].(string); ok {
		workdir = wd
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Determine exit code. -1 means the process never started (bad command,
	// permission denied, context timeout before exec). Any value ≥ 0 means
	// bash ran the command — including non-zero exits that are legitimate
	// (e.g. grep returning 1 for "no match", test returning 1, diff finding
	// differences). Treat ≥ 0 as a tool success; the AI reads exit_code to
	// decide whether the result is what it expected.
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// A process-level exec error with exit code -1 means the shell never ran.
	var execErr *exec.ExitError
	couldNotRun := err != nil && exitCode == -1

	result := &ToolResult{
		Success:      !couldNotRun,
		Output:       stdout.String(),
		OutputFormat: FormatText,
		Data: map[string]interface{}{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": exitCode,
		},
	}

	if err != nil {
		if couldNotRun {
			// Process failed to launch — surface the exec error.
			result.Error = err.Error()
		} else if !errors.As(err, &execErr) {
			// Unexpected non-ExitError: surface it.
			result.Error = err.Error()
		}
		// ExitError with exit_code >= 0: non-zero exit is not a tool failure.
		// Populate stderr so the AI can inspect the output.
		if stderr.Len() > 0 && result.Error == "" {
			result.Data["stderr"] = stderr.String()
		}
	}

	return result, nil
}

// ReadFileTool reads file contents
type ReadFileTool struct {
	BaseTool
}

// NewReadFileTool creates a new file reading tool
func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{
		BaseTool: BaseTool{
			name:        "read_file",
			description: "Read the complete contents of a file from the filesystem. Returns the raw text content of the file. Use when you need to view file contents.",
			category:    CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionSession,
		},
	}
}

func (t *ReadFileTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ReadFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
			"encoding": map[string]interface{}{
				"type":        "string",
				"description": "File encoding (default: utf-8)",
			},
		},
		"required": []string{"path"},
	}

	data, _ := json.Marshal(schema)
	return data
}

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{
			Success:      false,
			Error:        "path parameter must be a string",
			OutputFormat: FormatError,
		}, fmt.Errorf("invalid path parameter")
	}

	// Use bash tool to read file (leverages existing permissions)
	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("cat %s", shellescape(path)),
	})
}

// WriteFileTool writes content to a file
type WriteFileTool struct {
	BaseTool
}

func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{
		BaseTool: BaseTool{
			name:        "write_file",
			description: "Write content to a file (creates new file or overwrites existing). Use for creating or updating text files. Returns success/failure status.",
			category:    CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *WriteFileTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *WriteFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "Append to file instead of overwriting (default: false)",
			},
		},
		"required": []string{"path", "content"},
	}

	data, _ := json.Marshal(schema)
	return data
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path parameter must be a string", OutputFormat: FormatError}, fmt.Errorf("invalid path")
	}

	content, ok := params["content"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "content parameter must be a string", OutputFormat: FormatError}, fmt.Errorf("invalid content")
	}

	appendMode := false
	if a, ok := params["append"].(bool); ok {
		appendMode = a
	}

	// Capture before content for diff (only for overwrite, not append)
	var beforeContent string
	if !appendMode {
		if data, err := os.ReadFile(path); err == nil {
			beforeContent = string(data)
		}
	}

	operator := ">"
	if appendMode {
		operator = ">>"
	}

	afterContent := content // preserve unescaped content for diff

	// Escape content for bash heredoc
	escaped := strings.ReplaceAll(content, "'", "'\"'\"'")

	command := fmt.Sprintf("cat <<'GROKSTER_EOF' %s %s\n%s\nGROKSTER_EOF", operator, shellescape(path), escaped)

	bashTool := NewBashTool()
	result, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})

	// Attach before/after for diff rendering (overwrite of existing file only)
	if err == nil && result.Success && beforeContent != "" {
		if result.Data == nil {
			result.Data = make(map[string]interface{})
		}
		result.Data["before"] = beforeContent
		result.Data["after"] = afterContent
	}

	return result, err
}

// shellescape escapes a string for safe use in bash commands
func shellescape(s string) string {
	// Simple escaping - wrap in single quotes and escape single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
