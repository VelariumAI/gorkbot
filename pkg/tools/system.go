package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListProcessesTool lists running processes
type ListProcessesTool struct {
	BaseTool
}

func NewListProcessesTool() *ListProcessesTool {
	return &ListProcessesTool{
		BaseTool: BaseTool{
			name:              "list_processes",
			description:       "List running processes with details (PID, CPU, memory, command)",
			category:          CategorySystem,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *ListProcessesTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Filter processes by name (optional)",
			},
			"sort_by": map[string]interface{}{
				"type":        "string",
				"description": "Sort by: cpu, memory, pid (default: cpu)",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Limit number of results (default: 20)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListProcessesTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	sortBy := "cpu"
	if s, ok := params["sort_by"].(string); ok {
		sortBy = s
	}

	limit := 20
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	// Build ps command
	sortFlag := "--sort=-%cpu" // Default to CPU
	switch sortBy {
	case "memory", "mem":
		sortFlag = "--sort=-%mem"
	case "pid":
		sortFlag = "--sort=pid"
	}

	command := fmt.Sprintf("ps aux %s | head -n %d", sortFlag, limit+1)

	// Add filter if provided
	if filter, ok := params["filter"].(string); ok {
		command = fmt.Sprintf("ps aux %s | grep %s | grep -v grep | head -n %d",
			sortFlag, shellescape(filter), limit)
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// KillProcessTool terminates a process
type KillProcessTool struct {
	BaseTool
}

func NewKillProcessTool() *KillProcessTool {
	return &KillProcessTool{
		BaseTool: BaseTool{
			name:              "kill_process",
			description:       "Terminate a process by PID or name",
			category:          CategorySystem,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *KillProcessTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pid": map[string]interface{}{
				"type":        "number",
				"description": "Process ID to kill",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Process name to kill (kills all matching)",
			},
			"signal": map[string]interface{}{
				"type":        "string",
				"description": "Signal to send (default: TERM, options: TERM, KILL, INT)",
			},
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "Force kill with SIGKILL (default: false)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *KillProcessTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	signal := "TERM"
	if s, ok := params["signal"].(string); ok {
		signal = s
	}

	if f, ok := params["force"].(bool); ok && f {
		signal = "KILL"
	}

	var command string

	// Kill by PID
	if pid, ok := params["pid"].(float64); ok {
		command = fmt.Sprintf("kill -%s %d", signal, int(pid))
	} else if name, ok := params["name"].(string); ok {
		// Kill by name using pkill
		command = fmt.Sprintf("pkill -%s %s", signal, shellescape(name))
	} else {
		return &ToolResult{Success: false, Error: "either pid or name is required"}, fmt.Errorf("pid or name required")
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// EnvVarTool manages environment variables
type EnvVarTool struct {
	BaseTool
}

func NewEnvVarTool() *EnvVarTool {
	return &EnvVarTool{
		BaseTool: BaseTool{
			name:              "env_var",
			description:       "Get or set environment variables",
			category:          CategorySystem,
			requiresPermission: true,
			defaultPermission: PermissionSession,
		},
	}
}

func (t *EnvVarTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: get, set, list, unset",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Variable name",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "Variable value (for set action)",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *EnvVarTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	var command string

	switch action {
	case "get":
		name, ok := params["name"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "name is required for get action"}, fmt.Errorf("name required")
		}
		command = fmt.Sprintf("echo $%s", name)

	case "set":
		name, ok := params["name"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "name is required for set action"}, fmt.Errorf("name required")
		}
		value, ok := params["value"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "value is required for set action"}, fmt.Errorf("value required")
		}
		command = fmt.Sprintf("export %s=%s && echo 'Set %s=%s'", name, shellescape(value), name, shellescape(value))

	case "list":
		filter := ""
		if name, ok := params["name"].(string); ok {
			filter = fmt.Sprintf(" | grep %s", shellescape(name))
		}
		command = fmt.Sprintf("env | sort%s", filter)

	case "unset":
		name, ok := params["name"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "name is required for unset action"}, fmt.Errorf("name required")
		}
		command = fmt.Sprintf("unset %s && echo 'Unset %s'", name, name)

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action: %s", action)
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// SystemInfoTool gets system information
type SystemInfoTool struct {
	BaseTool
}

func NewSystemInfoTool() *SystemInfoTool {
	return &SystemInfoTool{
		BaseTool: BaseTool{
			name:              "system_info",
			description:       "Get system information (OS, CPU, memory, disk, uptime)",
			category:          CategorySystem,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *SystemInfoTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"detail": map[string]interface{}{
				"type":        "string",
				"description": "Type of info: all, os, cpu, memory, disk, uptime (default: all)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SystemInfoTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	detail := "all"
	if d, ok := params["detail"].(string); ok {
		detail = d
	}

	var command string

	switch detail {
	case "os":
		command = "uname -a"
	case "cpu":
		command = "cat /proc/cpuinfo | grep 'model name' | head -n 1 && cat /proc/cpuinfo | grep processor | wc -l | awk '{print \"CPU Cores: \" $1}'"
	case "memory":
		command = "free -h"
	case "disk":
		command = "df -h"
	case "uptime":
		command = "uptime"
	case "all":
		command = `echo "=== OS ===" && uname -a && echo "" && \
echo "=== CPU ===" && cat /proc/cpuinfo 2>/dev/null | grep "model name" | head -n 1 || sysctl -n machdep.cpu.brand_string 2>/dev/null && \
cat /proc/cpuinfo 2>/dev/null | grep processor | wc -l | awk '{print "CPU Cores: " $1}' || sysctl -n hw.ncpu 2>/dev/null | awk '{print "CPU Cores: " $1}' && echo "" && \
echo "=== Memory ===" && free -h 2>/dev/null || vm_stat 2>/dev/null && echo "" && \
echo "=== Disk ===" && df -h && echo "" && \
echo "=== Uptime ===" && uptime`
	default:
		return &ToolResult{Success: false, Error: "invalid detail type"}, fmt.Errorf("invalid detail: %s", detail)
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// DiskUsageTool analyzes disk usage
type DiskUsageTool struct {
	BaseTool
}

func NewDiskUsageTool() *DiskUsageTool {
	return &DiskUsageTool{
		BaseTool: BaseTool{
			name:              "disk_usage",
			description:       "Analyze disk usage of directories and files",
			category:          CategorySystem,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *DiskUsageTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to analyze (default: current directory)",
			},
			"depth": map[string]interface{}{
				"type":        "number",
				"description": "Directory depth (default: 1)",
			},
			"sort": map[string]interface{}{
				"type":        "boolean",
				"description": "Sort by size (default: true)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DiskUsageTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	depth := 1
	if d, ok := params["depth"].(float64); ok {
		depth = int(d)
	}

	sort := true
	if s, ok := params["sort"].(bool); ok {
		sort = s
	}

	command := fmt.Sprintf("du -h --max-depth=%d %s", depth, shellescape(path))

	if sort {
		command += " | sort -hr"
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
