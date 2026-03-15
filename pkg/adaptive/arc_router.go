package adaptive

import (
	"sync"
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// RouteDecision is the output of a single routing call.
type RouteDecision struct {
	Classification WorkflowType
	Budget         ResourceBudget
	Timestamp      time.Time
	// Confidence is the normalised separation between the top-scoring class and
	// the second-best class (0.0 = tie/random, 1.0 = unambiguous winner).
	// Values below 0.25 indicate the router was uncertain and the classification
	// may be less reliable.
	Confidence float64
	// LowConfidence is true when the classifier score margin was too narrow to
	// be reliable. In this case Classification has been conservatively escalated
	// to at least WorkflowAnalytical so the orchestrator uses richer reasoning.
	LowConfidence bool
}

// RouterStatsSnapshot is a lock-free point-in-time copy of routing statistics
// safe to return by value and read without holding any lock.
type RouterStatsSnapshot struct {
	TotalRouted       int
	DirectCount       int
	ReasonVerifyCount int
	CountByClass      [workflowClassCount]int
}

// RouterStats tracks aggregate routing history (internal — contains a mutex).
type RouterStats struct {
	mu                sync.Mutex
	TotalRouted       int
	DirectCount       int // Kept for backwards compat — equals CountByClass[WorkflowDirect]
	ReasonVerifyCount int // Kept for backwards compat
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
	classifier   *SemanticClassifier
	platform     PlatformClass
	stats        RouterStats
	mu           sync.Mutex
	lastDecision *RouteDecision
}

// NewARCRouter creates a router calibrated to the host platform via HALProfile.
func NewARCRouter(hal platform.HALProfile) *ARCRouter {
	return &ARCRouter{
		classifier: &SemanticClassifier{},
		platform:   SystemDetector(hal),
	}
}

// SetEmbedder wires an embedder into the classifier so routing uses semantic
// nearest-neighbour cosine similarity instead of the keyword heuristic.
func (r *ARCRouter) SetEmbedder(e embeddings.Embedder) {
	r.classifier.SetEmbedder(e)
}

// EmbedderName returns the active embedder's name, or a fallback description.
func (r *ARCRouter) EmbedderName() string {
	return r.classifier.EmbedderName()
}

// Route classifies the prompt and returns a RouteDecision with resource budget.
// It includes an entropy guard: when the top-class score is not clearly
// dominant (confidence < 0.25), the classification is conservatively escalated
// to at least WorkflowAnalytical and LowConfidence is set to true.
func (r *ARCRouter) Route(prompt string) RouteDecision {
	wf, conf := r.classifier.ClassifyWithConfidence(prompt)

	lowConf := false
	if conf < 0.25 {
		lowConf = true
		// Conservative escalation — never silently downgrade uncertain prompts
		// to Conversational or Factual where the model uses less reasoning depth.
		if wf == WorkflowConversational || wf == WorkflowFactual {
			wf = WorkflowAnalytical
		}
	}

	budget := ComputeBudget(r.platform, wf)
	dec := RouteDecision{
		Classification: wf,
		Budget:         budget,
		Timestamp:      time.Now(),
		Confidence:     conf,
		LowConfidence:  lowConf,
	}
	r.stats.record(wf)
	r.mu.Lock()
	r.lastDecision = &dec
	r.mu.Unlock()
	return dec
}

// Stats returns a lock-free snapshot of aggregate routing statistics.
func (r *ARCRouter) Stats() RouterStatsSnapshot {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	return RouterStatsSnapshot{
		TotalRouted:       r.stats.TotalRouted,
		DirectCount:       r.stats.DirectCount,
		ReasonVerifyCount: r.stats.ReasonVerifyCount,
		CountByClass:      r.stats.CountByClass,
	}
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
