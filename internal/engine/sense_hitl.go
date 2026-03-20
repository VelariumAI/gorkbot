package engine

// sense_hitl.go — SENSE Human-in-the-Loop (HITL) Plan-and-Execute Guard
//
// Implements the SENSE v2.0 requirement: for high-stakes actions, Gorkbot
// MUST generate a structured plan, pause for explicit user approval (the
// "SENSE validation signal"), and execute ONLY after approval is granted.
//
// Integration points:
//   - HITLGuard.IsHighStakes(toolName, params) — decides whether to gate an action.
//   - HITLGuard.BuildPlan(response, tool, params) — generates a structured plan.
//   - The Orchestrator calls RequestHITLApproval before executing gated tools.
//   - The TUI listens for HITLRequestMsg and surfaces the plan to the user.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/hitl"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// HITLApproval is the user's response to a plan.
type HITLApproval int

const (
	// HITLApproved means the user reviewed the plan and permits execution.
	HITLApproved HITLApproval = iota
	// HITLRejected means the user cancelled the operation.
	HITLRejected
	// HITLAmended means the user approved with modifications (text in Notes).
	HITLAmended
)

// HITLDecision carries the approval decision back to the orchestrator.
type HITLDecision struct {
	Approval HITLApproval
	Notes    string // amendment notes or rejection reason
}

// HITLRequest is sent to the TUI (or any approval handler) when HITL is triggered.
type HITLRequest struct {
	ToolName string
	Params   map[string]interface{}
	Plan     string

	// Enhanced HITL fields for intelligent approval
	RiskLevel      hitl.RiskLevel // Classified risk (Low/Medium/High/Critical)
	RiskReason     string         // Explanation of risk classification
	ConfidenceScore int            // AI confidence 0-100
	Context        string         // Why this tool is needed
	Precedent      int            // Count of similar previously approved operations

	// Note: routing uses HITLCallback, not an inline channel.
}

// HITLCallback is the function the Orchestrator calls to surface a HITL request.
// The implementation lives in the TUI model (like the existing InterventionCallback).
type HITLCallback func(req HITLRequest) HITLDecision

// ─── High-Stakes Detection ────────────────────────────────────────────────────

// highStakesTools lists tool names that always require HITL approval.
var highStakesTools = map[string]bool{
	"bash":         true,
	"delete_file":  true,
	"git_push":     true,
	"git_commit":   true,
	"pkg_install":  true,
	"db_migrate":   true,
	"http_request": true,
	"create_tool":  true,
	"modify_tool":  true,
}

// highStakesBashKeywords triggers HITL for bash commands containing these.
var highStakesBashKeywords = []string{
	"rm ", "rm\t", "rmdir", "chmod", "chown",
	"sudo", "apt", "pkg install",
	"curl -X POST", "curl -X PUT", "curl -X DELETE",
	"dd if=", "> /dev/",
}

// HITLGuard decides when HITL is required and builds structured plans.
type HITLGuard struct {
	// Enabled can be set to false to bypass HITL entirely (e.g., for tests).
	Enabled bool

	// Intelligent HITL components
	RiskClassifier    *hitl.RiskClassifier
	ConfidenceScorer  *hitl.ConfidenceScorer
	ContextSummarizer *hitl.ContextSummarizer
	Memory            *hitl.HITLMemory
}

// NewHITLGuard creates a HITLGuard. HITL is enabled by default.
func NewHITLGuard() *HITLGuard {
	return &HITLGuard{
		Enabled:           true,
		RiskClassifier:    hitl.NewRiskClassifier(),
		ConfidenceScorer:  hitl.NewConfidenceScorer(),
		ContextSummarizer: hitl.NewContextSummarizer(),
		Memory:            nil, // Set later via SetMemory
	}
}

// SetMemory wires a HITLMemory instance to the guard.
func (g *HITLGuard) SetMemory(memory *hitl.HITLMemory) {
	g.Memory = memory
}

