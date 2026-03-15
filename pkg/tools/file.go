package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/security"
)

// ListDirectoryTool lists directory contents
type ListDirectoryTool struct {
	BaseTool
}

func NewListDirectoryTool() *ListDirectoryTool {
	return &ListDirectoryTool{
		BaseTool: BaseTool{
			name:               "list_directory",
			description:        "List contents of a directory with details (size, permissions, modification time)",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ListDirectoryTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to list (default: current directory)",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "List recursively (default: false)",
			},
			"hidden": map[string]interface{}{
				"type":        "boolean",
				"description": "Include hidden files (default: true)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListDirectoryTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath := "."
	if p, ok := params["path"].(string); ok {
		rawPath = p
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	recursive := false
	if r, ok := params["recursive"].(bool); ok {
		recursive = r
	}

	hidden := true
	if h, ok := params["hidden"].(bool); ok {
		hidden = h
	}

	flags := "-lh"
	if hidden {
		flags += "a"
	}
	if recursive {
		flags += "R"
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("ls %s %s", flags, shellescape(path)),
	})
}

// SearchFilesTool searches for files by name/pattern
type SearchFilesTool struct {
	BaseTool
}

func NewSearchFilesTool() *SearchFilesTool {
	return &SearchFilesTool{
		BaseTool: BaseTool{
			name:               "search_files",
			description:        "Search for files by name pattern using find command",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SearchFilesTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "File name pattern (supports wildcards like *.txt)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search in (default: current directory)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "File type: 'f' for files, 'd' for directories (default: f)",
			},
		},
		"required": []string{"pattern"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SearchFilesTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "pattern is required"}, fmt.Errorf("pattern required")
	}

	rawPath := "."
	if p, ok := params["path"].(string); ok {
		rawPath = p
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	fileType := "f"
	if t, ok := params["type"].(string); ok {
		fileType = t
	}

	command := fmt.Sprintf("find %s -type %s -name %s 2>/dev/null",
		shellescape(path), fileType, shellescape(pattern))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// GrepContentTool searches file contents
type GrepContentTool struct {
	BaseTool
}

func NewGrepContentTool() *GrepContentTool {
	return &GrepContentTool{
		BaseTool: BaseTool{
			name:               "grep_content",
			description:        "Search for text patterns in files using grep",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *GrepContentTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Text pattern to search for (supports regex)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File or directory to search in",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Search recursively (default: false)",
			},
			"ignore_case": map[string]interface{}{
				"type":        "boolean",
				"description": "Case-insensitive search (default: false)",
			},
			"line_numbers": map[string]interface{}{
				"type":        "boolean",
				"description": "Show line numbers (default: true)",
			},
		},
		"required": []string{"pattern", "path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *GrepContentTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "pattern is required"}, fmt.Errorf("pattern required")
	}

	rawPath, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	flags := ""
	if recursive, ok := params["recursive"].(bool); ok && recursive {
		flags += "r"
	}
	if ignoreCase, ok := params["ignore_case"].(bool); ok && ignoreCase {
		flags += "i"
	}

	lineNumbers := true
	if ln, ok := params["line_numbers"].(bool); ok {
		lineNumbers = ln
	}
	if lineNumbers {
		flags += "n"
	}

	if flags != "" {
		flags = "-" + flags
	}

	command := fmt.Sprintf("grep %s %s %s 2>/dev/null || echo 'No matches found'",
		flags, shellescape(pattern), shellescape(path))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// FileInfoTool gets file metadata
type FileInfoTool struct {
	BaseTool
}

func NewFileInfoTool() *FileInfoTool {
	return &FileInfoTool{
		BaseTool: BaseTool{
			name:               "file_info",
			description:        "Get detailed information about a file (size, permissions, timestamps, type)",
			category:           CategoryFile,
			requiresPermission: false, // Safe read-only operation
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *FileInfoTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file or directory",
			},
		},
		"required": []string{"path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *FileInfoTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	command := fmt.Sprintf("stat -c 'Size: %%s bytes\nPermissions: %%A (%%a)\nOwner: %%U (%%u)\nGroup: %%G (%%g)\nModified: %%y\nAccessed: %%x\nCreated: %%w\nType: %%F' %s 2>/dev/null || stat -f 'Size: %%z bytes\nPermissions: %%Sp (%%p)\nOwner: %%Su (%%u)\nGroup: %%Sg (%%g)\nModified: %%Sm\nAccessed: %%Sa\nType: %%HT' %s",
		shellescape(path), shellescape(path))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// DeleteFileTool deletes files or directories
type DeleteFileTool struct {
	BaseTool
}

func NewDeleteFileTool() *DeleteFileTool {
	return &DeleteFileTool{
		BaseTool: BaseTool{
			name:               "delete_file",
			description:        "Delete a file or directory (use with caution!)",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionOnce, // Always ask!
		},
	}
}

func (t *DeleteFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to file or directory to delete",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Delete directory recursively (default: false)",
			},
		},
		"required": []string{"path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DeleteFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	recursive := false
	if r, ok := params["recursive"].(bool); ok {
		recursive = r
	}

	flags := ""
	if recursive {
		flags = "-rf"
	} else {
		flags = "-f"
	}

	command := fmt.Sprintf("rm %s %s", flags, shellescape(path))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// EditFileTool makes precise edits to existing files
type EditFileTool struct {
	BaseTool
}

func NewEditFileTool() *EditFileTool {
	return &EditFileTool{
		BaseTool: BaseTool{
			name:               "edit_file",
			description:        "Make precise edits to existing files by replacing old_string with new_string",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *EditFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The exact string to find and replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The string to replace it with",
			},
			"replace_all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default: false, only first match)",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *EditFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	oldStr, ok := params["old_string"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "old_string is required"}, fmt.Errorf("old_string required")
	}

	newStr, ok := params["new_string"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "new_string is required"}, fmt.Errorf("new_string required")
	}

	replaceAll := false
	if r, ok := params["replace_all"].(bool); ok {
		replaceAll = r
	}

	// Read file first
	bashTool := NewBashTool()
	readResult, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("cat %s", shellescape(path)),
	})
	if err != nil || !readResult.Success {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read file: %v", err),
		}, err
	}

	content := readResult.Output

	// Perform replacement
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		newContent = strings.Replace(content, oldStr, newStr, 1)
	}

	// Check if anything changed
	if newContent == content {
		return &ToolResult{
			Success: false,
			Error:   "old_string not found in file",
		}, fmt.Errorf("old_string not found")
	}

	// Write back
	escapedContent := strings.ReplaceAll(newContent, "'", "'\"'\"'")
	command := fmt.Sprintf("cat <<'GROKSTER_EOF' > %s\n%s\nGROKSTER_EOF", shellescape(path), escapedContent)

	result, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})

	if err == nil && result.Success {
		result.Output = "File edited successfully"
		if result.Data == nil {
			result.Data = make(map[string]interface{})
		}
		result.Data["before"] = content
		result.Data["after"] = newContent
	}

	return result, err
}

