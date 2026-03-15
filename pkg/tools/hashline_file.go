// Package tools — hashline_file.go
//
// Hashline File Tools — inspired by oh-my-opencode's hashline-edit system.
//
// The "Harness Problem": standard file edit tools fail because the AI must
// reproduce exact file content from memory (whitespace, indentation) to
// identify edit locations. Any mismatch → edit rejected. Hashline solves this
// by tagging every line with a short content-derived hash when the file is
// read, then validating that hash before applying an edit. The AI never needs
// to reproduce content — it just references hash tags.
//
// Usage workflow:
//  1. read_file_hashed → returns lines tagged "N#XXXX| content"
//  2. edit_file_hashed → supply old_hash ("42#AB12") + new_content
//     → system validates hash matches current content before writing
//
// Result: stale-line edit failures drop dramatically.
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/velariumai/gorkbot/pkg/security"
)

// hashlineTag produces a 4-char uppercase hex tag for a line's raw content.
// Uses the first 2 bytes of SHA-256 so tags are short but meaningful.
func hashlineTag(line string) string {
	h := sha256.Sum256([]byte(line))
	return fmt.Sprintf("%02X%02X", h[0], h[1])
}

// ─────────────────────────────────────────────────────────────────────────────
// ReadFileHashedTool
// ─────────────────────────────────────────────────────────────────────────────

// ReadFileHashedTool reads a file and prefixes each line with a content-hash
// tag so the AI can reference lines unambiguously in edit_file_hashed.
type ReadFileHashedTool struct {
	BaseTool
}

