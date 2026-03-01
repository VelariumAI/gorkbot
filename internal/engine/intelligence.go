package engine

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/internal/arc"
	"github.com/velariumai/gorkbot/internal/mel"
	"github.com/velariumai/gorkbot/internal/platform"
)

// IntelligenceLayer bundles the ARC Router and MEL Meta-Experience Learning
// system. It provides routing decisions and heuristic injection without
// modifying core orchestrator logic beyond three call sites.
type IntelligenceLayer struct {
	Router   *arc.ARCRouter
	Store    *mel.VectorStore
	Analyzer *mel.BifurcationAnalyzer
	Reframer *arc.ReframedEvaluator
}

// NewIntelligenceLayer initializes the full intelligence stack.
// configDir is the gorkbot config directory; the MEL vector store is
// persisted at configDir/vector_store.json.
func NewIntelligenceLayer(hal platform.HALProfile, configDir string) (*IntelligenceLayer, error) {
	storePath := fmt.Sprintf("%s/vector_store.json", configDir)
	store, err := mel.NewVectorStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("intelligence layer: vector store: %w", err)
	}

	return &IntelligenceLayer{
		Router:   arc.NewARCRouter(hal),
		Store:    store,
		Analyzer: mel.NewBifurcationAnalyzer(store),
		Reframer: &arc.ReframedEvaluator{},
	}, nil
}

// Route classifies a prompt and returns its resource budget.
func (il *IntelligenceLayer) Route(prompt string) arc.RouteDecision {
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
