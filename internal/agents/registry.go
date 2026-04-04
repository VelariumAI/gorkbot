package agents

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Agent represents a remote or local agent
type Agent struct {
	ID           string
	Name         string
	Endpoint     string
	Capabilities []string
	Status       string // "online", "offline", "error"
	LastSeen     time.Time
	Version      string
	Metadata     map[string]interface{}
}

// AgentRegistry manages agent discovery and coordination
type AgentRegistry struct {
	logger    *slog.Logger
	agents    map[string]*Agent
	byCapability map[string][]*Agent
	mu        sync.RWMutex
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry(logger *slog.Logger) *AgentRegistry {
	if logger == nil {
		logger = slog.Default()
	}

	return &AgentRegistry{
		logger:       logger,
		agents:       make(map[string]*Agent),
		byCapability: make(map[string][]*Agent),
	}
}

// RegisterAgent registers an agent
func (ar *AgentRegistry) RegisterAgent(agent *Agent) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if agent.ID == "" {
		return fmt.Errorf("agent ID required")
	}

	ar.agents[agent.ID] = agent
	agent.LastSeen = time.Now()

	// Index by capabilities
	for _, cap := range agent.Capabilities {
		ar.byCapability[cap] = append(ar.byCapability[cap], agent)
	}

	ar.logger.Debug("registered agent",
		slog.String("id", agent.ID),
		slog.String("name", agent.Name),
		slog.Int("capabilities", len(agent.Capabilities)),
	)

	return nil
}

// GetAgent retrieves an agent by ID
func (ar *AgentRegistry) GetAgent(id string) *Agent {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	return ar.agents[id]
}

// FindAgentsByCapability finds agents with specific capability
func (ar *AgentRegistry) FindAgentsByCapability(capability string) []*Agent {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	agents := make([]*Agent, 0, len(ar.byCapability[capability]))
	for _, agent := range ar.byCapability[capability] {
		if agent.Status == "online" {
			agents = append(agents, agent)
		}
	}
	return agents
}

// ListAgents returns all registered agents
func (ar *AgentRegistry) ListAgents() []*Agent {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	agents := make([]*Agent, 0, len(ar.agents))
	for _, agent := range ar.agents {
		agents = append(agents, agent)
	}
	return agents
}

// UpdateAgentStatus updates an agent's status
func (ar *AgentRegistry) UpdateAgentStatus(id string, status string) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	agent, ok := ar.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.Status = status
	agent.LastSeen = time.Now()

	ar.logger.Debug("updated agent status",
		slog.String("id", id),
		slog.String("status", status),
	)

	return nil
}

// RemoveAgent removes an agent from registry
func (ar *AgentRegistry) RemoveAgent(id string) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	agent, ok := ar.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	delete(ar.agents, id)

	// Remove from capability index
	for _, cap := range agent.Capabilities {
		filtered := make([]*Agent, 0)
		for _, a := range ar.byCapability[cap] {
			if a.ID != id {
				filtered = append(filtered, a)
			}
		}
		ar.byCapability[cap] = filtered
	}

	return nil
}

// HealthCheck checks if an agent is healthy
func (ar *AgentRegistry) HealthCheck(id string) error {
	agent := ar.GetAgent(id)
	if agent == nil {
		return fmt.Errorf("agent not found: %s", id)
	}

	// Would make HTTP request to agent endpoint in production
	ar.UpdateAgentStatus(id, "online")

	return nil
}

// GetStats returns registry statistics
func (ar *AgentRegistry) GetStats() map[string]interface{} {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	online := 0
	for _, agent := range ar.agents {
		if agent.Status == "online" {
			online++
		}
	}

	return map[string]interface{}{
		"total_agents":    len(ar.agents),
		"online_agents":   online,
		"capabilities":    len(ar.byCapability),
	}
}