// NewReadFileHashedTool creates a ReadFileHashedTool.
func NewReadFileHashedTool() *ReadFileHashedTool {
	return &ReadFileHashedTool{
		BaseTool: BaseTool{
			name:               "read_file_hashed",
			description:        "Read a file with every line tagged by a content-hash (format: 'N#XXXX| content'). Use edit_file_hashed to modify lines by their hash tag, eliminating stale-line edit failures caused by whitespace or content drift.",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ReadFileHashedTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path to read",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "First line to include (1-indexed, default: 1)",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "Last line to include inclusive (default: all lines)",
			},
		},
		"required": []string{"path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ReadFileHashedTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok || strings.TrimSpace(rawPath) == "" {
		return &ToolResult{Success: false, Output: "path is required", OutputFormat: FormatError}, nil
	}

	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Output: fmt.Sprintf("security validation failed: %v", err), OutputFormat: FormatError}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Success: false, Output: fmt.Sprintf("cannot read %s: %v", path, err), OutputFormat: FormatError}, nil
	}

	lines := strings.Split(string(raw), "\n")
	// Trim trailing phantom line if the file ends with '\n'.
	hasTrailingNewline := len(raw) > 0 && raw[len(raw)-1] == '\n'
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	total := len(lines)

	startLine := 1
	if v, ok := params["start_line"].(float64); ok && v >= 1 {
		startLine = int(v)
	}
	endLine := total
	if v, ok := params["end_line"].(float64); ok && v >= 1 {
		endLine = int(v)
	}

	if startLine < 1 {
		startLine = 1
	}
	if endLine > total {
		endLine = total
	}
	if startLine > endLine {
		return &ToolResult{
			Success:      false,
			Output:       fmt.Sprintf("start_line (%d) > end_line (%d)", startLine, endLine),
			OutputFormat: FormatError,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Hashline view of: %s\n", path))
	sb.WriteString(fmt.Sprintf("# Total lines: %d  |  Showing: %d–%d\n", total, startLine, endLine))
	sb.WriteString("# Format: LINE#HASH| content\n")
	sb.WriteString("# To edit: call edit_file_hashed with old_hash=\"LINE#HASH\" and new_content=\"replacement\"\n\n")

	for i := startLine - 1; i < endLine; i++ {
		lineNum := i + 1
		tag := hashlineTag(lines[i])
		sb.WriteString(fmt.Sprintf("%d#%s| %s\n", lineNum, tag, lines[i]))
	}

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// EditFileHashedTool
// ─────────────────────────────────────────────────────────────────────────────

// hashlineEditSpec is one atomic edit operation: identify a line by hash,
// replace its content.
type hashlineEditSpec struct {
	OldHash    string `json:"old_hash"`    // "LINE#XXXX" from read_file_hashed output
	NewContent string `json:"new_content"` // replacement text (without the hash prefix)
}

// EditFileHashedTool applies hash-validated edits to a file.  Only lines whose
// current content matches the provided hash tag will be modified; mismatches
// are reported without corrupting the file.
type EditFileHashedTool struct {
	BaseTool
}

// NewEditFileHashedTool creates an EditFileHashedTool.
func NewEditFileHashedTool() *EditFileHashedTool {
	return &EditFileHashedTool{
		BaseTool: BaseTool{
			name:               "edit_file_hashed",
			description:        "Edit a file by replacing lines identified by their hash tags from read_file_hashed. Provide old_hash (e.g. '42#AB12') and new_content for each line. Edits are rejected if the hash no longer matches the file — prompting you to re-read and get fresh hashes. Multiple edits are applied in a single atomic write.",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *EditFileHashedTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path of the file to edit",
			},
			"edits": map[string]interface{}{
				"type":        "array",
				"description": "Ordered list of edits to apply",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"old_hash": map[string]interface{}{
							"type":        "string",
							"description": "Hash tag from read_file_hashed, e.g. '42#AB12'. The number prefix is the line number hint; XXXX is the 4-char hash.",
						},
						"new_content": map[string]interface{}{
							"type":        "string",
							"description": "New content for this line (without hash prefix). Use empty string to delete the line.",
						},
					},
					"required": []string{"old_hash", "new_content"},
				},
			},
		},
		"required": []string{"path", "edits"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *EditFileHashedTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok || strings.TrimSpace(rawPath) == "" {
		return &ToolResult{Success: false, Output: "path is required", OutputFormat: FormatError}, nil
	}

	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Output: fmt.Sprintf("security validation failed: %v", err), OutputFormat: FormatError}, nil
	}

	editsRaw, ok := params["edits"].([]interface{})
	if !ok || len(editsRaw) == 0 {
		return &ToolResult{Success: false, Output: "edits array is required and must not be empty", OutputFormat: FormatError}, nil
	}

	// Parse edits.
	var edits []hashlineEditSpec
	for idx, e := range editsRaw {
		eMap, ok := e.(map[string]interface{})
		if !ok {
			return &ToolResult{
				Success:      false,
				Output:       fmt.Sprintf("edits[%d] is not an object", idx),
				OutputFormat: FormatError,
			}, nil
		}
		spec := hashlineEditSpec{}
		if v, ok := eMap["old_hash"].(string); ok {
			spec.OldHash = strings.TrimSpace(v)
		}
		if v, ok := eMap["new_content"].(string); ok {
			spec.NewContent = v
		}
		if spec.OldHash == "" {
			return &ToolResult{
				Success:      false,
				Output:       fmt.Sprintf("edits[%d].old_hash is required", idx),
				OutputFormat: FormatError,
			}, nil
		}
		edits = append(edits, spec)
	}

	// Read current file.
	raw, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Success: false, Output: fmt.Sprintf("cannot read %s: %v", path, err), OutputFormat: FormatError}, nil
	}

	lines := strings.Split(string(raw), "\n")
	hasTrailingNewline := len(raw) > 0 && raw[len(raw)-1] == '\n'
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	type editOutcome struct {
		desc string
		ok   bool
	}
	outcomes := make([]editOutcome, 0, len(edits))

	applied := 0
	rejected := 0

	for _, edit := range edits {
		// Parse "N#XXXX" → lineNum hint + hashPart.
		parts := strings.SplitN(edit.OldHash, "#", 2)
		var lineNumHint int
		var hashPart string

		if len(parts) == 2 {
			lineNumHint, _ = strconv.Atoi(parts[0])
			hashPart = strings.ToUpper(strings.TrimSpace(parts[1]))
		} else {
			// No '#' — treat whole thing as hash only.
			hashPart = strings.ToUpper(strings.TrimSpace(parts[0]))
		}

		if hashPart == "" {
			outcomes = append(outcomes, editOutcome{
				desc: fmt.Sprintf("✗ bad old_hash format: %q", edit.OldHash),
				ok:   false,
			})
			rejected++
			continue
		}

		matched := false

		// 1. Try exact line-number hint first (fast path).
		if lineNumHint >= 1 && lineNumHint <= len(lines) {
			idx := lineNumHint - 1
			currentHash := hashlineTag(lines[idx])
			if currentHash == hashPart {
				if edit.NewContent == "" {
					// Delete line: mark for removal.
					lines[idx] = "\x00DELETE\x00"
				} else {
					lines[idx] = edit.NewContent
				}
				outcomes = append(outcomes, editOutcome{
					desc: fmt.Sprintf("✓ Line %d updated", lineNumHint),
					ok:   true,
				})
				applied++
				matched = true
			} else {
				// Hash mismatch at hint line — try full scan before rejecting.
				for i, line := range lines {
					if hashlineTag(line) == hashPart {
						if edit.NewContent == "" {
							lines[i] = "\x00DELETE\x00"
						} else {
							lines[i] = edit.NewContent
						}
						outcomes = append(outcomes, editOutcome{
							desc: fmt.Sprintf("✓ Line %d updated (hash scan; hint was line %d)", i+1, lineNumHint),
							ok:   true,
						})
						applied++
						matched = true
						break
					}
				}
				if !matched {
					outcomes = append(outcomes, editOutcome{
						desc: fmt.Sprintf("✗ Line %d hash mismatch: want %s got %s — re-read the file to get fresh hashes",
							lineNumHint, hashPart, currentHash),
						ok: false,
					})
					rejected++
				}
			}
		} else {
			// No line number hint — full scan.
			for i, line := range lines {
				if hashlineTag(line) == hashPart {
					if edit.NewContent == "" {
						lines[i] = "\x00DELETE\x00"
					} else {
						lines[i] = edit.NewContent
					}
					outcomes = append(outcomes, editOutcome{
						desc: fmt.Sprintf("✓ Line %d updated (hash-only match)", i+1),
						ok:   true,
					})
					applied++
					matched = true
					break
				}
			}
			if !matched {
				outcomes = append(outcomes, editOutcome{
					desc: fmt.Sprintf("✗ Hash %s not found anywhere in file", hashPart),
					ok:   false,
				})
				rejected++
			}
		}
	}

	// If all edits were rejected, do NOT write the file.
	if applied == 0 {
		var sb strings.Builder
		sb.WriteString("All edits rejected — file unchanged:\n\n")
		for _, o := range outcomes {
			sb.WriteString("  " + o.desc + "\n")
		}
		sb.WriteString("\nRe-read the file with read_file_hashed to obtain current hash tags.")
		return &ToolResult{Success: false, Output: sb.String(), OutputFormat: FormatError}, nil
	}

	// Remove deleted lines.
	kept := lines[:0]
	for _, l := range lines {
		if l != "\x00DELETE\x00" {
			kept = append(kept, l)
		}
	}
	lines = kept

	// Reconstruct file content.
	output := strings.Join(lines, "\n")
	if hasTrailingNewline {
		output += "\n"
	}

	if err := os.WriteFile(path, []byte(output), 0644); err != nil {
		return &ToolResult{
			Success:      false,
			Output:       fmt.Sprintf("write error: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	// Build summary report.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Applied %d/%d edits to %s\n\n", applied, len(edits), path))
	for _, o := range outcomes {
		sb.WriteString("  " + o.desc + "\n")
	}
	if rejected > 0 {
		sb.WriteString(fmt.Sprintf("\n%d edit(s) rejected. Call read_file_hashed again to refresh hash tags.", rejected))
	}

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}
