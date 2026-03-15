package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StructuredBashTool runs a shell command and returns a strictly typed JSON
// envelope with heuristic-parsed output (SENSE Module 4 — Universal Parsing Engine).
//
// Parsing pipeline (applied in order):
//  1. Native JSON   — valid json.Unmarshal → data_type: "json"
//  2. Tabular       — header row + aligned columns (ps, ls -l, df, ip) → data_type: "tabular"
//  3. Key-Value     — KEY=VALUE or "Key: Value" patterns (env, sysctl, /proc) → data_type: "keyvalue"
//  4. Raw fallback  — safely truncated plain text → data_type: "raw"
//
// A hard 5 MB stdout cap prevents OOM kills on commands like dumpsys/journalctl.
type StructuredBashTool struct {
	BaseTool
}

// NewStructuredBashTool creates the structured_bash tool.
func NewStructuredBashTool() *StructuredBashTool {
	return &StructuredBashTool{
		BaseTool: NewBaseTool(
			"structured_bash",
			"Execute a bash command and return a structured JSON result with heuristic-parsed output. "+
				"Automatically detects and parses JSON, tabular (ps/ls/df), and key=value (env/sysctl) formats. "+
				"Enforces a 5 MB output cap to prevent memory issues on verbose commands. "+
				"Use instead of bash when you need to reason about or chain the output programmatically.",
			CategoryShell,
			true,
			PermissionOnce,
		),
	}
}

// Parameters returns the JSON schema for structured_bash.
func (t *StructuredBashTool) Parameters() json.RawMessage {
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

// OutputFormat signals structured JSON output.
func (t *StructuredBashTool) OutputFormat() OutputFormat { return FormatJSON }

// structuredResult is the universal response schema returned for every execution.
type structuredResult struct {
	Command    string      `json:"command"`
	ExitCode   int         `json:"exit_code"`
	Success    bool        `json:"success"`
	DataType   string      `json:"data_type"`   // "json" | "tabular" | "keyvalue" | "raw"
	ParsedData interface{} `json:"parsed_data"` // typed according to DataType
	RawStderr  string      `json:"raw_stderr,omitempty"`
	Truncated  bool        `json:"truncated,omitempty"` // true when output was capped at 5 MB
}

// Execute runs the command and returns a structuredResult JSON envelope.
func (t *StructuredBashTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &ToolResult{Success: false, Error: "command parameter required"}, nil
	}

	timeoutSecs := 30.0
	if tv, ok := params["timeout"].(float64); ok && tv > 0 {
		timeoutSecs = tv
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	if wd, ok := params["workdir"].(string); ok && wd != "" {
		cmd.Dir = expandWorkdir(wd)
	}

	// 5 MB stdout cap — critical for dumpsys/journalctl/logcat class commands.
	var stdout, stderr limitedBuffer
	stdout.limit = 5 * 1024 * 1024
	stderr.limit = 512 * 1024
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	processStarted := cmd.ProcessState != nil

	rawOut := stdout.buf.String()
	dataType, parsedData := parseCommandOutput(rawOut)

	res := structuredResult{
		Command:    command,
		ExitCode:   exitCode,
		Success:    processStarted && exitCode == 0,
		DataType:   dataType,
		ParsedData: parsedData,
		RawStderr:  stderr.String(),
		Truncated:  stdout.truncated,
	}

	out, _ := json.MarshalIndent(res, "", "  ")

	if !processStarted && runErr != nil {
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("process failed to start: %v", runErr),
			Output:       string(out),
			OutputFormat: FormatJSON,
		}, nil
	}

	return &ToolResult{
		Success:      processStarted,
		Output:       string(out),
		OutputFormat: FormatJSON,
	}, nil
}

// ── Universal Parsing Engine ──────────────────────────────────────────────────

// parseCommandOutput applies the four-stage heuristic pipeline and returns
// (dataType, parsedData) for inclusion in the structured response envelope.
func parseCommandOutput(raw string) (string, interface{}) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "raw", trimmed
	}

	// Stage 1: Native JSON
	var jsonVal interface{}
	if json.Unmarshal([]byte(trimmed), &jsonVal) == nil {
		return "json", jsonVal
	}

	// Stage 2: Tabular (ps, ls -l, df, netstat, ip route)
	if tbl, ok := tryParseTabular(trimmed); ok {
		return "tabular", tbl
	}

	// Stage 3: Key-Value (env, sysctl, /proc/meminfo, getprop)
	if kv, ok := tryParseKeyValue(trimmed); ok {
		return "keyvalue", kv
	}

	// Stage 4: Raw fallback
	return "raw", trimmed
}

// tryParseTabular attempts to interpret output as a header + rows table.
// Returns (rows, true) when heuristics confirm tabular structure.
func tryParseTabular(raw string) ([]map[string]string, bool) {
	lines := nonEmptyLines(raw)
	if len(lines) < 2 {
		return nil, false
	}

	// Skip "total N" artifact from ls -l
	startIdx := 0
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "total ") {
		startIdx = 1
	}
	if startIdx >= len(lines)-1 {
		return nil, false
	}

	headers := strings.Fields(lines[startIdx])
	if len(headers) < 2 {
		return nil, false
	}

	var rows []map[string]string
	for _, line := range lines[startIdx+1:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		row := make(map[string]string, len(headers))
		for i, h := range headers {
			if i >= len(fields) {
				break
			}
			if i == len(headers)-1 {
				// Last header absorbs remaining fields (handles COMMAND in ps, etc.)
				row[h] = strings.Join(fields[i:], " ")
			} else {
				row[h] = fields[i]
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, false
	}

	// Require ≥60% of rows to have at least 2 populated columns.
	valid := 0
	for _, row := range rows {
		populated := 0
		for _, v := range row {
			if v != "" {
				populated++
			}
		}
		if populated >= 2 {
			valid++
		}
	}
	if float64(valid)/float64(len(rows)) < 0.60 {
		return nil, false
	}

	return rows, true
}

// tryParseKeyValue attempts to interpret output as KEY=VALUE or "Key: Value" pairs.
// Returns (map, true) when ≥60% of non-empty lines match and at least 2 pairs exist.
func tryParseKeyValue(raw string) (map[string]string, bool) {
	kv := make(map[string]string)
	matched, total := 0, 0

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		total++

		// Pattern 1: KEY=VALUE (env vars, /proc/cmdline tokens, Android getprop)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			if key != "" && !strings.ContainsAny(key, " \t") {
				kv[key] = strings.TrimSpace(line[idx+1:])
				matched++
				continue
			}
		}

		// Pattern 2: "Key: Value" — sysctl, /proc/meminfo, ip/ss output headers
		if idx := strings.Index(line, ": "); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			if key != "" && !strings.ContainsAny(key, "\t") {
				kv[key] = strings.TrimSpace(line[idx+2:])
				matched++
				continue
			}
		}
	}

	if total == 0 || matched < 2 {
		return nil, false
	}
	if float64(matched)/float64(total) < 0.60 {
		return nil, false
	}
	return kv, true
}

// nonEmptyLines splits s on newlines and returns only non-blank lines.
func nonEmptyLines(s string) []string {
	all := strings.Split(s, "\n")
	out := make([]string, 0, len(all))
	for _, l := range all {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
