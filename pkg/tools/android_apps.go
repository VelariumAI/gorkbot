package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ApkDecompileTool decompiles APKs using jadx or apktool (if available).
type ApkDecompileTool struct {
	BaseTool
}

func NewApkDecompileTool() *ApkDecompileTool {
	return &ApkDecompileTool{
		BaseTool: BaseTool{
			name:        "apk_decompile",
			description: "Decompile an Android APK file to analyze its resources or source code.",
			category:          CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ApkDecompileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the APK file.",
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Output directory for decompiled files.",
			},
			"tool": map[string]interface{}{
				"type":        "string",
				"description": "Tool to use: 'apktool' (resources) or 'jadx' (source). Default: jadx.",
				"enum":        []string{"jadx", "apktool"},
			},
		},
		"required": []string{"path", "output_dir"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ApkDecompileTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	apkPath, _ := args["path"].(string)
	outDir, _ := args["output_dir"].(string)
	tool, _ := args["tool"].(string)

	if tool == "" {
		tool = "jadx"
	}

	if apkPath == "" || outDir == "" {
		return &ToolResult{Success: false, Error: "Missing path or output_dir"}, nil
	}

	var cmd *exec.Cmd
	if tool == "apktool" {
		cmd = exec.CommandContext(ctx, "apktool", "d", apkPath, "-o", outDir, "-f")
	} else {
		cmd = exec.CommandContext(ctx, "jadx", "-d", outDir, apkPath)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("%s failed: %v\nOutput: %s", tool, err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: fmt.Sprintf("Decompiled to %s\n%s", outDir, string(out))}, nil
}

// SqliteExplorerTool executes raw SQL queries on a database file.
type SqliteExplorerTool struct {
	BaseTool
}

func NewSqliteExplorerTool() *SqliteExplorerTool {
	return &SqliteExplorerTool{
		BaseTool: BaseTool{
			name:        "sqlite_explorer",
			description: "Execute SQL queries on an SQLite database file.",
			category:          CategoryDatabase,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *SqliteExplorerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"db_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the .sqlite or .db file.",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query to execute (e.g., SELECT * FROM table).",
			},
		},
		"required": []string{"db_path", "query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SqliteExplorerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	dbPath, _ := args["db_path"].(string)
	query, _ := args["query"].(string)

	if dbPath == "" || query == "" {
		return &ToolResult{Success: false, Error: "Missing db_path or query"}, nil
	}

	// Use 'sqlite3' command line tool
	cmd := exec.CommandContext(ctx, "sqlite3", dbPath, query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("SQL query failed: %v\nOutput: %s", err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}

// TermuxApiBridgeTool exposes common Termux:API functionality.
type TermuxApiBridgeTool struct {
	BaseTool
}

func NewTermuxApiBridgeTool() *TermuxApiBridgeTool {
	return &TermuxApiBridgeTool{
		BaseTool: BaseTool{
			name:        "termux_api_bridge",
			description: "Access Android hardware features via Termux:API (camera, sms, contacts, location, vibrate, torch).",
			category:          CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *TermuxApiBridgeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"api": map[string]interface{}{
				"type":        "string",
				"description": "API to call: camera-photo, sms-send, contact-list, location, vibrate, torch, battery-status.",
				"enum":        []string{"camera-photo", "sms-send", "contact-list", "location", "vibrate", "torch", "battery-status"},
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "Arguments for the API call (e.g., phone number for sms, duration for vibrate). Space-separated.",
			},
		},
		"required": []string{"api"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TermuxApiBridgeTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	api, _ := args["api"].(string)
	apiArgs, _ := args["args"].(string)

	var cmdName string
	var cmdArgs []string

	switch api {
	case "camera-photo":
		cmdName = "termux-camera-photo"
		// usage: termux-camera-photo output.jpg
		if apiArgs == "" {
			return &ToolResult{Success: false, Error: "Output filename required for camera-photo"}, nil
		}
		cmdArgs = strings.Fields(apiArgs)
	case "sms-send":
		cmdName = "termux-sms-send"
		// usage: termux-sms-send -n number text
		// Here we assume simple args handling, might need robust parsing.
		cmdArgs = strings.Fields(apiArgs)
	case "contact-list":
		cmdName = "termux-contact-list"
	case "location":
		cmdName = "termux-location"
	case "vibrate":
		cmdName = "termux-vibrate"
		if apiArgs != "" {
			cmdArgs = strings.Fields(apiArgs)
		}
	case "torch":
		cmdName = "termux-torch"
		if apiArgs != "" {
			cmdArgs = strings.Fields(apiArgs)
		}
	case "battery-status":
		cmdName = "termux-battery-status"
	default:
		return &ToolResult{Success: false, Error: "Unknown API: " + api}, nil
	}

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Termux API call failed (is package 'termux-api' installed?): %v\nOutput: %s", err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}
