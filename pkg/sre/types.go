package sre

import (
	"fmt"
	"time"
)

// SREVersion is the semantic version of the Step-wise Reasoning Engine.
//
// History:
//
//	1.0.0 — initial release (v5.2.0): CoS (Chain-of-Thought) engine with HYPOTHESIS/PRUNE/CONVERGE phases,
//	         GroundingExtractor (semantic fact extraction), AnchorLayer (working memory),
//	         EnsembleManager (multi-trajectory reasoning), CorrectionEngine (deviation detection),
//	         TUI integration with authoritative status line showing phase progress and SRE labels.
const SREVersion = "1.0.0"

// SREPhase is the current step-wise reasoning phase.
type SREPhase int

const (
	SREPhaseHypothesis SREPhase = iota // turns 1–HypothesisTurns: broad exploration
	SREPhasePrune                      // next PruneTurns turns: critique + counterfactual
	SREPhaseConverge                   // remaining turns: synthesis + commit
)

func (p SREPhase) String() string {
	switch p {
	case SREPhaseHypothesis:
		return "HYPOTHESIS"
	case SREPhasePrune:
		return "PRUNE"
	case SREPhaseConverge:
		return "CONVERGE"
	default:
		return "UNKNOWN"
	}
}

func (p SREPhase) Label() string {
	return fmt.Sprintf("[SRE: %s]", p.String())
}

func (p SREPhase) RoleInstruction() string {
	switch p {
	case SREPhaseHypothesis:
		return `[SRE: HYPOTHESIS PHASE]
Generate multiple competing hypotheses and approaches. Do NOT commit to one path yet.
Identify unknowns. Explore tool options without destructive operations.
Reference grounding anchors for factual bounds.`

	case SREPhasePrune:
		return `[SRE: PRUNE PHASE]
Evaluate all hypotheses. Select the approach satisfying ALL constraints.
Counterfactual check: "What if assumption X is wrong?"
Execute verification tools. Reference working memory anchors. Self-correct if
prior reasoning deviates from established facts.`

	case SREPhaseConverge:
		return `[SRE: CONVERGE PHASE]
Execute the best-validated approach. Verify outputs against all anchors and constraints.
Produce consolidated structured output. Flag unresolved items explicitly.
Do not introduce new hypotheses.`

	default:
		return ""
	}
}

// SREConfig holds runtime options for the Step-wise Reasoning Engine.
type SREConfig struct {
	EnsembleEnabled  bool
	CoSEnabled       bool
	GroundingEnabled bool
	HypothesisTurns  int     // 0 → default 3
	PruneTurns       int     // 0 → default 6
	CorrectionThresh float64 // Jaccard floor; 0 → default 0.30
}

// WorldModelState is the structured semantic grounding of a task prompt.
// Produced by GroundingExtractor before any planning or tool use.
type WorldModelState struct {
	Entities    []string          `json:"entities"`
	Constraints []string          `json:"constraints"`
	Facts       []string          `json:"facts"`
	Anchors     map[string]string `json:"anchors"`
	Confidence  float64           `json:"confidence"`
	GroundedAt  time.Time         `json:"grounded_at"`
}

// FormatBlock returns "[SRE_GROUNDING]\nEntities: ..."
func (w *WorldModelState) FormatBlock() string {
	if w == nil {
		return ""
	}
	block := fmt.Sprintf("[SRE_GROUNDING] Confidence: %.2f\n", w.Confidence)

	if len(w.Entities) > 0 {
		block += fmt.Sprintf("Entities: %v\n", w.Entities)
	}
	if len(w.Constraints) > 0 {
		block += fmt.Sprintf("Constraints: %v\n", w.Constraints)
	}
	if len(w.Facts) > 0 {
		block += fmt.Sprintf("Facts: %v\n", w.Facts)
	}
	if len(w.Anchors) > 0 {
		block += "Anchors:\n"
		for k, v := range w.Anchors {
			block += fmt.Sprintf("  %s: %s\n", k, v)
		}
	}

	return block
}

// ToMemoryContent returns compact JSON for AgeMem storage
func (w *WorldModelState) ToMemoryContent() string {
	if w == nil {
		return ""
	}
	return fmt.Sprintf("WorldModel: %d entities, %d constraints, %d facts, confidence=%.2f",
		len(w.Entities), len(w.Constraints), len(w.Facts), w.Confidence)
}

// SENSEProvider is the minimal interface to *sense.SENSETracer needed by SRE.
// Satisfied automatically by *sense.SENSETracer after adding 4 LogSREXxx methods.
type SENSEProvider interface {
	LogSREGrounding(confidence float64, entityCount, factCount int)
	LogSREPhase(fromPhase, toPhase string, turn int)
	LogSRECorrection(reason, revertPhase string)
	LogSREEnsemble(conflictCount int, confidence float64)
}
