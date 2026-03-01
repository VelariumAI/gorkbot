package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// MaxDepth is the maximum delegation depth for recursive sub-agents.
const MaxDepth = 4

// depthContextKey is the context key that tracks the current delegation depth.
type depthContextKey struct{}

// SpawnSubAgentTool is a discovery-aware, depth-limited, verifiable sub-agent spawner.
// Unlike SpawnAgentTool it uses live-polled model capabilities to pick the best
// specialist for the given task category and enforces a hard recursion limit.
type SpawnSubAgentTool struct {
	tools.BaseTool
	agentManager *Manager
	registry     *tools.Registry
	disc         *discovery.Manager
}

// NewSpawnSubAgentTool creates the discovery-aware spawner tool.
func NewSpawnSubAgentTool(manager *Manager, reg *tools.Registry, disc *discovery.Manager) *SpawnSubAgentTool {
	return &SpawnSubAgentTool{
		BaseTool: tools.NewBaseTool(
			"spawn_sub_agent",
			"Delegate a sub-task to the best available AI model discovered via live API polling. "+
				"Automatically selects the specialist model (reasoning/speed/coding/general). "+
				"Supports an optional verifier pass. Hard depth limit: 4 levels.",
			tools.CategoryMeta,
			false,
			tools.PermissionAlways,
		),
		agentManager: manager,
		registry:     reg,
		disc:         disc,
	}
}

func (t *SpawnSubAgentTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The sub-task to delegate",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Capability hint for model selection: reasoning, speed, coding, or general (default: general)",
				"enum":        []string{"reasoning", "speed", "coding", "general"},
			},
			"verify": map[string]interface{}{
				"type":        "boolean",
				"description": "When true a secondary reasoning model verifies the primary result",
			},
			"success_criteria": map[string]interface{}{
				"type":        "string",
				"description": "Criteria the verifier checks (only used when verify=true)",
			},
			"isolated": map[string]interface{}{
				"type":        "boolean",
				"description": "Run the agent in an isolated git worktree so edits cannot affect the main tree",
			},
		},
		"required": []string{"task"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SpawnSubAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	// ── depth guard ──────────────────────────────────────────────────────────
	depth := 0
	if d, ok := ctx.Value(depthContextKey{}).(int); ok {
		depth = d
	}
	if depth >= MaxDepth {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("max delegation depth (%d) reached", MaxDepth),
		}, nil
	}
	childCtx := context.WithValue(ctx, depthContextKey{}, depth+1)

	// ── params ───────────────────────────────────────────────────────────────
	task, ok := params["task"].(string)
	if !ok || task == "" {
		return &tools.ToolResult{Success: false, Error: "task is required"}, nil
	}
	categoryStr, _ := params["category"].(string)
	verify, _ := params["verify"].(bool)
	successCriteria, _ := params["success_criteria"].(string)
	isolated, _ := params["isolated"].(bool)

	// ── model selection via discovery ────────────────────────────────────────
	cap := categoryToCapability(categoryStr)
	primaryModelID, primaryProvider := t.selectModel(cap, "")
	aiProvider := t.providerForModel(primaryModelID, primaryProvider)
	if aiProvider == nil {
		return &tools.ToolResult{Success: false, Error: "no AI provider available"}, nil
	}

	// ── optional worktree isolation ──────────────────────────────────────────
	var worktreePath string
	if isolated {
		if root, err := agentGitRoot(); err == nil {
			wm := NewWorktreeManager(root)
			wtName := fmt.Sprintf("sub-agent-%d", time.Now().UnixNano())
			if p, err := wm.Create(wtName); err == nil {
				worktreePath = p
				task = fmt.Sprintf("[ISOLATION] All file operations MUST be within: %s\n\n%s", p, task)
			}
		}
	}

	// ── spawn primary agent ──────────────────────────────────────────────────
	agentID, err := t.agentManager.SpawnAgent(childCtx, AgentType("general-purpose"), task, aiProvider, t.registry)
	if err != nil {
		if worktreePath != "" {
			_ = removeWorktreeByPath(worktreePath)
		}
		return &tools.ToolResult{Success: false, Error: "spawn failed: " + err.Error()}, err
	}

	// Register node in discovery manager for TUI tree rendering.
	if t.disc != nil {
		t.disc.RegisterAgent(&discovery.AgentNode{
			ID:        agentID,
			Task:      truncate(task, 60),
			ModelID:   primaryModelID,
			Depth:     depth + 1,
			Status:    "running",
			StartedAt: time.Now(),
		})
	}

	// ── cleanup + optional verifier ──────────────────────────────────────────
	go func() {
		result := t.waitForAgent(agentID)
		status := "done"
		if result == "" {
			status = "failed"
		}
		if t.disc != nil {
			t.disc.UpdateAgent(agentID, status)
		}
		if worktreePath != "" {
			_ = removeWorktreeByPath(worktreePath)
		}
	}()

	var verifierNote string
	if verify {
		verModelID, verProvider := t.selectModel(discovery.CapReasoning, "")
		if verModelID != primaryModelID {
			if verAI := t.providerForModel(verModelID, verProvider); verAI != nil {
				verifierNote = fmt.Sprintf("\nVerifier: %s (%s) will check results", verModelID, verProvider)
				if t.disc != nil {
					t.disc.UpdateAgent(agentID, "verifying")
				}
				go t.runVerifier(childCtx, agentID, task, successCriteria, verAI)
			}
		}
	}

	catDisplay := categoryStr
	if catDisplay == "" {
		catDisplay = "general"
	}
	output := fmt.Sprintf(
		"Sub-agent spawned (depth %d/%d)\nID: %s\nModel: %s (%s)\nCategory: %s%s\n\nUse `check_agent_status` with the ID to poll results.",
		depth+1, MaxDepth, agentID, primaryModelID, primaryProvider, catDisplay, verifierNote,
	)

	return &tools.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"agent_id":  agentID,
			"model":     primaryModelID,
			"provider":  primaryProvider,
			"depth":     depth + 1,
			"max_depth": MaxDepth,
		},
	}, nil
}

