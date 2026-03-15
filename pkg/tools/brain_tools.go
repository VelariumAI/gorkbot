package tools

// brain_tools.go — Gorkbot Brain File Tools
//
// Provides four tools for persistent self-knowledge management across sessions:
//
//   record_fact      — write a non-obvious fact/quirk/solution to MEMORY.md
//   record_user_pref — write a user preference/workflow pattern to USER.md
//   read_brain       — read a brain file (MEMORY.md, USER.md, SOUL.md, etc.)
//   forget_fact      — remove entries from MEMORY.md or USER.md by substring
//
// Brain files live in ~/.gorkbot/brain/ and persist across sessions.
// Each entry is delimited with § to allow reliable count and removal.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// brainDir returns the brain directory path: ~/.gorkbot/brain
func brainDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gorkbot", "brain"), nil
}

// ensureBrainFile ensures the brain directory and the given file exist.
// Returns the full path to the file.
func ensureBrainFile(filename string) (string, error) {
	dir, err := brainDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("cannot create brain dir: %w", err)
	}
	fpath := filepath.Join(dir, filename)
	if _, err := os.Stat(fpath); os.IsNotExist(err) {
		if err := os.WriteFile(fpath, []byte{}, 0644); err != nil {
			return "", fmt.Errorf("cannot create %s: %w", filename, err)
		}
	}
	return fpath, nil
}

// countEntries counts the number of § delimiters in content.
func countEntries(content string) int {
	return strings.Count(content, "§")
}

// ── record_fact ──────────────────────────────────────────────────────────────

// RecordFactTool records a fact/quirk/solution to MEMORY.md.
type RecordFactTool struct {
	BaseTool
}

// NewRecordFactTool creates the record_fact tool.
func NewRecordFactTool() *RecordFactTool {
	return &RecordFactTool{
		BaseTool: NewBaseTool(
			"record_fact",
			"Record a non-obvious fact, tool quirk, or solution to MEMORY.md brain file for future sessions",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *RecordFactTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {
				"type": "string",
				"description": "The fact, quirk, or solution to record (max 3000 chars)",
				"maxLength": 3000
			},
			"tags": {
				"type": "string",
				"description": "Optional comma-separated tags for the entry"
			}
		},
		"required": ["content"]
	}`)
}

func (t *RecordFactTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	content, ok := params["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return &ToolResult{Success: false, Error: "content parameter must be a non-empty string", OutputFormat: FormatError}, nil
	}
	if len(content) > 3000 {
		return &ToolResult{Success: false, Error: "content exceeds 3000 character limit", OutputFormat: FormatError}, nil
	}

	fpath, err := ensureBrainFile("MEMORY.md")
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error(), OutputFormat: FormatError}, nil
	}

	existing, err := os.ReadFile(fpath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot read MEMORY.md: %v", err), OutputFormat: FormatError}, nil
	}

	count := countEntries(string(existing))
	if count >= 50 {
		return &ToolResult{
			Success:      false,
			Error:        "MEMORY.md is full (50 entries max), use forget_fact to remove old entries first",
			OutputFormat: FormatError,
		}, nil
	}

	date := time.Now().Format("2006-01-02")
	entry := fmt.Sprintf("\n§ [%s] %s\n", date, content)

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot open MEMORY.md for append: %v", err), OutputFormat: FormatError}, nil
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot write to MEMORY.md: %v", err), OutputFormat: FormatError}, nil
	}

	newCount := count + 1
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Fact recorded to MEMORY.md (entry %d of 50)", newCount),
		OutputFormat: FormatText,
	}, nil
}

func (t *RecordFactTool) OutputFormat() OutputFormat { return FormatText }

// ── record_user_pref ─────────────────────────────────────────────────────────

// RecordUserPrefTool records a user preference to USER.md.
type RecordUserPrefTool struct {
	BaseTool
}

// NewRecordUserPrefTool creates the record_user_pref tool.
func NewRecordUserPrefTool() *RecordUserPrefTool {
	return &RecordUserPrefTool{
		BaseTool: NewBaseTool(
			"record_user_pref",
			"Record a user preference, workflow pattern, or behavioral preference to USER.md brain file",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *RecordUserPrefTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"preference": {
				"type": "string",
				"description": "The user preference, workflow pattern, or behavioral note to record (max 2000 chars)",
				"maxLength": 2000
			}
		},
		"required": ["preference"]
	}`)
}

func (t *RecordUserPrefTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	preference, ok := params["preference"].(string)
	if !ok || strings.TrimSpace(preference) == "" {
		return &ToolResult{Success: false, Error: "preference parameter must be a non-empty string", OutputFormat: FormatError}, nil
	}
	if len(preference) > 2000 {
		return &ToolResult{Success: false, Error: "preference exceeds 2000 character limit", OutputFormat: FormatError}, nil
	}

	fpath, err := ensureBrainFile("USER.md")
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error(), OutputFormat: FormatError}, nil
	}

	existing, err := os.ReadFile(fpath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot read USER.md: %v", err), OutputFormat: FormatError}, nil
	}

	count := countEntries(string(existing))
	if count >= 30 {
		return &ToolResult{
			Success:      false,
			Error:        "USER.md is full (30 entries max), use forget_fact to remove old entries first",
			OutputFormat: FormatError,
		}, nil
	}

	date := time.Now().Format("2006-01-02")
	entry := fmt.Sprintf("\n§ [%s] %s\n", date, preference)

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot open USER.md for append: %v", err), OutputFormat: FormatError}, nil
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot write to USER.md: %v", err), OutputFormat: FormatError}, nil
	}

	newCount := count + 1
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("User preference recorded to USER.md (entry %d of 30)", newCount),
		OutputFormat: FormatText,
	}, nil
}

