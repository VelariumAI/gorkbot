// Package engine — background_agents.go
//
// Background Agent Spawning: parallel AI agent execution.
//
// Inspired by oh-my-opencode's background-task system, which allows the main
// agent to fire sub-agents in parallel goroutines, continue its primary work
// immediately, and collect results later via notifications.
//
// In Gorkbot this enables:
//   - The primary AI to spawn specialist sub-tasks while it continues working.
//   - Multiple sub-agents researching/executing in parallel (e.g. one searches
//     docs while another runs tests).
//   - TUI notification when a background agent completes.
//   - Bounded concurrency (default: 4 parallel agents).
//
// Usage (from a tool):
//
//	mgr := engine.BackgroundAgentManagerFromContext(ctx)
//	id := mgr.Spawn(ctx, BackgroundAgentSpec{
//	    Label:  "search-docs",
//	    Prompt: "Find the official Gemini streaming API docs",
//	    Model:  "gemini-2.0-flash",
//	})
//	// ... AI continues other work ...
//	result, err := mgr.Collect(ctx, id)
package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// BackgroundAgentStatus represents the lifecycle state of a spawned agent.
type BackgroundAgentStatus int

const (
	AgentPending   BackgroundAgentStatus = iota // queued, not yet started
	AgentRunning                                // actively generating
	AgentDone                                   // completed successfully
	AgentFailed                                 // completed with error
	AgentCancelled                              // cancelled by caller
)

func (s BackgroundAgentStatus) String() string {
	switch s {
	case AgentPending:
		return "pending"
	case AgentRunning:
		return "running"
	case AgentDone:
		return "done"
	case AgentFailed:
		return "failed"
	case AgentCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// BackgroundAgent captures the full state of a spawned sub-agent.
type BackgroundAgent struct {
	ID        string
	Label     string
	Prompt    string
	ModelID   string
	Status    BackgroundAgentStatus
	Result    string
	Error     error
	StartedAt time.Time
	DoneAt    time.Time
}

// Elapsed returns the time the agent has been running (or its total runtime).
func (a *BackgroundAgent) Elapsed() time.Duration {
	if a.DoneAt.IsZero() {
		return time.Since(a.StartedAt)
	}
	return a.DoneAt.Sub(a.StartedAt)
}

// BackgroundAgentSpec is the input to Spawn().
type BackgroundAgentSpec struct {
	// Label is a short human-readable identifier shown in the TUI.
	Label string

	// Prompt is the full prompt sent to the sub-agent.
	Prompt string

	// Model is the model ID to use (e.g. "gemini-2.0-flash").
	// If empty, the manager's default model is used.
	Model string

	// SystemPrompt is an optional additional system instruction.
	SystemPrompt string
}

// BackgroundAgentDoneFunc is called when a background agent completes.
// It's used to notify the TUI via tea.Program.Send().
type BackgroundAgentDoneFunc func(agentID, label, result string, err error)

// BackgroundAgentManager manages the lifecycle of parallel background agents.
type BackgroundAgentManager struct {
	mu           sync.RWMutex
	agents       map[string]*BackgroundAgent
	semaphore    chan struct{} // limits concurrency
	onDone       BackgroundAgentDoneFunc
	defaultModel string
	counter      int
}

// NewBackgroundAgentManager creates a manager with bounded concurrency.
func NewBackgroundAgentManager(maxConcurrent int, defaultModel string, onDone BackgroundAgentDoneFunc) *BackgroundAgentManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	return &BackgroundAgentManager{
		agents:       make(map[string]*BackgroundAgent),
		semaphore:    make(chan struct{}, maxConcurrent),
		onDone:       onDone,
		defaultModel: defaultModel,
	}
}

// Spawn starts a background agent and returns its unique ID immediately.
// The caller can continue working and collect the result later.
func (m *BackgroundAgentManager) Spawn(ctx context.Context, spec BackgroundAgentSpec, provider ai.AIProvider) string {
	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("bg-%04d", m.counter)
	label := spec.Label
	if label == "" {
		label = fmt.Sprintf("agent-%d", m.counter)
	}

	agent := &BackgroundAgent{
		ID:        id,
		Label:     label,
		Prompt:    spec.Prompt,
		ModelID:   spec.Model,
		Status:    AgentPending,
		StartedAt: time.Now(),
	}
	if agent.ModelID == "" {
		agent.ModelID = m.defaultModel
	}
	m.agents[id] = agent
	m.mu.Unlock()

	go func() {
		// Acquire semaphore slot.
		select {
		case m.semaphore <- struct{}{}:
			defer func() { <-m.semaphore }()
		case <-ctx.Done():
			m.mu.Lock()
			agent.Status = AgentCancelled
			agent.DoneAt = time.Now()
			m.mu.Unlock()
			if m.onDone != nil {
				m.onDone(id, label, "", ctx.Err())
			}
			return
		}

		m.mu.Lock()
		agent.Status = AgentRunning
		m.mu.Unlock()

		// Build prompt with optional system context.
		fullPrompt := spec.Prompt
		if spec.SystemPrompt != "" {
			fullPrompt = spec.SystemPrompt + "\n\n" + spec.Prompt
		}

		var result strings.Builder
		var runErr error

		// Check if provider supports streaming; fall back to simple generation.
		if provider != nil {
			// Use simple Generate (non-streaming) for background agents.
			// Background agents run synchronously in their goroutine.
			res, err := provider.Generate(ctx, fullPrompt)
			if err != nil {
				runErr = err
			} else {
				result.WriteString(res)
			}
		} else {
			runErr = fmt.Errorf("no AI provider available for background agent")
		}

		m.mu.Lock()
		agent.DoneAt = time.Now()
		if runErr != nil {
			agent.Status = AgentFailed
			agent.Error = runErr
		} else {
			agent.Status = AgentDone
			agent.Result = result.String()
		}
		m.mu.Unlock()

		if m.onDone != nil {
			m.onDone(id, label, agent.Result, runErr)
		}
	}()

	return id
}

// Collect returns the result of a background agent.
// If the agent is still running, it blocks until completion or ctx cancellation.
func (m *BackgroundAgentManager) Collect(ctx context.Context, id string) (string, error) {
	for {
		m.mu.RLock()
		agent, exists := m.agents[id]
		m.mu.RUnlock()

		if !exists {
			return "", fmt.Errorf("background agent %q not found", id)
		}

		switch agent.Status {
		case AgentDone:
			return agent.Result, nil
		case AgentFailed:
			return "", agent.Error
		case AgentCancelled:
			return "", fmt.Errorf("background agent %q was cancelled", id)
		}

		// Still running — poll.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
			// continue polling
		}
	}
}

