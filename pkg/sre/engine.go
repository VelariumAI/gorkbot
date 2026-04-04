package sre

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/spark"
)

// ── CoSEngine ────────────────────────────────────────────────────────────────

// CoSEngine implements the Step-wise Reasoning phase gate.
// The SPARK DriveScore quality-gates phase advance:
// if DriveScore < 0.3 during PhasePrune, extends Prune by up to 2 extra turns.
type CoSEngine struct {
	mu              sync.RWMutex
	phase           SREPhase
	HypothesisTurns int          // default 3
	PruneTurns      int          // default 6
	spark           *spark.SPARK // nil-safe; used for DriveScore quality gate
	onTransition    func(from, to SREPhase)
	extendedPrune   int // 0–2 extra turns added
}

func newCoSEngine(hypothesisTurns, pruneTurns int, sparkInst *spark.SPARK) *CoSEngine {
	if hypothesisTurns <= 0 {
		hypothesisTurns = 3
	}
	if pruneTurns <= 0 {
		pruneTurns = 6
	}
	return &CoSEngine{
		phase:           SREPhaseHypothesis,
		HypothesisTurns: hypothesisTurns,
		PruneTurns:      pruneTurns,
		spark:           sparkInst,
		onTransition:    nil,
		extendedPrune:   0,
	}
}

// Tick computes the phase from the turn counter.
// Fires onTransition callback on boundary cross.
// Quality gate: if sparkInst != nil && sparkInst.DriveScore() < 0.3 during PhasePrune,
// extends PruneTurns by up to 2 before advancing to Converge.
func (e *CoSEngine) Tick(turn int) SREPhase {
	e.mu.Lock()
	defer e.mu.Unlock()

	var newPhase SREPhase

	if turn <= e.HypothesisTurns {
		newPhase = SREPhaseHypothesis
	} else if turn <= e.HypothesisTurns+e.PruneTurns+e.extendedPrune {
		newPhase = SREPhasePrune

		// Quality gate: check if we should extend prune
		if e.spark != nil && e.extendedPrune < 2 {
			if e.spark.DriveScore() < 0.3 {
				e.extendedPrune++
				// Stay in prune, do not transition
			}
		}
	} else {
		newPhase = SREPhaseConverge
	}

	// Fire transition callback if phase changed
	if newPhase != e.phase {
		oldPhase := e.phase
		e.phase = newPhase
		if e.onTransition != nil {
			go e.onTransition(oldPhase, newPhase)
		}
	}

	return e.phase
}

func (e *CoSEngine) CurrentPhase() SREPhase {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.phase
}

func (e *CoSEngine) PhaseLabel() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.phase.Label()
}

func (e *CoSEngine) RoleBlock() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.phase.RoleInstruction()
}

func (e *CoSEngine) OnTransition(fn func(from, to SREPhase)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onTransition = fn
}

func (e *CoSEngine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.phase = SREPhaseHypothesis
	e.extendedPrune = 0
}

// ── CorrectionEngine ─────────────────────────────────────────────────────────

// CorrectionEngine detects response deviations from working memory anchors.
// Pushes PhaseDeviation debt to SPARK IDL when ShouldBacktrack() is triggered.
type CorrectionEngine struct {
	spark       *spark.SPARK  // nil-safe; for RecordPhaseDeviation
	sense       SENSEProvider // nil-safe; for LogSRECorrection
	threshold   float64       // Jaccard overlap floor (default 0.30)
	consecutive int
	mu          sync.Mutex
	logger      *slog.Logger
}

func newCorrectionEngine(sparkInst *spark.SPARK, sense SENSEProvider, threshold float64, logger *slog.Logger) *CorrectionEngine {
	if threshold <= 0 {
		threshold = 0.30
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CorrectionEngine{
		spark:       sparkInst,
		sense:       sense,
		threshold:   threshold,
		consecutive: 0,
		logger:      logger,
	}
}

// Check evaluates whether response deviates from current anchors.
// Uses tokenized Jaccard overlap of response vs all anchor ContentStrings().
// Deviation flagged when: overlap < threshold AND len(anchorContent) >= 2.
// Returns (triggered bool, reason string).
func (ce *CorrectionEngine) Check(response string, anchorContents []string) (bool, string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if len(anchorContents) < 2 {
		ce.consecutive = 0
		return false, ""
	}

	responseTokens := tokenize(response)
	if len(responseTokens) == 0 {
		ce.consecutive++
		return true, "empty_response"
	}

	// Compute Jaccard overlap with all anchor content
	allAnchorTokens := make(map[string]struct{})
	for _, ac := range anchorContents {
		tokens := tokenize(ac)
		for t := range tokens {
			allAnchorTokens[t] = struct{}{}
		}
	}

	overlap := jaccardSets(responseTokens, allAnchorTokens)

	if overlap < ce.threshold {
		ce.consecutive++
		reason := "low_anchor_overlap"
		return true, reason
	}

	ce.consecutive = 0
	return false, ""
}

// ShouldBacktrack returns true after >= 2 consecutive Check() positives.
func (ce *CorrectionEngine) ShouldBacktrack() bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	return ce.consecutive >= 2
}

// Backtrack triggers backtrack: pushes IDL debt + logs SENSE event.
// Returns the phase to revert to (AnchorPhaseHypothesis).
func (ce *CorrectionEngine) Backtrack(currentPhase string) AnchorPhase {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.spark != nil {
		reason := "low_anchor_overlap"
		if ce.consecutive > 2 {
			reason = "sustained_deviation"
		}
		ce.spark.RecordPhaseDeviation(currentPhase, reason, 0.6)
	}

	if ce.sense != nil {
		ce.sense.LogSRECorrection("sustained_anchor_deviation", "hypothesis")
	}

	ce.consecutive = 0
	return AnchorPhaseHypothesis
}

// Reset clears the consecutive counter. Called on each phase advance.
func (ce *CorrectionEngine) Reset() {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.consecutive = 0
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func tokenize(s string) map[string]struct{} {
	// Lowercase, split on non-alphanumeric, collect unique tokens
	tokens := make(map[string]struct{})
	s = strings.ToLower(s)

	var current strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			current.WriteRune(ch)
		} else {
			if current.Len() > 0 {
				tokens[current.String()] = struct{}{}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens[current.String()] = struct{}{}
	}

	return tokens
}

func jaccardSets(a, b map[string]struct{}) float64 {
	// Compute |A∩B|/|A∪B|
	intersection := 0
	for token := range a {
		if _, ok := b[token]; ok {
			intersection++
		}
	}

	union := len(a)
	for token := range b {
		if _, ok := a[token]; !ok {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
