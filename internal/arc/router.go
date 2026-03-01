package arc

import (
	"sync"
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
)

// RouteDecision is the output of a single routing call.
type RouteDecision struct {
	Classification WorkflowType
	Budget         ResourceBudget
	Timestamp      time.Time
}

// RouterStats tracks aggregate routing history.
type RouterStats struct {
	mu                sync.Mutex
	TotalRouted       int
	DirectCount       int    // Kept for backwards compat — equals CountByClass[WorkflowDirect]
	ReasonVerifyCount int    // Kept for backwards compat
	CountByClass      [workflowClassCount]int
}

func (s *RouterStats) record(wf WorkflowType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalRouted++
	idx := int(wf)
	if idx >= 0 && idx < workflowClassCount {
		s.CountByClass[idx]++
	}
	// Maintain legacy fields for compatibility with existing callers
	switch {
	case wf == WorkflowConversational || wf == WorkflowFactual:
		s.DirectCount++
	default:
		s.ReasonVerifyCount++
	}
}

// ARCRouter routes incoming prompts to the appropriate execution workflow
// and computes platform-aware resource budgets.
type ARCRouter struct {
	classifier   *QueryClassifier
	platform     PlatformClass
	stats        RouterStats
	mu           sync.Mutex
	lastDecision *RouteDecision
}

// NewARCRouter creates a router calibrated to the host platform via HALProfile.
func NewARCRouter(hal platform.HALProfile) *ARCRouter {
	return &ARCRouter{
		classifier: &QueryClassifier{},
		platform:   SystemDetector(hal),
	}
}

// Route classifies the prompt and returns a RouteDecision with resource budget.
func (r *ARCRouter) Route(prompt string) RouteDecision {
	wf := r.classifier.Classify(prompt)
	budget := ComputeBudget(r.platform, wf)
	dec := RouteDecision{
		Classification: wf,
		Budget:         budget,
		Timestamp:      time.Now(),
	}
	r.stats.record(wf)
	r.mu.Lock()
	r.lastDecision = &dec
	r.mu.Unlock()
	return dec
}

// Stats returns a snapshot of aggregate routing statistics.
func (r *ARCRouter) Stats() RouterStats {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	snap := RouterStats{
		TotalRouted:       r.stats.TotalRouted,
		DirectCount:       r.stats.DirectCount,
		ReasonVerifyCount: r.stats.ReasonVerifyCount,
		CountByClass:      r.stats.CountByClass,
	}
	return snap
}

// LastDecision returns a copy of the most recent route decision, or nil if
// no prompts have been routed yet.
func (r *ARCRouter) LastDecision() *RouteDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastDecision == nil {
		return nil
	}
	cp := *r.lastDecision
	return &cp
}

// PlatformName returns a human-readable platform class string.
func (r *ARCRouter) PlatformName() string {
	return r.platform.String()
}
