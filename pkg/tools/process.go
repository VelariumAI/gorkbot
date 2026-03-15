package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/process"
)

// StartManagedProcessTool allows the AI to start a background process
type StartManagedProcessTool struct {
	BaseTool
	manager *process.Manager
}

// NewStartManagedProcessTool creates a new start process tool
func NewStartManagedProcessTool(pm *process.Manager) *StartManagedProcessTool {
	return &StartManagedProcessTool{
		BaseTool: BaseTool{
			name:               "start_background_process",
			description:        "Start a long-running background process or server managed by Gorkbot. Returns a Process ID.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
		manager: pm,
	}
}

func (t *StartManagedProcessTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command to execute (e.g., 'npm start')",
			},
			"args": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Arguments for the command",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Optional custom ID for the process",
			},
		},
		"required": []string{"command"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *StartManagedProcessTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	command, ok := params["command"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "command is required"}, fmt.Errorf("invalid command")
	}

	var args []string
	if a, ok := params["args"].([]interface{}); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}
	}

	id := ""
	if i, ok := params["id"].(string); ok {
		id = i
	}
	if id == "" {
		id = fmt.Sprintf("proc-%d", time.Now().UnixNano())
	}

	// Use PTY for AI started processes too, to capture colored output
	proc, err := t.manager.Start(id, command, args, true)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Process started with ID: %s. Use list_background_processes or stop_background_process to manage it.", proc.ID),
		Data: map[string]interface{}{
			"process_id": proc.ID,
		},
	}, nil
}

// ListManagedProcessesTool allows the AI to see running processes
type ListManagedProcessesTool struct {
	BaseTool
	manager *process.Manager
}

func NewListManagedProcessesTool(pm *process.Manager) *ListManagedProcessesTool {
	return &ListManagedProcessesTool{
		BaseTool: BaseTool{
			name:               "list_background_processes",
			description:        "List all active background processes managed by Gorkbot.",
			category:           CategorySystem,
			requiresPermission: false,
		},
		manager: pm,
	}
}

func (t *ListManagedProcessesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *ListManagedProcessesTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	procs := t.manager.ListProcesses()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Active Processes (%d):\n", len(procs)))
	for _, p := range procs {
		sb.WriteString(fmt.Sprintf("- [%s] %s (State: %s, Started: %s)\n",
			p.ID, p.Command, p.State, p.StartTime.Format("15:04:05")))
	}

	return &ToolResult{
		Success: true,
		Output:  sb.String(),
	}, nil
}

// StopManagedProcessTool allows the AI to stop a process
type StopManagedProcessTool struct {
	BaseTool
	manager *process.Manager
}

func NewStopManagedProcessTool(pm *process.Manager) *StopManagedProcessTool {
	return &StopManagedProcessTool{
		BaseTool: BaseTool{
			name:               "stop_background_process",
			description:        "Stop a running background process by ID.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
		manager: pm,
	}
}

func (t *StopManagedProcessTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the process to stop",
			},
		},
		"required": []string{"id"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *StopManagedProcessTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	id, ok := params["id"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "id is required"}, fmt.Errorf("invalid id")
	}

	if err := t.manager.Stop(id); err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Process %s stopped successfully.", id),
	}, nil
}

// ReadManagedProcessOutputTool allows the AI to read the buffered output of a process
type ReadManagedProcessOutputTool struct {
	BaseTool
	manager *process.Manager
}

func NewReadManagedProcessOutputTool(pm *process.Manager) *ReadManagedProcessOutputTool {
	return &ReadManagedProcessOutputTool{
		BaseTool: BaseTool{
			name:               "read_background_process_output",
			description:        "Read the stdout/stderr output of a running or completed managed process.",
			category:           CategorySystem,
			requiresPermission: false,
		},
		manager: pm,
	}
}

func (t *ReadManagedProcessOutputTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the process",
			},
			"lines": map[string]interface{}{
				"type":        "number",
				"description": "Number of recent lines to read (default: all)",
			},
		},
		"required": []string{"id"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ReadManagedProcessOutputTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	id, ok := params["id"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "id is required"}, fmt.Errorf("invalid id")
	}

	proc, exists := t.manager.GetProcess(id)
	if !exists {
		return &ToolResult{Success: false, Error: "process not found"}, fmt.Errorf("process not found")
	}

	// Safely access output
	output := proc.GetOutput()

	// Handle lines limit if requested
	if l, ok := params["lines"].(float64); ok && l > 0 {
		lines := strings.Split(output, "\n")
		limit := int(l)
		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
			output = strings.Join(lines, "\n")
		}
	}

	return &ToolResult{
		Success: true,
		Output:  output,
	}, nil
}
