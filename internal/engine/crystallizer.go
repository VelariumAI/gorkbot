package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Crystallizer daemon monitors conversation history for repeated tool patterns
// and automatically forges new Python plugins to replace complex shell sequences.
type Crystallizer struct {
	orchestrator *Orchestrator
}

func NewCrystallizer(o *Orchestrator) *Crystallizer {
	return &Crystallizer{orchestrator: o}
}

// CheckAndForge should be called periodically (e.g., in a background goroutine).
func (c *Crystallizer) CheckAndForge(ctx context.Context) {
	if c.orchestrator.Consultant == nil {
		return
	}

	// 1. Gather recent history to find repetitive bash patterns.
	history := c.orchestrator.GetHistory()
	if history == nil {
		return
	}

	msgs := history.GetMessages()
	var recentBashCommands []string

	for _, m := range msgs {
		if m.Role == "assistant" && strings.Contains(m.Content, "bash") {
			// Extremely naive extraction for demonstration.
			// Real implementation would parse ToolRequests.
			lines := strings.Split(m.Content, "\n")
			for _, l := range lines {
				if strings.Contains(l, "bash") || strings.Contains(l, "run_bash") {
					recentBashCommands = append(recentBashCommands, l)
				}
			}
		}
	}

	// 2. If we see enough complex bash usage, trigger the Forge.
	// For this prototype, we'll just check if there are a lot of bash calls.
	if len(recentBashCommands) > 5 {
		c.orchestrator.Logger.Info("Crystallizer detected high bash usage. Initiating ToolForge...")
		c.ForgeNewTool(ctx, strings.Join(recentBashCommands, "\n"))
	}
}

type forgedTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PythonCode  string `json:"python_code"`
}

// ForgeNewTool uses the Consultant to write a Python plugin and saves it.
func (c *Crystallizer) ForgeNewTool(ctx context.Context, bashContext string) {
	sysPrompt := `You are the ToolForge. Your task is to analyze the following sequence of bash commands and create a generalized, reusable Python plugin for Gorkbot.

Output ONLY valid JSON in this format:
{
  "name": "tool_name",
  "description": "What the tool does",
  "python_code": "import sys
import json
..."
}

The Python code must read JSON from stdin, perform the operation, and print JSON to stdout {"success": true, "output": "...", "error": ""}.`

	prompt := fmt.Sprintf("Bash commands:\n%s\n\nCreate a python plugin that crystallizes this pattern.", bashContext)

	resp, err := c.orchestrator.Consultant.Generate(ctx, sysPrompt+"\n\n"+prompt)
	if err != nil {
		c.orchestrator.Logger.Warn("ToolForge generation failed", "error", err)
		return
	}

	// Extract JSON
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start >= 0 && end > start {
		resp = resp[start : end+1]
	}

	var forged forgedTool
	if err := json.Unmarshal([]byte(resp), &forged); err != nil {
		c.orchestrator.Logger.Warn("ToolForge JSON parse failed", "error", err)
		return
	}

	// 3. Write to plugins/python/auto_forged/
	c.saveForgedPlugin(forged)
}

func (c *Crystallizer) saveForgedPlugin(tool forgedTool) {
	cwd, _ := os.Getwd()
	pluginDir := filepath.Join(cwd, "plugins", "python", "auto_forged", tool.Name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		c.orchestrator.Logger.Warn("Failed to create plugin dir", "error", err)
		return
	}

	pyPath := filepath.Join(pluginDir, "main.py")
	if err := os.WriteFile(pyPath, []byte(tool.PythonCode), 0755); err != nil {
		c.orchestrator.Logger.Warn("Failed to write python code", "error", err)
		return
	}

	// Generate manifest.json
	manifest := fmt.Sprintf(`{
  "name": "%s",
  "description": "%s",
  "author": "ToolForge",
  "version": "1.0.0",
  "entry_point": "main.py",
  "parameters": {
    "type": "object",
    "properties": {
       "args": { "type": "string", "description": "Arguments for the tool" }
    }
  }
}`, tool.Name, tool.Description)

	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		c.orchestrator.Logger.Warn("Failed to write manifest", "error", err)
		return
	}

	c.orchestrator.Logger.Info("Tool Crystallized Successfully!", "tool", tool.Name)
	// Optionally inform the AI via System message
	if c.orchestrator.ConversationHistory != nil {
		c.orchestrator.ConversationHistory.AddSystemMessage(fmt.Sprintf("[TOOL FORGE]: I have autonomously crystallized a new tool '%s'. It is now available.", tool.Name))
	}
}
