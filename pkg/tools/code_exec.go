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

// CodeExecTool executes code in various programming languages
type CodeExecTool struct {
	BaseTool
}

// NewCodeExecTool creates a new code execution tool
func NewCodeExecTool() *CodeExecTool {
	return &CodeExecTool{
		BaseTool: NewBaseTool(
			"execute_code",
			"Execute code in various languages: python3, node, ruby, perl, lua, go (go run), php, bash. Creates a temp file, runs it, captures output.",
			CategoryShell,
			true,
			PermissionSession,
		),
	}
}

// Parameters returns the JSON schema for the code exec tool
func (t *CodeExecTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"description": "Language: python3, node, ruby, perl, lua, go, php, bash",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Source code to execute",
			},
			"timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Execution timeout in seconds (default: 30, max: 120)",
			},
			"stdin_input": map[string]interface{}{
				"type":        "string",
				"description": "Optional stdin input for the program",
			},
		},
		"required": []string{"language", "code"},
	}
	data, _ := json.Marshal(schema)
	return data
}

// langSpec describes how to run a language
type langSpec struct {
	binary    string
	extension string
	buildCmd  func(file string) []string
}

var languageSpecs = map[string]langSpec{
	"python3": {binary: "python3", extension: ".py", buildCmd: func(f string) []string { return []string{"python3", f} }},
	"python":  {binary: "python3", extension: ".py", buildCmd: func(f string) []string { return []string{"python3", f} }},
	"node":    {binary: "node", extension: ".js", buildCmd: func(f string) []string { return []string{"node", f} }},
	"nodejs":  {binary: "node", extension: ".js", buildCmd: func(f string) []string { return []string{"node", f} }},
	"js":      {binary: "node", extension: ".js", buildCmd: func(f string) []string { return []string{"node", f} }},
	"ruby":    {binary: "ruby", extension: ".rb", buildCmd: func(f string) []string { return []string{"ruby", f} }},
	"perl":    {binary: "perl", extension: ".pl", buildCmd: func(f string) []string { return []string{"perl", f} }},
	"lua":     {binary: "lua", extension: ".lua", buildCmd: func(f string) []string { return []string{"lua", f} }},
	"go":      {binary: "go", extension: ".go", buildCmd: func(f string) []string { return []string{"go", "run", f} }},
	"golang":  {binary: "go", extension: ".go", buildCmd: func(f string) []string { return []string{"go", "run", f} }},
	"php":     {binary: "php", extension: ".php", buildCmd: func(f string) []string { return []string{"php", f} }},
	"bash":    {binary: "bash", extension: ".sh", buildCmd: func(f string) []string { return []string{"bash", f} }},
	"sh":      {binary: "bash", extension: ".sh", buildCmd: func(f string) []string { return []string{"bash", f} }},
}

// Execute runs the code in the specified language
func (t *CodeExecTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	language, ok := params["language"].(string)
	if !ok || language == "" {
		return &ToolResult{Success: false, Error: "language parameter must be a string"}, fmt.Errorf("invalid language parameter")
	}
	language = strings.ToLower(strings.TrimSpace(language))

	code, ok := params["code"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "code parameter must be a string"}, fmt.Errorf("invalid code parameter")
	}

	// Parse timeout (default 30, max 120)
	timeoutSecs := 30
	if ts, ok := params["timeout_seconds"].(float64); ok {
		timeoutSecs = int(ts)
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}
	if timeoutSecs > 120 {
		timeoutSecs = 120
	}

	stdinInput := ""
	if si, ok := params["stdin_input"].(string); ok {
		stdinInput = si
	}

	// Look up language spec
	spec, found := languageSpecs[language]
	if !found {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported language '%s'. Supported: python3, node, ruby, perl, lua, go, php, bash", language),
		}, nil
	}

	// Check binary availability
	binaryPath, err := exec.LookPath(spec.binary)
	if err != nil {
		installHint := installHintFor(spec.binary)
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("language '%s' not available: install with %s", language, installHint),
		}, nil
	}
	_ = binaryPath

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gorkbot-exec-*")
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to create temp dir: %v", err)}, nil
	}
	defer os.RemoveAll(tmpDir)

	// Write code to temp file
	filename := "main" + spec.extension
	filePath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to write code file: %v", err)}, nil
	}

	// Build command args
	cmdArgs := spec.buildCmd(filePath)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Set up command
	cmd := exec.CommandContext(cmdCtx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = tmpDir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if stdinInput != "" {
		cmd.Stdin = strings.NewReader(stdinInput)
	}

	// Measure runtime
	startTime := time.Now()
	runErr := cmd.Run()
	runtimeMs := time.Since(startTime).Milliseconds()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	// Build output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Language: %s | Exit: %d | Runtime: %dms]\n\n", language, exitCode, runtimeMs))

	if stdout != "" {
		sb.WriteString("=== STDOUT ===\n")
		sb.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			sb.WriteByte('\n')
		}
	}

	if stderr != "" {
		sb.WriteString("=== STDERR ===\n")
		sb.WriteString(stderr)
		if !strings.HasSuffix(stderr, "\n") {
			sb.WriteByte('\n')
		}
	}

	if stdout == "" && stderr == "" {
		sb.WriteString("(no output)\n")
	}

	success := runErr == nil || exitCode == 0
	result := &ToolResult{
		Success: success,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"stdout":     stdout,
			"stderr":     stderr,
			"exit_code":  exitCode,
			"runtime_ms": runtimeMs,
			"language":   language,
		},
	}

	if !success {
		errMsg := ""
		if runErr != nil {
			errMsg = runErr.Error()
		}
		if stderr != "" {
			errMsg = stderr
		}
		result.Error = errMsg
	}

	return result, nil
}

// installHintFor returns a pkg install hint for a binary
func installHintFor(binary string) string {
	hints := map[string]string{
		"python3": "pkg install python",
		"node":    "pkg install nodejs",
		"ruby":    "pkg install ruby",
		"perl":    "pkg install perl",
		"lua":     "pkg install lua54",
		"go":      "pkg install golang",
		"php":     "pkg install php",
		"bash":    "pkg install bash",
	}
	if hint, ok := hints[binary]; ok {
		return hint
	}
	return fmt.Sprintf("pkg install %s", binary)
}