// ─── model selection ─────────────────────────────────────────────────────────

func (t *SpawnSubAgentTool) selectModel(cap discovery.CapabilityClass, preferProvider string) (id, provider string) {
	if t.disc != nil {
		if m := t.disc.BestForCap(cap, preferProvider); m != nil {
			return m.ID, m.Provider
		}
	}
	// Fall back to the registry's primary provider.
	if p := t.registry.GetAIProvider(); p != nil {
		if ap, ok := p.(ai.AIProvider); ok {
			meta := ap.GetMetadata()
			return meta.ID, "xai"
		}
	}
	return "grok-3", "xai"
}

func (t *SpawnSubAgentTool) providerForModel(modelID, providerID string) ai.AIProvider {
	if providerID == "google" {
		if p := t.registry.GetConsultantProvider(); p != nil {
			if ap, ok := p.(ai.AIProvider); ok {
				return ap.WithModel(modelID)
			}
		}
	}
	if p := t.registry.GetAIProvider(); p != nil {
		if ap, ok := p.(ai.AIProvider); ok {
			return ap.WithModel(modelID)
		}
	}
	return nil
}

// ─── agent lifecycle helpers ─────────────────────────────────────────────────

// waitForAgent polls until the agent finishes (up to 15 min) and returns its result.
func (t *SpawnSubAgentTool) waitForAgent(agentID string) string {
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		ag := t.agentManager.GetAgent(agentID)
		if ag == nil {
			return ""
		}
		switch ag.Status {
		case "completed":
			return ag.Result
		case "failed":
			return ""
		}
	}
	return ""
}

// runVerifier waits for the primary agent, runs a verification pass, then uses
// the Synthesizer to merge both results and stores the consensus back on the agent.
func (t *SpawnSubAgentTool) runVerifier(ctx context.Context, primaryID, task, criteria string, verAI ai.AIProvider) {
	primaryResult := t.waitForAgent(primaryID)
	if primaryResult == "" {
		return
	}

	// Generate verifier output.
	history := ai.NewConversationHistory()
	history.AddMessage("user", buildVerifyPrompt(task, primaryResult, criteria))
	verifierResult, err := verAI.GenerateWithHistory(ctx, history)
	if err != nil || verifierResult == "" {
		// Verification failed — leave primary result unchanged.
		if t.disc != nil {
			t.disc.UpdateAgent(primaryID, "done")
		}
		return
	}

	// Synthesize primary + verifier into a consensus.
	synth := NewSynthesizer(verAI)
	sr, err := synth.Synthesize(ctx, []SourcedResult{
		{Label: "primary", Output: primaryResult},
		{Label: "verifier", Output: verifierResult},
	})
	if err == nil && sr != nil {
		t.agentManager.UpdateResult(primaryID, sr.Format())
	}

	if t.disc != nil {
		t.disc.UpdateAgent(primaryID, "done")
	}
}

func buildVerifyPrompt(task, result, criteria string) string {
	var b strings.Builder
	b.WriteString("You are an independent reviewer verifying work done by another AI agent.\n\n")
	b.WriteString("ORIGINAL TASK:\n")
	b.WriteString(task)
	b.WriteString("\n\nAGENT RESULT:\n")
	b.WriteString(result)
	if criteria != "" {
		b.WriteString("\n\nSUCCESS CRITERIA:\n")
		b.WriteString(criteria)
	}
	b.WriteString("\n\nEvaluate whether the result satisfies the task. State PASS or FAIL with clear reasoning.")
	return b.String()
}

// ─── utils ───────────────────────────────────────────────────────────────────

func categoryToCapability(cat string) discovery.CapabilityClass {
	switch strings.ToLower(cat) {
	case "reasoning":
		return discovery.CapReasoning
	case "speed":
		return discovery.CapSpeed
	case "coding":
		return discovery.CapCoding
	default:
		return discovery.CapGeneral
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
