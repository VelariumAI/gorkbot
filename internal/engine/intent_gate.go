package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// IntentGateResult represents the structured output of the intent gate.
type IntentGateResult struct {
	Category    string `json:"category"`
	SpawnAgents []struct {
		Label  string `json:"label"`
		Prompt string `json:"prompt"`
	} `json:"spawn_agents"`
}

// RunIntentGate uses a fast LLM (the Consultant or Primary) to classify the user's intent
// and optionally spawn background agents for research or exploration.
func (o *Orchestrator) RunIntentGate(ctx context.Context, prompt string) *IntentGateResult {
	provider := o.Consultant
	if provider == nil {
		provider = o.Primary
	}
	if provider == nil {
		return nil
	}

	sysPrompt := `You are the Intent Gate. Your job is to classify the user's prompt into a category and decide if background sub-agents should be spawned to gather information BEFORE the main agent answers.

Valid Categories: auto, deep, quick, visual, research, security, code, creative, data, plan

If the task requires exploring a codebase, reading docs, or doing web research, you should spawn agents. Keep agents focused and single-purpose.

Output ONLY valid JSON in this format:
{
  "category": "research",
  "spawn_agents": [
    {
      "label": "grep-docs",
      "prompt": "Find documentation related to X"
    }
  ]
}

If no background agents are needed, leave "spawn_agents" empty.`

	// Use a short timeout to not block the user for long.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create a temporary history just for this call
	history := ai.NewConversationHistory()
	history.AddSystemMessage(sysPrompt)
	history.AddUserMessage(prompt)

	resp, err := provider.GenerateWithHistory(ctx, history)
	if err != nil {
		o.Logger.Warn("Intent Gate failed", "error", err)
		return nil
	}

	// Extract JSON
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start >= 0 && end > start {
		resp = resp[start : end+1]
	}

	var result IntentGateResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		o.Logger.Warn("Intent Gate JSON parse failed", "error", err)
		return nil
	}

	return &result
}
