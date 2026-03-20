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
	// PrimaryBiasProvider adds +200 score to this provider for Primary role.
	// "" means no bias (any provider may win).
	PrimaryBiasProvider string
	// ConsultantBiasProvider adds +200 score to this provider for Consultant role.
	// "" means no bias (any provider may win).
	ConsultantBiasProvider string
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

		// Role-Based Provider Bias (configurable; defaults to no bias when fields are "").
		// Biases heavily but does not hard-lock: a vastly superior model from
		// another provider can still win if the score gap is large enough.
		if req.Role == RolePrimary && r.PrimaryBiasProvider != "" && m.Provider == registry.ProviderID(r.PrimaryBiasProvider) {
			score += 200
		}
		if req.Role == RoleConsultant && r.ConsultantBiasProvider != "" && m.Provider == registry.ProviderID(r.ConsultantBiasProvider) {
			score += 200
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

// SelectSystemModels selects a Primary and a Specialist model from whatever
// providers are active. Provider-agnostic: any provider may win.
// If PrimaryBiasProvider / ConsultantBiasProvider are set, those providers
// receive a +200 score bonus but are not required.
func (r *Router) SelectSystemModels() (*SystemConfiguration, error) {
	candidates := r.registry.ListActiveModels()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no active models found in registry")
	}

	type scored struct {
		model registry.ModelDefinition
		score int
	}

	var all []scored
	for _, m := range candidates {
		s := calculateDynamicScore(m)
		if s < 0 {
			continue // filter legacy/low-quality
		}
		// Apply Primary bias for scoring purposes only.
		if r.PrimaryBiasProvider != "" && m.Provider == registry.ProviderID(r.PrimaryBiasProvider) {
			s += 200
		}
		all = append(all, scored{m, s})
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no suitable models found in registry")
	}

	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	primary := all[0]

	// Specialist: best model from a different provider than primary.
	// Apply Consultant bias when searching.
	var specialist *scored
	for i := range all {
		if all[i].model.Provider == primary.model.Provider {
			continue
		}
		s := all[i].score
		if r.ConsultantBiasProvider != "" && all[i].model.Provider == registry.ProviderID(r.ConsultantBiasProvider) {
			s += 200
		}
		if specialist == nil || s > specialist.score {
			candidate := scored{all[i].model, s}
			specialist = &candidate
		}
	}

	// Edge case: only one provider available — use 2nd model of same provider.
	if specialist == nil && len(all) > 1 {
		specialist = &all[1]
	}
	if specialist == nil {
		specialist = &primary // last resort: same model for both roles
	}

	return &SystemConfiguration{
		PrimaryModel:    primary.model,
		SpecialistModel: specialist.model,
		Reasoning: fmt.Sprintf("Selected Primary: %s/%s (score %d), Specialist: %s/%s (score %d)",
			primary.model.Provider, primary.model.Name, primary.score,
			specialist.model.Provider, specialist.model.Name, specialist.score),
	}, nil
}
