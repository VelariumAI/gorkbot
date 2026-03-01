package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// SpawnAgentTool launches specialized subagents asynchronously
type SpawnAgentTool struct {
	tools.BaseTool
	agentManager *Manager
	registry     *tools.Registry
}

// NewSpawnAgentTool creates a new agent spawner tool
func NewSpawnAgentTool(manager *Manager, registry *tools.Registry) *SpawnAgentTool {
	return &SpawnAgentTool{
		BaseTool: tools.NewBaseTool(
			"spawn_agent",
			"Spawn a specialized subagent in the background to perform a complex task. Returns an Agent ID to check status later.",
			tools.CategoryMeta,
			false,
			tools.PermissionAlways,
		),
		agentManager: manager,
		registry:     registry,
	}
}

func (t *SpawnAgentTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_type": map[string]interface{}{
				"type":        "string",
				"description": "Type of specialized agent to use",
				"enum": []string{
					"general-purpose",
					"explore",
					"plan",
					"frontend-styling-expert",
					"full-stack-developer",
					"code-reviewer",
					"test-engineer",
					"redteam-recon",
					"redteam-injection",
					"redteam-xss",
					"redteam-auth",
					"redteam-ssrf",
					"redteam-authz",
					"redteam-reporter",
				},
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Optional description providing additional context (3-5 words)",
			},
			"isolated": map[string]interface{}{
				"type":        "boolean",
				"description": "When true, the agent runs inside a fresh git worktree so its file edits cannot affect the main working tree. The worktree is removed automatically when the agent finishes.",
			},
		},
		"required": []string{"agent_type", "task"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SpawnAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	agentTypeStr, ok := params["agent_type"].(string)
	if !ok {
		return &tools.ToolResult{Success: false, Error: "agent_type is required"}, fmt.Errorf("agent_type required")
	}

	task, ok := params["task"].(string)
	if !ok {
		return &tools.ToolResult{Success: false, Error: "task is required"}, fmt.Errorf("task required")
	}

	description := ""
	if desc, ok := params["description"].(string); ok {
		description = desc
	}

	isolated, _ := params["isolated"].(bool)

	// Use injected registry instead of getting from context if possible,
	// but context is still good for reliability.
	// Prefer Consultant for subagents if available, otherwise Primary
	var aiProvider ai.AIProvider
	if cons := t.registry.GetConsultantProvider(); cons != nil {
		if p, ok := cons.(ai.AIProvider); ok {
			aiProvider = p
		}
	}
	
	if aiProvider == nil {
		if prim := t.registry.GetAIProvider(); prim != nil {
			if p, ok := prim.(ai.AIProvider); ok {
				aiProvider = p
			}
		}
	}

	if aiProvider == nil {
		return &tools.ToolResult{Success: false, Error: "AI provider not configured"}, fmt.Errorf("AI provider not configured")
	}

	// Convert string to AgentType
	agentType := AgentType(agentTypeStr)

	// Optional git worktree isolation.
	var worktreePath string
	if isolated {
		repoRoot, err := agentGitRoot()
		if err == nil {
			wm := NewWorktreeManager(repoRoot)
			// Use a timestamped name so concurrent agents don't collide.
			wtName := fmt.Sprintf("agent-%d", time.Now().UnixNano())
			if p, err := wm.Create(wtName); err == nil {
				worktreePath = p
				task = fmt.Sprintf("[ISOLATION] All file operations MUST be performed within: %s\n\n%s", p, task)
			}
		}
	}

	// Spawn agent async
	// Pass registry to SpawnAgent so the agent can use tools!
	agentID, err := t.agentManager.SpawnAgent(ctx, agentType, task, aiProvider, t.registry)
	if err != nil {
		// Clean up worktree if spawn failed.
		if worktreePath != "" {
			_ = removeWorktreeByPath(worktreePath)
		}
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("agent spawn failed: %v", err),
		}, err
	}

	// If isolated, launch a cleanup goroutine that removes the worktree
	// once the agent finishes (or after a max wait of 15 min).
	if worktreePath != "" {
		mgr := t.agentManager
		wtPath := worktreePath
		go func() {
			deadline := time.Now().Add(15 * time.Minute)
			for time.Now().Before(deadline) {
				time.Sleep(10 * time.Second)
				ag := mgr.GetAgent(agentID)
				if ag == nil || ag.Status != "running" {
					break
				}
			}
			_ = removeWorktreeByPath(wtPath)
		}()
	}

	output := fmt.Sprintf("Agent spawned successfully.\nID: %s\nType: %s\n", agentID, agentType)
	if description != "" {
		output += fmt.Sprintf("Description: %s\n", description)
	}
	if worktreePath != "" {
		output += fmt.Sprintf("Isolated worktree: %s\n", worktreePath)
	}
	output += "\nUse `check_agent_status` with the ID to see results."

	return &tools.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"agent_id":   agentID,
			"agent_type": agentTypeStr,
			"status":     "running",
		},
	}, nil
}

