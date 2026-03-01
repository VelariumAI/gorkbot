package router

import (
	"math/rand"
	"strings"
	"sync"
	"time"
)

// QueryCategory classifies incoming prompts for adaptive routing decisions.
type QueryCategory string

const (
	QueryCategoryCoding    QueryCategory = "coding"
	QueryCategoryCreative  QueryCategory = "creative"
	QueryCategoryReasoning QueryCategory = "reasoning"
	QueryCategoryGeneral   QueryCategory = "general"
)

// RouteOutcome records the result of a routing decision for feedback learning.
type RouteOutcome struct {
	Category  QueryCategory
	ModelUsed string
	Score     float64 // 0.0–1.0 satisfaction / quality signal
	Timestamp time.Time
}

// AdaptiveRouter adjusts model selection weights based on historical performance.
// It uses weighted-random selection to balance exploration vs. exploitation.
type AdaptiveRouter struct {
	mu         sync.RWMutex
	outcomes   []RouteOutcome
	weights    map[QueryCategory]map[string]float64
	maxHistory int
}

// NewAdaptiveRouter creates an AdaptiveRouter with a rolling history cap.
func NewAdaptiveRouter(maxHistory int) *AdaptiveRouter {
	if maxHistory <= 0 {
		maxHistory = 200
	}
	return &AdaptiveRouter{
		outcomes:   make([]RouteOutcome, 0, maxHistory),
		weights:    make(map[QueryCategory]map[string]float64),
		maxHistory: maxHistory,
	}
}

// SuggestModel returns the statistically preferred model for a query category.
// Returns "" when there is insufficient history (caller should use its default routing).
func (ar *AdaptiveRouter) SuggestModel(category QueryCategory) string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	w, ok := ar.weights[category]
	if !ok || len(w) == 0 {
		return ""
	}

	// Weighted-random selection balances exploitation of high-performers with
	// enough exploration to keep learning.
	total := 0.0
	for _, weight := range w {
		total += weight
	}
	if total == 0 {
		return ""
	}

	r := rand.Float64() * total
	cumulative := 0.0
	for model, weight := range w {
		cumulative += weight
		if r <= cumulative {
			return model
		}
	}
	return ""
}

// RecordOutcome records feedback after a model handled a query.
func (ar *AdaptiveRouter) RecordOutcome(category QueryCategory, modelUsed string, score float64) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	ar.outcomes = append(ar.outcomes, RouteOutcome{
		Category:  category,
		ModelUsed: modelUsed,
		Score:     score,
		Timestamp: time.Now(),
	})
	// Enforce rolling window.
	if len(ar.outcomes) > ar.maxHistory {
		ar.outcomes = ar.outcomes[len(ar.outcomes)-ar.maxHistory:]
	}
	ar.recomputeWeights(category)
}

// recomputeWeights updates routing weights for a category from accumulated outcomes.
// Must be called with ar.mu held for writing.
func (ar *AdaptiveRouter) recomputeWeights(category QueryCategory) {
	scores := make(map[string][]float64)
	for _, o := range ar.outcomes {
		if o.Category == category {
			scores[o.ModelUsed] = append(scores[o.ModelUsed], o.Score)
		}
	}
	if len(scores) == 0 {
		return
	}

	totals := make(map[string]float64)
	grand := 0.0
	for model, ss := range scores {
		avg := 0.0
		for _, s := range ss {
			avg += s
		}
		avg = avg/float64(len(ss)) + 0.05 // epsilon prevents zero weight
		totals[model] = avg
		grand += avg
	}
	if grand == 0 {
		return
	}

	if ar.weights[category] == nil {
		ar.weights[category] = make(map[string]float64)
	}
	for model, total := range totals {
		ar.weights[category][model] = total / grand
	}
}

// ClassifyQuery heuristically maps a prompt to a QueryCategory.
func ClassifyQuery(prompt string) QueryCategory {
	lower := strings.ToLower(prompt)
	for _, kw := range []string{"code", "bug", "function", "implement", "debug", "compile", "test", "error", "refactor"} {
		if strings.Contains(lower, kw) {
			return QueryCategoryCoding
		}
	}
	for _, kw := range []string{"explain", "why", "analyze", "reason", "think", "strategy", "plan", "evaluate"} {
		if strings.Contains(lower, kw) {
			return QueryCategoryReasoning
		}
	}
	for _, kw := range []string{"write", "story", "poem", "create", "design", "imagine", "generate", "draft"} {
		if strings.Contains(lower, kw) {
			return QueryCategoryCreative
		}
	}
	return QueryCategoryGeneral
}