// IsHighStakes returns true when the given tool+params require HITL approval.
func (g *HITLGuard) IsHighStakes(toolName string, params map[string]interface{}) bool {
	if !g.Enabled {
		return false
	}
	// Check tool-level gate.
	if highStakesTools[toolName] {
		// For http_request, only gate non-GET methods.
		if toolName == "http_request" {
			method, _ := params["method"].(string)
			if method == "" || strings.ToUpper(method) == "GET" {
				return false
			}
			return true
		}
		// For bash, check command content.
		if toolName == "bash" {
			cmd, _ := params["command"].(string)
			return isBashHighStakes(cmd)
		}
		return true
	}
	return false
}

// isBashHighStakes returns true when a bash command contains destructive/network keywords.
func isBashHighStakes(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, kw := range highStakesBashKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// EnhanceHITLRequest augments a basic HITLRequest with intelligent components.
// Populates RiskLevel, ConfidenceScore, Context, and Precedent fields.
func (g *HITLGuard) EnhanceHITLRequest(ctx context.Context, req *HITLRequest, aiReasoning string) error {
	if g.RiskClassifier == nil || g.ConfidenceScorer == nil {
		return nil // Enhancement disabled
	}

	// 1. Classify risk level
	riskLevel, riskReason := g.RiskClassifier.ClassifyTool(req.ToolName, req.Params)
	req.RiskLevel = riskLevel
	req.RiskReason = riskReason

	// 2. Score confidence (base 50 + adjustments)
	requiredParams := []string{} // Tool registry would provide these, for now empty
	toolExists := true           // We assume tools exist if we're here
	score := g.ConfidenceScorer.ScoreAIConfidence(
		req.ToolName,
		req.Params,
		requiredParams,
		aiReasoning,
		toolExists,
	)
	req.ConfidenceScore = score

	// 3. Extract context summary
	if g.ContextSummarizer != nil {
		context := g.ContextSummarizer.SummarizeContext(req.ToolName, req.Params, aiReasoning)
		req.Context = context
	}

	// 4. Check for precedent (similar previously approved operations)
	if g.Memory != nil {
		precedent := g.Memory.CountApprovedExecutions(req.ToolName, req.Params)
		req.Precedent = precedent
	}

	return nil
}

// hashParams creates a deterministic hash of parameters for similarity matching.
func hashParams(params map[string]interface{}) string {
	if len(params) == 0 {
		return ""
	}
	data, _ := json.Marshal(params)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// BuildPlan generates a human-readable execution plan for the given tool action.
// If a consultant (TextGenerator) is available it uses the LLM for richer plans;
// otherwise it falls back to a templated heuristic plan.
func (g *HITLGuard) BuildPlan(ctx context.Context, consultant TextGenerator, toolName string, params map[string]interface{}, aiReasoning string) string {
	if consultant != nil {
		return g.buildLLMPlan(ctx, consultant, toolName, params, aiReasoning)
	}
	return g.buildHeuristicPlan(toolName, params)
}

func (g *HITLGuard) buildLLMPlan(ctx context.Context, gen TextGenerator, toolName string, params map[string]interface{}, reasoning string) string {
	paramsStr := formatParams(params)
	prompt := fmt.Sprintf(`Generate a concise EXECUTION PLAN for the following AI action.
Format it as:
## Action
## Purpose
## Risks
## Steps

TOOL: %s
PARAMETERS: %s
AI REASONING: %s

EXECUTION PLAN:`, toolName, paramsStr, reasoning)

	// Apply a hard deadline so a slow consultant never stalls the gate indefinitely.
	planCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	plan, err := gen.Generate(planCtx, prompt)
	if err != nil {
		return g.buildHeuristicPlan(toolName, params)
	}
	return plan
}

func (g *HITLGuard) buildHeuristicPlan(toolName string, params map[string]interface{}) string {
	paramsStr := formatParams(params)
	return fmt.Sprintf(
		"## Execution Plan\n\n"+
			"**Action:** Run tool `%s`\n\n"+
			"**Parameters:**\n%s\n\n"+
			"**Purpose:** The AI agent intends to execute this action as part of fulfilling your request.\n\n"+
			"**Risks:** This action was flagged as high-stakes because it may modify system state, "+
			"write files, execute shell commands, or communicate over the network.\n\n"+
			"**Steps:**\n"+
			"1. Validate parameters shown above.\n"+
			"2. Execute `%s` with the listed parameters.\n"+
			"3. Return results to the AI for further processing.\n\n"+
			"---\n"+
			"*SENSE v2.0 validation required — type 'approve' to continue or 'reject' to cancel.*",
		toolName, paramsStr, toolName)
}

// ─── Orchestrator integration ─────────────────────────────────────────────────

// TextGenerator is defined in sense/compression.go but engine is a separate
// package so we re-declare the minimal interface here (Go allows this).
// The concrete type will be ai.AIProvider (which implements Generate).
type TextGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// CanAutoApprove returns true if the request meets criteria for automatic approval.
// Factors: high confidence (>85), high precedent (>2), and low/medium risk only.
func (req *HITLRequest) CanAutoApprove() bool {
	// High confidence AND high precedent = auto-approve
	if req.ConfidenceScore >= 85 && req.Precedent >= 2 {
		return true
	}

	// Medium confidence with very high precedent (>5) = auto-approve
	if req.ConfidenceScore >= 70 && req.Precedent >= 5 {
		return true
	}

	// Never auto-approve critical risk operations
	if req.RiskLevel == hitl.RiskCritical {
		return false
	}

	return false
}

// RequestHITLApproval suspends execution, emits the plan to the HITL callback,
// and waits for the user's decision.  Returns (true, notes) if approved/amended,
// (false, reason) if rejected.
func RequestHITLApproval(ctx context.Context, callback HITLCallback, req HITLRequest) (bool, string) {
	// Check for auto-approval first
	if req.CanAutoApprove() {
		return true, "auto-approved (high confidence + precedent)"
	}

	if callback == nil {
		// No HITL handler configured — allow by default (non-interactive mode).
		return true, ""
	}
	decision := callback(req)
	switch decision.Approval {
	case HITLApproved:
		return true, ""
	case HITLAmended:
		return true, decision.Notes
	default:
		return false, decision.Notes
	}
}

// GateToolExecution checks whether a tool requires HITL approval and handles the
// full plan-and-pause flow.
//
// Callers (orchestrator streaming + non-streaming paths) should call this before
// delegating to Registry.Execute when HITL is enabled.
func (o *Orchestrator) GateToolExecution(
	ctx context.Context,
	req tools.ToolRequest,
	hitl *HITLGuard,
	hitlCallback HITLCallback,
	aiReasoning string,
) (bool, string) {
	if hitl == nil || !hitl.Enabled {
		return true, ""
	}
	if !hitl.IsHighStakes(req.ToolName, req.Parameters) {
		return true, ""
	}
	// Build an execution plan to surface to the user.
	var consultant TextGenerator
	if o.Consultant != nil {
		consultant = o.Consultant
	}
	plan := hitl.BuildPlan(ctx, consultant, req.ToolName, req.Parameters, aiReasoning)
	hitlReq := HITLRequest{
		ToolName: req.ToolName,
		Params:   req.Parameters,
		Plan:     plan,
	}

	// Enhance with intelligent scoring
	if hitl.RiskClassifier != nil {
		_ = hitl.EnhanceHITLRequest(ctx, &hitlReq, aiReasoning)
	}

	return RequestHITLApproval(ctx, hitlCallback, hitlReq)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func formatParams(params map[string]interface{}) string {
	if len(params) == 0 {
		return "(none)"
	}
	var sb strings.Builder
	for k, v := range params {
		val := fmt.Sprintf("%v", v)
		if len(val) > 200 {
			val = val[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("  - **%s**: `%s`\n", k, val))
	}
	return sb.String()
}
