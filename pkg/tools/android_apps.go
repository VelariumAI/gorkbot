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
			name:               "apk_decompile",
			description:        "Decompile an Android APK file to analyze its resources or source code.",
			category:           CategorySystem,
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
			name:               "sqlite_explorer",
			description:        "Execute SQL queries on an SQLite database file.",
			category:           CategoryDatabase,
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

// SmsSendTool sends SMS messages via Termux:API (simple, user-friendly wrapper).
type SmsSendTool struct {
	BaseTool
}

func NewSmsSendTool() *SmsSendTool {
	return &SmsSendTool{
		BaseTool: BaseTool{
			name:               "sms_send",
			description:        "Send an SMS message via Termux:API. Requires Termux:API app installed with SMS permission enabled.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *SmsSendTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"phone_number": map[string]interface{}{
				"type":        "string",
				"description": "Recipient phone number (e.g., '8707164465')",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "SMS message text (no length limit enforced here; carrier limits apply)",
			},
		},
		"required": []string{"phone_number", "message"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SmsSendTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	phoneNumber, ok := params["phone_number"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "phone_number required"}, nil
	}

	message, ok := params["message"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "message required"}, nil
	}

	// Use termux-sms-send with -n flag
	cmd := exec.CommandContext(ctx, "termux-sms-send", "-n", phoneNumber, message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("SMS send failed: %v\nOutput: %s", err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: fmt.Sprintf("SMS sent to %s", phoneNumber)}, nil
}

// TermuxApiBridgeTool exposes common Termux:API functionality.
type TermuxApiBridgeTool struct {
	BaseTool
}

func NewTermuxApiBridgeTool() *TermuxApiBridgeTool {
	return &TermuxApiBridgeTool{
		BaseTool: BaseTool{
			name:               "termux_api_bridge",
			description:        "Access Android hardware features via Termux:API (camera, sms, contacts, location, vibrate, torch).",
			category:           CategorySystem,
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
		// Special handling for SMS to preserve quoted message text
		// Format: phone_number message
		// or: -n phone_number message
		cmdName = "termux-sms-send"

		// Parse phone number and message carefully
		parts := strings.SplitN(apiArgs, " ", 2)
		if len(parts) < 2 {
			return &ToolResult{Success: false, Error: "SMS requires phone number and message. Format: 'phone_number message' or '-n phone_number message'"}, nil
		}

		phoneNumber := parts[0]
		message := parts[1]

		// Handle -n flag if present
		if phoneNumber == "-n" {
			// Format: -n phone_number message
			msgParts := strings.SplitN(message, " ", 2)
			if len(msgParts) < 2 {
				return &ToolResult{Success: false, Error: "SMS with -n flag requires: '-n phone_number message'"}, nil
			}
			phoneNumber = msgParts[0]
			message = msgParts[1]
			cmdArgs = []string{"-n", phoneNumber, message}
		} else {
			// Direct format without -n flag
			cmdArgs = []string{"-n", phoneNumber, message}
		}
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
		return &ToolResult{Success: false, Error: fmt.Sprintf("Termux API call failed: %v\nOutput: %s", err, string(out))}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}