func (t *RecordUserPrefTool) OutputFormat() OutputFormat { return FormatText }

// ── read_brain ───────────────────────────────────────────────────────────────

// ReadBrainTool reads a brain file.
type ReadBrainTool struct {
	BaseTool
}

// NewReadBrainTool creates the read_brain tool.
func NewReadBrainTool() *ReadBrainTool {
	return &ReadBrainTool{
		BaseTool: NewBaseTool(
			"read_brain",
			"Read current content of a brain file (MEMORY.md, USER.md, SOUL.md, IDENTITY.md, etc.)",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *ReadBrainTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file": {
				"type": "string",
				"description": "Brain file name only (no path) — e.g. MEMORY.md, USER.md, SOUL.md"
			}
		},
		"required": ["file"]
	}`)
}

func (t *ReadBrainTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	filename, ok := params["file"].(string)
	if !ok || strings.TrimSpace(filename) == "" {
		return &ToolResult{Success: false, Error: "file parameter must be a non-empty string", OutputFormat: FormatError}, nil
	}

	// Security: reject path traversal or directory separator characters.
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		return &ToolResult{
			Success:      false,
			Error:        "Invalid brain file name",
			OutputFormat: FormatError,
		}, nil
	}

	dir, err := brainDir()
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error(), OutputFormat: FormatError}, nil
	}

	fpath := filepath.Join(dir, filename)
	data, err := os.ReadFile(fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ToolResult{
				Success:      false,
				Error:        fmt.Sprintf("brain file %q does not exist", filename),
				OutputFormat: FormatError,
			}, nil
		}
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot read %s: %v", filename, err), OutputFormat: FormatError}, nil
	}

	content := string(data)
	entryCount := countEntries(content)
	output := content + fmt.Sprintf("\n\n[Entry count: %d]", entryCount)

	return &ToolResult{
		Success:      true,
		Output:       output,
		OutputFormat: FormatText,
	}, nil
}

func (t *ReadBrainTool) OutputFormat() OutputFormat { return FormatText }

// ── forget_fact ──────────────────────────────────────────────────────────────

// ForgetFactTool removes matching entries from MEMORY.md or USER.md.
type ForgetFactTool struct {
	BaseTool
}

// NewForgetFactTool creates the forget_fact tool.
func NewForgetFactTool() *ForgetFactTool {
	return &ForgetFactTool{
		BaseTool: NewBaseTool(
			"forget_fact",
			"Remove an entry from MEMORY.md or USER.md by matching a substring",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *ForgetFactTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file": {
				"type": "string",
				"description": "Brain file to modify: MEMORY.md or USER.md",
				"enum": ["MEMORY.md", "USER.md"]
			},
			"substring": {
				"type": "string",
				"description": "Text to match (case-insensitive) — entries containing this text will be removed"
			}
		},
		"required": ["file", "substring"]
	}`)
}

func (t *ForgetFactTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	filename, ok := params["file"].(string)
	if !ok || (filename != "MEMORY.md" && filename != "USER.md") {
		return &ToolResult{Success: false, Error: "file must be MEMORY.md or USER.md", OutputFormat: FormatError}, nil
	}

	substring, ok := params["substring"].(string)
	if !ok || strings.TrimSpace(substring) == "" {
		return &ToolResult{Success: false, Error: "substring parameter must be a non-empty string", OutputFormat: FormatError}, nil
	}

	dir, err := brainDir()
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error(), OutputFormat: FormatError}, nil
	}

	fpath := filepath.Join(dir, filename)
	data, err := os.ReadFile(fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ToolResult{
				Success:      true,
				Output:       fmt.Sprintf("Removed 0 matching entries from %s (file does not exist)", filename),
				OutputFormat: FormatText,
			}, nil
		}
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot read %s: %v", filename, err), OutputFormat: FormatError}, nil
	}

	content := string(data)
	lowerSubstring := strings.ToLower(substring)

	// Split on § delimiter. The first element is the preamble (before any §).
	// Each subsequent element starts at § and contains the entry text.
	parts := strings.Split(content, "§")
	preamble := parts[0]
	entries := parts[1:]

	var kept []string
	removed := 0
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry), lowerSubstring) {
			removed++
		} else {
			kept = append(kept, entry)
		}
	}

	// Reconstruct file: preamble + rejoined kept entries each prefixed with §
	var sb strings.Builder
	sb.WriteString(preamble)
	for _, entry := range kept {
		sb.WriteString("§")
		sb.WriteString(entry)
	}

	newContent := sb.String()
	if err := os.WriteFile(fpath, []byte(newContent), 0644); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("cannot write %s: %v", filename, err), OutputFormat: FormatError}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Removed %d matching entries from %s", removed, filename),
		OutputFormat: FormatText,
	}, nil
}

func (t *ForgetFactTool) OutputFormat() OutputFormat { return FormatText }

// ── Registration ─────────────────────────────────────────────────────────────

// RegisterBrainTools registers all four brain tools into the given registry.
func RegisterBrainTools(reg *Registry) {
	_ = reg.Register(NewRecordFactTool())
	_ = reg.Register(NewRecordUserPrefTool())
	_ = reg.Register(NewReadBrainTool())
	_ = reg.Register(NewForgetFactTool())
}