// CheckAgentStatusTool checks the status/result of a spawned agent
type CheckAgentStatusTool struct {
	tools.BaseTool
	agentManager *Manager
}

func NewCheckAgentStatusTool(manager *Manager) *CheckAgentStatusTool {
	return &CheckAgentStatusTool{
		BaseTool: tools.NewBaseTool(
			"check_agent_status",
			"Check the status and results of a spawned subagent",
			tools.CategoryMeta,
			false,
			tools.PermissionAlways,
		),
		agentManager: manager,
	}
}

func (t *CheckAgentStatusTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the agent to check",
			},
		},
		"required": []string{"agent_id"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CheckAgentStatusTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	agentID, ok := params["agent_id"].(string)
	if !ok {
		return &tools.ToolResult{Success: false, Error: "agent_id is required"}, fmt.Errorf("agent_id required")
	}

	agent := t.agentManager.GetAgent(agentID)
	if agent == nil {
		return &tools.ToolResult{Success: false, Error: "agent not found"}, fmt.Errorf("agent not found")
	}

	output := fmt.Sprintf("Agent Status: %s\n", agent.Status)
	output += fmt.Sprintf("Type: %s\n", agent.Type)
	output += fmt.Sprintf("Started: %s\n", agent.Started.Format(time.RFC3339))
	
	if !agent.Completed.IsZero() {
		output += fmt.Sprintf("Completed: %s\n", agent.Completed.Format(time.RFC3339))
		duration := agent.Completed.Sub(agent.Started)
		output += fmt.Sprintf("Duration: %s\n", duration)
	}

	if agent.Status == "completed" {
		output += "\nResult:\n" + agent.Result
	} else if agent.Status == "failed" {
		output += "\nError:\n" + agent.Result
	} else {
		output += "\nThe agent is still working. Check back later."
	}

	return &tools.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"agent_id": agent.ID,
			"status":   agent.Status,
			"result":   agent.Result,
		},
	}, nil
}

// ListAgentsTool lists available agent types
type ListAgentsTool struct {
	tools.BaseTool
	agentManager *Manager
}

// NewListAgentsTool creates a tool to list available agents
func NewListAgentsTool(manager *Manager) *ListAgentsTool {
	return &ListAgentsTool{
		BaseTool: tools.NewBaseTool(
			"list_agents",
			"List all available specialized agent types and their descriptions",
			tools.CategoryMeta,
			false,
			tools.PermissionAlways,
		),
		agentManager: manager,
	}
}

func (t *ListAgentsTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Output format: table, json (default: table)",
				"enum":        []string{"table", "json"},
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListAgentsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	format := "table"
	if f, ok := params["format"].(string); ok {
		format = f
	}

	agents := t.agentManager.GetRegistry().List()

	var output string

	switch format {
	case "json":
		agentData := make([]map[string]string, 0, len(agents))
		for _, agent := range agents {
			agentData = append(agentData, map[string]string{
				"type":        string(agent.Type()),
				"name":        agent.Name(),
				"description": agent.Description(),
			})
		}
		data, _ := json.MarshalIndent(agentData, "", "  ")
		output = string(data)

	case "table":
		output = "Available Specialized Agents\n"
		output += "============================\n\n"
		for _, agent := range agents {
			output += fmt.Sprintf("Type: %s\n", agent.Type())
			output += fmt.Sprintf("Name: %s\n", agent.Name())
			output += fmt.Sprintf("Description: %s\n\n", agent.Description())
		}
	}

	return &tools.ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]interface{}{"count": len(agents)},
	}, nil
}

// ─── worktree helpers (avoid import cycle with pkg/tools) ────────────────────

func agentGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func removeWorktreeByPath(path string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	// Run from parent to avoid "can't remove checked-out worktree" issues.
	if err := cmd.Run(); err != nil {
		return err
	}
	// Best-effort: delete the directory if git didn't.
	_ = os.RemoveAll(path)
	return nil
}
