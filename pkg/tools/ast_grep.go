package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ASTGrepTool allows structural search and replace using ast-grep (sg).
type ASTGrepTool struct {
	BaseTool
}

// NewASTGrepTool creates an ASTGrepTool.
func NewASTGrepTool() *ASTGrepTool {
	return &ASTGrepTool{
		BaseTool: BaseTool{
			name:               "ast_grep",
			description:        "Search code structurally using ast-grep (sg). Useful for finding exact syntax patterns across files, ignoring whitespace and comments. Provide a pattern (e.g. 'func $NAME($ARGS) { $$$ }') and a language (e.g. 'go', 'python', 'ts').",
			category:           CategoryFile,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *ASTGrepTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The AST pattern to search for, using $VAR for variables and $$$ for multiple statements. Example: 'if err != nil { $$$ }'",
			},
			"language": map[string]interface{}{
				"type":        "string",
				"description": "The programming language (e.g. 'go', 'python', 'rust', 'ts', 'js').",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory or file path to search within (default: current directory).",
			},
			"rewrite": map[string]interface{}{
				"type":        "string",
				"description": "Optional: A pattern to replace the matched AST with. If provided, the files will be modified.",
			},
		},
		"required": []string{"pattern", "language"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ASTGrepTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return &ToolResult{Success: false, Output: "pattern is required", OutputFormat: FormatError}, nil
	}

	lang, ok := params["language"].(string)
	if !ok || lang == "" {
		return &ToolResult{Success: false, Output: "language is required", OutputFormat: FormatError}, nil
	}

	searchPath := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchPath = p
	}

	rewrite, hasRewrite := params["rewrite"].(string)

	// Check if ast-grep (sg) is installed
	if err := exec.Command("sg", "--version").Run(); err != nil {
		return &ToolResult{
			Success:      false,
			Output:       "ast-grep ('sg') is not installed on this system. Please install it to use AST structural search, or use regular 'grep_content' instead.",
			OutputFormat: FormatError,
		}, nil
	}

	args := []string{"run", "--pattern", pattern, "--language", lang}

	if hasRewrite && rewrite != "" {
		args = append(args, "--rewrite", rewrite, "--update-all")
	}

	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "sg", args...)
	out, err := cmd.CombinedOutput()

	if err != nil {
		// 'sg' exits with 1 if no matches are found, or if an error occurs.
		if len(out) == 0 {
			return &ToolResult{
				Success:      true,
				Output:       "No matches found.",
				OutputFormat: FormatText,
			}, nil
		}
		return &ToolResult{
			Success:      false,
			Output:       fmt.Sprintf("ast-grep failed:\n%s\nError: %v", string(out), err),
			OutputFormat: FormatError,
		}, nil
	}

	outputStr := strings.TrimSpace(string(out))
	if outputStr == "" {
		outputStr = "Operation completed successfully, but there was no output."
		if hasRewrite {
			outputStr = "Rewrite applied successfully."
		}
	}

	return &ToolResult{
		Success:      true,
		Output:       outputStr,
		OutputFormat: FormatText,
	}, nil
}
