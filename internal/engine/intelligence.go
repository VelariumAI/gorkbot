package engine

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// IntelligenceLayer bundles the ARC Router, MEL Meta-Experience Learning
// system, and a RoutingTable fallback. It provides routing decisions and
// heuristic injection without modifying core orchestrator logic.
type IntelligenceLayer struct {
	Router   *adaptive.ARCRouter
	Store    *adaptive.VectorStore
	Analyzer *adaptive.BifurcationAnalyzer
	Reframer *adaptive.ReframedEvaluator
	// FallbackRouting provides pattern-based source→agent routing that fires
	// when ARC has not yet accumulated enough history to be reliable.
	FallbackRouting *adaptive.RoutingTable
}

// NewIntelligenceLayer initializes the full intelligence stack.
// configDir is the gorkbot config directory; the MEL vector store is
// persisted at configDir/vector_store.json.
func NewIntelligenceLayer(hal platform.HALProfile, configDir string) (*IntelligenceLayer, error) {
	storePath := fmt.Sprintf("%s/vector_store.json", configDir)
	store, err := adaptive.NewVectorStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("intelligence layer: vector store: %w", err)
	}

	return &IntelligenceLayer{
		Router:          adaptive.NewARCRouter(hal),
		Store:           store,
		Analyzer:        adaptive.NewBifurcationAnalyzer(store),
		Reframer:        &adaptive.ReframedEvaluator{},
		FallbackRouting: adaptive.NewRoutingTable(),
	}, nil
}

// RouteSource returns the agentID for a given source identifier using the
// RoutingTable fallback. Returns ("", false) if no binding matches.
// This is the pattern-based routing path; ARC handles workflow classification
// separately via Route(prompt).
func (il *IntelligenceLayer) RouteSource(source string) (agentID string, ok bool) {
	if il.FallbackRouting == nil {
		return "", false
	}
	return il.FallbackRouting.Route(source)
}

// Route classifies a prompt and returns its resource budget.
func (il *IntelligenceLayer) Route(prompt string) adaptive.RouteDecision {
	return il.Router.Route(prompt)
}

// HeuristicContext returns MEL heuristics relevant to the prompt as a
// formatted system-prompt section. Returns "" if no relevant heuristics exist.
func (il *IntelligenceLayer) HeuristicContext(prompt string) string {
	heuristics := il.Store.Query(prompt, 5)
	if len(heuristics) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Learned Heuristics (MEL):\n")
	for _, h := range heuristics {
		sb.WriteString("- ")
		sb.WriteString(h.Text())
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// ObserveFailed records a tool failure for bifurcation analysis.
func (il *IntelligenceLayer) ObserveFailed(toolName string, params map[string]interface{}, errMsg string) {
	il.Analyzer.ObserveFailed(toolName, params, errMsg)
}

// ObserveSuccess completes the bifurcation cycle, generating a heuristic
// if parameter differences are found from the prior failure.
func (il *IntelligenceLayer) ObserveSuccess(toolName string, params map[string]interface{}) {
	il.Analyzer.ObserveSuccess(toolName, params)
}

// IsHighRisk returns true if the prompt contains destructive operation keywords.
func (il *IntelligenceLayer) IsHighRisk(prompt string) bool {
	return il.Reframer.IsHighRisk(prompt)
}

// SetEmbedderWithProjection wires an embedder into both the ARCRouter
// classifier and the MEL VectorStore, wrapping it in a VectorProjector
// calibrated to the current HAL RAM profile. Reduces embedding dimensions on
// low-RAM devices without changing the Embedder interface seen by callers.
func (il *IntelligenceLayer) SetEmbedderWithProjection(e embeddings.Embedder, hal platform.HALProfile) {
	projected := adaptive.NewVectorProjector(e, hal)
	il.Router.SetEmbedder(projected)
	il.Store.SetEmbedder(projected)
}