// Status returns the current status of a background agent.
func (m *BackgroundAgentManager) Status(id string) (BackgroundAgentStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if a, ok := m.agents[id]; ok {
		return a.Status, true
	}
	return AgentPending, false
}

// List returns a snapshot of all agents (copy, safe to read without lock).
func (m *BackgroundAgentManager) List() []BackgroundAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]BackgroundAgent, 0, len(m.agents))
	for _, a := range m.agents {
		out = append(out, *a)
	}
	return out
}

// Running returns agents that are currently pending or running.
func (m *BackgroundAgentManager) Running() []BackgroundAgent {
	all := m.List()
	var out []BackgroundAgent
	for _, a := range all {
		if a.Status == AgentPending || a.Status == AgentRunning {
			out = append(out, a)
		}
	}
	return out
}

// Cancel cancels a running agent by ID.
func (m *BackgroundAgentManager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.agents[id]; ok {
		if a.Status == AgentPending || a.Status == AgentRunning {
			a.Status = AgentCancelled
			a.DoneAt = time.Now()
		}
	}
}

// CancelAll cancels all pending and running agents.
func (m *BackgroundAgentManager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.agents {
		if a.Status == AgentPending || a.Status == AgentRunning {
			a.Status = AgentCancelled
			a.DoneAt = time.Now()
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Context key for passing the manager through tool context
// ─────────────────────────────────────────────────────────────────────────────

type bgAgentMgrKey struct{}

// BackgroundAgentManagerToContext stores the manager in a context.
func BackgroundAgentManagerToContext(ctx context.Context, mgr *BackgroundAgentManager) context.Context {
	return context.WithValue(ctx, bgAgentMgrKey{}, mgr)
}

// BackgroundAgentManagerFromContext retrieves the manager from a context.
// Returns nil if not present.
func BackgroundAgentManagerFromContext(ctx context.Context) *BackgroundAgentManager {
	if v := ctx.Value(bgAgentMgrKey{}); v != nil {
		if mgr, ok := v.(*BackgroundAgentManager); ok {
			return mgr
		}
	}
	return nil
}
