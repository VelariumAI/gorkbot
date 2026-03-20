package subagents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// AgentType defines different specialized agent types
type AgentType string

const (
	AgentTypeGeneralPurpose        AgentType = "general-purpose"
	AgentTypeExplore               AgentType = "explore"
	AgentTypePlan                  AgentType = "plan"
	AgentTypeFrontendStylingExpert AgentType = "frontend-styling-expert"
	AgentTypeFullStackDeveloper    AgentType = "full-stack-developer"
	AgentTypeCodeReviewer          AgentType = "code-reviewer"
	AgentTypeTestEngineer          AgentType = "test-engineer"
	AgentTypeRedTeamRecon          AgentType = "redteam-recon"
	AgentTypeRedTeamInjection      AgentType = "redteam-injection"
	AgentTypeRedTeamXSS            AgentType = "redteam-xss"
	AgentTypeRedTeamAuth           AgentType = "redteam-auth"
	AgentTypeRedTeamSSRF           AgentType = "redteam-ssrf"
	AgentTypeRedTeamAuthz          AgentType = "redteam-authz"
	AgentTypeRedTeamReporter       AgentType = "redteam-reporter"
)

// Agent represents a specialized AI agent
type Agent interface {
	Type() AgentType
	Name() string
	Description() string
	SystemPrompt() string
	Execute(ctx context.Context, task string, provider ai.AIProvider, registry *tools.Registry) (string, error)
}

// BaseAgent provides common functionality for all agents
type BaseAgent struct {
	agentType    AgentType
	name         string
	description  string
	systemPrompt string
}

func (a *BaseAgent) Type() AgentType {
	return a.agentType
}

func (a *BaseAgent) Name() string {
	return a.name
}

func (a *BaseAgent) Description() string {
	return a.description
}

func (a *BaseAgent) SystemPrompt() string {
	return a.systemPrompt
}

func (a *BaseAgent) Execute(ctx context.Context, task string, provider ai.AIProvider, registry *tools.Registry) (string, error) {
	// Initialize history
	history := ai.NewConversationHistory()

	// Strong Preamble to override sandbox hallucinations
	preamble := `CRITICAL OPERATIONAL PROTOCOL:
1. You are a specialized sub-agent running within Gorkbot.
2. You HAVE FULL ACCESS to the local system via the provided tools.
3. You MUST EXECUTE tools directly to perform tasks (e.g., reading files, running shell commands).
4. DO NOT simulate actions or provide hypothetical advice if you can execute them.
5. If you lack permissions, execute the tool anyway; the system will prompt the user for approval.
6. The user has explicitly authorized you to modify the project.`

	// Add system prompt with tool context if registry is available
	systemPrompt := preamble + "\n\n" + a.systemPrompt
	// Inject shared security context brief for red team agents
	if registry != nil {
		brief := registry.GetSecurityBrief()
		if brief != "" {
			systemPrompt += "\n\n" + brief
		}
		systemPrompt += "\n\n" + registry.GetSystemPrompt()
	}
	history.AddSystemMessage(systemPrompt)

	// Add user task
	history.AddUserMessage(task)

	maxTurns := 10
	var finalResponse string

	for turn := 0; turn < maxTurns; turn++ {
		// Check shared session budget if available.
		if budget := BudgetFromContext(ctx); budget != nil {
			if !budget.Consume(1) {
				finalResponse += fmt.Sprintf("\n\n[Session budget exhausted: %s]", budget.Report())
				break
			}
		}

		// Generate response
		response, err := provider.GenerateWithHistory(ctx, history)
		if err != nil {
			return "", fmt.Errorf("agent %s failed: %w", a.name, err)
		}

		history.AddAssistantMessage(response)
		finalResponse = response

		// Parse tool requests
		// Note: We use the public ParseToolRequests from pkg/tools
		toolRequests := tools.ParseToolRequests(response)

		if len(toolRequests) == 0 {
			break
		}

		if registry == nil {
			// If no registry, we can't execute tools.
			// We should probably inform the agent or just stop?
			// For now, let's stop to prevent loops.
			break
		}

		// Execute tools
		toolResults := make([]string, len(toolRequests))
		var wg sync.WaitGroup

		for i, req := range toolRequests {
			wg.Add(1)
			go func(i int, req tools.ToolRequest) {
				defer wg.Done()
				result, err := registry.Execute(ctx, &req)
				if err != nil {
					result = &tools.ToolResult{
						Success: false,
						Error:   err.Error(),
					}
				}

				var resultStr string
				if result.Success {
					resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: true\nOutput:\n%s\n</tool_result>",
						req.ToolName, result.Output)
				} else {
					resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: false\nError: %s\n</tool_result>",
						req.ToolName, result.Error)
				}
				toolResults[i] = resultStr
			}(i, req)
		}
		wg.Wait()

		// Add results to history
		toolResultsMessage := "Here are the results from the tools you requested:\n\n"
		toolResultsMessage += fmt.Sprintf("%s\n\n", resultStrJoin(toolResults, "\n\n"))
		toolResultsMessage += "Please continue with the task based on these results."

		history.AddUserMessage(toolResultsMessage)
	}

	return finalResponse, nil
}

func resultStrJoin(items []string, sep string) string {
	// Simple join helper since strings.Join is standard
	// Just re-implementing to avoid import loop issues if any, but strings is fine.
	// Actually we imported "fmt" and "context" and "ai" and "tools".
	// We need "strings" if we use strings.Join.
	// Let's assume strings is imported or add it.
	// Wait, I didn't add strings to imports in previous step. I should have.
	// I'll assume I can add it or write a simple loop.
	res := ""
	for i, item := range items {
		if i > 0 {
			res += sep
		}
		res += item
	}
	return res
}