// MultiEditFileTool performs multiple edits in a single operation
type MultiEditFileTool struct {
	BaseTool
}

func NewMultiEditFileTool() *MultiEditFileTool {
	return &MultiEditFileTool{
		BaseTool: BaseTool{
			name:               "multi_edit_file",
			description:        "Make multiple precise edits to a file in a single operation",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *MultiEditFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"edits": map[string]interface{}{
				"type":        "array",
				"description": "Array of edit operations to perform sequentially",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"old_string": map[string]interface{}{
							"type":        "string",
							"description": "The exact string to find and replace",
						},
						"new_string": map[string]interface{}{
							"type":        "string",
							"description": "The string to replace it with",
						},
					},
					"required": []string{"old_string", "new_string"},
				},
			},
		},
		"required": []string{"path", "edits"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *MultiEditFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawPath, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}
	path, err := security.ValidatePath(rawPath)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("security validation failed: %v", err)}, err
	}

	editsRaw, ok := params["edits"].([]interface{})
	if !ok {
		return &ToolResult{Success: false, Error: "edits must be an array"}, fmt.Errorf("edits required")
	}

	// Read file first
	bashTool := NewBashTool()
	readResult, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("cat %s", shellescape(path)),
	})
	if err != nil || !readResult.Success {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read file: %v", err),
		}, err
	}

	content := readResult.Output

	// Perform all edits sequentially
	editCount := 0
	for i, editRaw := range editsRaw {
		editMap, ok := editRaw.(map[string]interface{})
		if !ok {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("edit %d is not a valid object", i),
			}, fmt.Errorf("invalid edit")
		}

		oldStr, ok := editMap["old_string"].(string)
		if !ok {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("edit %d missing old_string", i),
			}, fmt.Errorf("missing old_string")
		}

		newStr, ok := editMap["new_string"].(string)
		if !ok {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("edit %d missing new_string", i),
			}, fmt.Errorf("missing new_string")
		}

		// Check if old_string exists
		if !strings.Contains(content, oldStr) {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("edit %d: old_string not found in file", i),
			}, fmt.Errorf("old_string not found")
		}

		// Perform replacement (only first occurrence)
		content = strings.Replace(content, oldStr, newStr, 1)
		editCount++
	}

	// Write back
	escapedContent := strings.ReplaceAll(content, "'", "'\"'\"'")
	command := fmt.Sprintf("cat <<'GROKSTER_EOF' > %s\n%s\nGROKSTER_EOF", shellescape(path), escapedContent)

	result, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})

	if err == nil && result.Success {
		result.Output = fmt.Sprintf("File edited successfully with %d changes", editCount)
	}

	return result, err
}
