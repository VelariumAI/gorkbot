package router

import (
	"github.com/velariumai/gorkbot/pkg/registry"
)

// RouteRequest encapsulates the parameters needed to select a model
type RouteRequest struct {
	// The user's input prompt
	Prompt string

	// Estimated number of tokens in the prompt + conversation history
	ContextSize int

	// Specific capabilities required for this task
	RequiresVision bool
	RequiresTools  bool
	RequiresJSON   bool

	// Role defines the intended use (Primary interaction vs Consultant)
	Role ModelRole

	// Optional: Preference for speed vs quality (0.0 = Speed, 1.0 = Quality)
	// If nil/default, router decides based on complexity heuristics.
	QualityPreference float64
}

// ModelRole defines the intended usage pattern
type ModelRole int

const (
	RolePrimary    ModelRole = iota // Interactive, chat, user-facing
	RoleConsultant                  // Background reasoning, deep analysis, critique
	RoleTool                        // Specific tool execution
)

// RouteDecision represents the result of the routing process
type RouteDecision struct {
	// The best matching model for the task
	Primary registry.ModelDefinition

	// A suitable backup if the primary fails
	Fallback *registry.ModelDefinition

	// Human-readable explanation of why this route was chosen
	Reasoning string

	// The "Tier" assigned to this request (e.g., "Fast", "Reasoning", "HighContext")
	RoutingTier string
}

// SystemConfiguration represents the selected model pair for the application lifecycle
type SystemConfiguration struct {
	PrimaryModel    registry.ModelDefinition // Best available primary model
	SpecialistModel registry.ModelDefinition // Best available specialist/consultant model
	Reasoning       string
}

// Feedback represents the outcome of a routing decision, used for future learning
type Feedback struct {
	TaskID     string
	Route      RouteDecision
	Success    bool
	UserRating int   // 1-5
	Latency    int64 // ms
	Error      string
}
