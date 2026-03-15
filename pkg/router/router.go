package router

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// Router selects the appropriate model for a task
type Router struct {
	registry *registry.ModelRegistry
	logger   *slog.Logger
}

// NewRouter creates a new router instance
func NewRouter(reg *registry.ModelRegistry, logger *slog.Logger) *Router {
	return &Router{
		registry: reg,
		logger:   logger,
	}
}

// SelectModel determines the best model for the given request
func (r *Router) SelectModel(req RouteRequest) (*RouteDecision, error) {
	// 1. Get all candidates
	candidates := r.registry.ListActiveModels()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no active models found in registry")
	}

	// 2. Determine Task Needs
	complexity := analyzePromptComplexity(req.Prompt)

	// Override complexity if quality preference is explicit
	if req.QualityPreference > 0.8 {
		complexity = ComplexityAdvanced
	}

	// 3. Score Candidates
	type scoredModel struct {
		model registry.ModelDefinition
		score int
		tier  ModelTier
	}

	var scoredCandidates []scoredModel

	for _, m := range candidates {
		// Start with the dynamic capability-based score
		score := calculateDynamicScore(m)
		tier := classifyModelTier(m)

		// -- Filter: Hard Constraints --

		// Context Window
		if req.ContextSize > m.Capabilities.MaxContextTokens {
			continue
		}

		// Capabilities
		if req.RequiresVision && !m.Capabilities.SupportsVision {
			continue
		}
		if req.RequiresTools && !m.Capabilities.SupportsTools {
			continue
		}
		if req.RequiresJSON && !m.Capabilities.SupportsJSONMode {
			continue
		}

		// -- Scoring: Contextual Bias --

		// Role-Based Provider Bias (The "Intelligent Selection")
		// We bias heavily but don't hard-lock, allowing a vastly superior model
		// from another provider to win if the score gap is huge.
		if req.Role == RolePrimary && m.Provider == "xai" {
			score += 200 // Strong preference for Grok as Primary
		}
		if req.Role == RoleConsultant && m.Provider == "google" {
			score += 200 // Strong preference for Gemini as Consultant
		}

		// Complexity Matching
		// Penalize "Fast" models for "Advanced" tasks
		if complexity == ComplexityAdvanced && tier == TierFast {
			score -= 150
		}
		// Penalize "Reasoning" models for "Simple" tasks (efficiency)
		if complexity == ComplexitySimple && tier == TierReasoning {
			score -= 50
		}

		// Low-End Filter (Soft Penalty)
		// Instead of hard filtering "nano/micro", we nuke their score
		// so they are only picked if they are the ONLY valid option.
		id := strings.ToLower(string(m.ID))
		if strings.Contains(id, "nano") ||
			strings.Contains(id, "micro") ||
			strings.Contains(id, "8b") ||
			strings.Contains(id, "1b") ||
			strings.Contains(id, "2b") {
			score -= 500
		}

		// Deprecated/Legacy Filter
		if strings.Contains(id, "001") || strings.Contains(id, "1.0") {
			score -= 300
		}

		scoredCandidates = append(scoredCandidates, scoredModel{m, score, tier})
	}

	if len(scoredCandidates) == 0 {
		return nil, fmt.Errorf("no models satisfy requirements (Context: %d, Vision: %v, Tools: %v)",
			req.ContextSize, req.RequiresVision, req.RequiresTools)
	}

	// 4. Sort (Highest Score First)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	// 5. Build Decision
	best := scoredCandidates[0]
	var fallback *registry.ModelDefinition

	if len(scoredCandidates) > 1 {
		fallback = &scoredCandidates[1].model
	}

	// Explain logic
	tierName := "Standard"
	switch best.tier {
	case TierFast:
		tierName = "Fast"
	case TierReasoning:
		tierName = "Reasoning"
	}

	compName := "Standard"
	switch complexity {
	case ComplexitySimple:
		compName = "Simple"
	case ComplexityAdvanced:
		compName = "Advanced"
	}

	reasoning := fmt.Sprintf("Task '%s'. Selected '%s' (%s) score=%d. (Role Bias: %v)",
		compName, best.model.Name, tierName, best.score, req.Role)

	return &RouteDecision{
		Primary:     best.model,
		Fallback:    fallback,
		Reasoning:   reasoning,
		RoutingTier: tierName,
	}, nil
}

// SelectSystemModels performs the strict 2-step selection process
// Step 1: Select best Grok (xAI) model for Primary role (Interaction)
// Step 2: Select best Gemini (Google) model for Specialist role (Reasoning/Context)
func (r *Router) SelectSystemModels() (*SystemConfiguration, error) {
	// 1. Discovery & Grading
	candidates := r.registry.ListActiveModels()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no active models found in registry")
	}

	type candidate struct {
		model registry.ModelDefinition
		score int
	}

	var grokCandidates []candidate
	var geminiCandidates []candidate

	for _, m := range candidates {
		// Grade every model
		score := calculateDynamicScore(m)

		// Filter out low-quality/legacy models immediately for system selection
		if score < 0 {
			continue
		}

		// Sort into buckets
		if m.Provider == "xai" {
			grokCandidates = append(grokCandidates, candidate{m, score})
		} else if m.Provider == "google" {
			geminiCandidates = append(geminiCandidates, candidate{m, score})
		}
	}

	// 2. Selection Step 1: Best Grok for Primary
	if len(grokCandidates) == 0 {
		return nil, fmt.Errorf("no xAI (Grok) models available for Primary role")
	}

	// Sort by score descending
	sort.Slice(grokCandidates, func(i, j int) bool {
		return grokCandidates[i].score > grokCandidates[j].score
	})
	primary := grokCandidates[0].model

	// 3. Selection Step 2: Best Gemini for Specialist
	if len(geminiCandidates) == 0 {
		return nil, fmt.Errorf("no Google (Gemini) models available for Specialist role")
	}

	// For Specialist, we prioritize score heavily (Reasoning/Context)
	sort.Slice(geminiCandidates, func(i, j int) bool {
		return geminiCandidates[i].score > geminiCandidates[j].score
	})
	specialist := geminiCandidates[0].model

	return &SystemConfiguration{
		PrimaryModel:    primary,
		SpecialistModel: specialist,
		Reasoning: fmt.Sprintf("Selected Primary: %s (Score: %d), Specialist: %s (Score: %d)",
			primary.Name, grokCandidates[0].score, specialist.Name, geminiCandidates[0].score),
	}, nil
}
