package discovery

import "time"

// Provider constants for the discovery manager.
const (
	ProviderXAI       = "xai"
	ProviderGoogle    = "google"
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderMiniMax    = "minimax"
	ProviderOpenRouter = "openrouter"
)

// CapabilityClass classifies a model's primary strength based on its ID keywords.
type CapabilityClass int

const (
	CapGeneral   CapabilityClass = iota
	CapReasoning                 // "reasoning", "thinking", "pro", o-series
	CapSpeed                     // "fast", "flash", "lite", "haiku", "mini"
	CapCoding                    // "code", "vision", "gpt-4o"
)

func (c CapabilityClass) String() string {
	switch c {
	case CapReasoning:
		return "reasoning"
	case CapSpeed:
		return "speed"
	case CapCoding:
		return "coding"
	default:
		return "general"
	}
}

// DiscoveredModel is a live model entry returned from API polling.
type DiscoveredModel struct {
	ID           string
	Name         string
	Provider     string // "xai", "google", "anthropic", "openai", or "minimax"
	Caps         []CapabilityClass
	DiscoveredAt time.Time
}

// HasCap reports whether the model has the given capability class.
func (d *DiscoveredModel) HasCap(c CapabilityClass) bool {
	for _, cap := range d.Caps {
		if cap == c {
			return true
		}
	}
	return false
}

// BestCap returns the model's highest-priority capability class.
func (d *DiscoveredModel) BestCap() CapabilityClass {
	for _, prio := range []CapabilityClass{CapReasoning, CapCoding, CapSpeed} {
		if d.HasCap(prio) {
			return prio
		}
	}
	return CapGeneral
}

// CapIcon returns a compact icon for display.
func (c CapabilityClass) Icon() string {
	switch c {
	case CapReasoning:
		return "🧠"
	case CapSpeed:
		return "⚡"
	case CapCoding:
		return "💻"
	default:
		return "✦"
	}
}

// AgentNode is a live sub-agent entry used to build the hierarchical TUI tree.
type AgentNode struct {
	ID        string
	Task      string // short task description
	ModelID   string // which model is handling this node
	Depth     int    // 0 = root; max 4
	Status    string // "running", "verifying", "done", "failed"
	StartedAt time.Time
	Children  []*AgentNode
}

// StatusIcon returns a compact icon for agent status.
func (a *AgentNode) StatusIcon() string {
	switch a.Status {
	case "running":
		return "⟳"
	case "verifying":
		return "✔?"
	case "done":
		return "✓"
	case "failed":
		return "✗"
	default:
		return "?"
	}
}