// AgentRegistry manages available agents
type AgentRegistry struct {
	agents map[AgentType]Agent
}

func NewAgentRegistry() *AgentRegistry {
	registry := &AgentRegistry{
		agents: make(map[AgentType]Agent),
	}

	// Register all built-in agents
	registry.Register(NewGeneralPurposeAgent())
	registry.Register(NewExploreAgent())
	registry.Register(NewPlanAgent())
	registry.Register(NewFrontendStylingExpert())
	registry.Register(NewFullStackDeveloper())
	registry.Register(NewCodeReviewerAgent())
	registry.Register(NewTestEngineerAgent())

	// Red Team Specialists (Shannon-inspired)
	registry.Register(NewRedTeamReconAgent())
	registry.Register(NewRedTeamInjectionAgent())
	registry.Register(NewRedTeamXSSAgent())
	registry.Register(NewRedTeamAuthAgent())
	registry.Register(NewRedTeamSSRFAgent())
	registry.Register(NewRedTeamAuthzAgent())
	registry.Register(NewRedTeamReporterAgent())

	return registry
}

func (r *AgentRegistry) Register(agent Agent) {
	r.agents[agent.Type()] = agent
}

func (r *AgentRegistry) Get(agentType AgentType) (Agent, bool) {
	agent, exists := r.agents[agentType]
	return agent, exists
}

func (r *AgentRegistry) List() []Agent {
	agents := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	return agents
}

// Running agent tracking
type RunningAgent struct {
	ID        string
	Type      AgentType
	Task      string
	Started   time.Time
	Completed time.Time
	Status    string // running, completed, failed
	Result    string
	Error     error
}

// Manager manages running agents
type Manager struct {
	mu              sync.Mutex
	registry        *AgentRegistry
	runningAgents   map[string]*RunningAgent
	completedAgents map[string]*RunningAgent
}

func NewManager() *Manager {
	return &Manager{
		registry:        NewAgentRegistry(),
		runningAgents:   make(map[string]*RunningAgent),
		completedAgents: make(map[string]*RunningAgent),
	}
}

func (m *Manager) GetRegistry() *AgentRegistry {
	return m.registry
}

// cleanupCompletedAgents removes completed agents older than 30 minutes.
// Must be called with m.mu held.
func (m *Manager) cleanupCompletedAgents() {
	cutoff := time.Now().Add(-30 * time.Minute)
	for id, agent := range m.completedAgents {
		if agent.Completed.Before(cutoff) {
			delete(m.completedAgents, id)
		}
	}
}

func (m *Manager) SpawnAgent(ctx context.Context, agentType AgentType, task string, provider ai.AIProvider, registry *tools.Registry) (string, error) {
	agent, exists := m.registry.Get(agentType)
	if !exists {
		// Build a list of valid types to help the caller correct the mistake.
		valid := make([]string, 0, len(m.registry.agents))
		for t := range m.registry.agents {
			valid = append(valid, string(t))
		}
		// Sort for deterministic output (simple insertion sort — small slice).
		for i := 1; i < len(valid); i++ {
			for j := i; j > 0 && valid[j] < valid[j-1]; j-- {
				valid[j], valid[j-1] = valid[j-1], valid[j]
			}
		}
		return "", fmt.Errorf("agent type %q not found; valid types: %v", agentType, valid)
	}

	// Create running agent entry
	agentID := fmt.Sprintf("agent_%d", time.Now().UnixNano())
	running := &RunningAgent{
		ID:      agentID,
		Type:    agentType,
		Task:    task,
		Started: time.Now(),
		Status:  "running",
	}

	m.mu.Lock()
	m.cleanupCompletedAgents()
	m.runningAgents[agentID] = running
	m.mu.Unlock()

	// Execute agent in a goroutine (async)
	go func() {
		// Create a detached context with timeout to ensure the agent completes even if the parent request ends
		// but doesn't run forever.
		agentCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		result, err := agent.Execute(agentCtx, task, provider, registry)

		m.mu.Lock()
		defer m.mu.Unlock()

		// Move from running to completed
		if _, ok := m.runningAgents[agentID]; ok {
			delete(m.runningAgents, agentID)
		}

		if err != nil {
			running.Status = "failed"
			running.Error = err
			// Store error string in Result for easier reading if needed
			running.Result = fmt.Sprintf("Error: %v", err)
		} else {
			running.Status = "completed"
			running.Result = result
		}

		running.Completed = time.Now()
		m.completedAgents[agentID] = running
	}()

	return agentID, nil
}

func (m *Manager) GetAgent(agentID string) *RunningAgent {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, ok := m.runningAgents[agentID]; ok {
		return agent
	}
	if agent, ok := m.completedAgents[agentID]; ok {
		return agent
	}
	return nil
}

func (m *Manager) GetRunningAgents() []*RunningAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	agents := make([]*RunningAgent, 0, len(m.runningAgents))
	for _, agent := range m.runningAgents {
		agents = append(agents, agent)
	}
	return agents
}

func (m *Manager) GetCompletedAgents() []*RunningAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	agents := make([]*RunningAgent, 0, len(m.completedAgents))
	for _, agent := range m.completedAgents {
		agents = append(agents, agent)
	}
	return agents
}

// UpdateResult replaces the stored result for a completed agent.
// Used by the synthesizer to inject the consensus output after a verifier pass.
func (m *Manager) UpdateResult(agentID, newResult string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ag, ok := m.completedAgents[agentID]; ok {
		ag.Result = newResult
		return true
	}
	return false
}
