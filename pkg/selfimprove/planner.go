package selfimprove

import (
	"sync"
)

// ImprovementCandidate represents a potential improvement action.
type ImprovementCandidate struct {
	Source       SignalSource
	Target       string  // action target/name
	BaseScore    float64 // 0.0-1.0
	ModeMultiplier float64 // applied per mode
	RiskFilter   bool    // true if should be filtered out
}

// ImprovementPlanner selects the best improvement action from multiple sources.
type ImprovementPlanner struct {
	mu                sync.RWMutex
	lastActions       []string // last 3 (source+target) pairs for loop guarding
	maxLoopGuardDepth int
}

// NewImprovementPlanner creates a new planner.
func NewImprovementPlanner() *ImprovementPlanner {
	return &ImprovementPlanner{
		lastActions:       make([]string, 0, 3),
		maxLoopGuardDepth: 3,
	}
}

// Select returns the best-scoring improvement candidate from available sources.
// Returns nil if no viable candidate exists.
func (p *ImprovementPlanner) Select(mode EmotionalMode,
	sparkDirectives []string,
	freeWillProposals []FreeWillProposalSummary,
	harnessFailures *HarnessFailureInfo,
	researchDocs int,
) *ImprovementCandidate {

	p.mu.Lock()
	defer p.mu.Unlock()

	candidates := make([]ImprovementCandidate, 0)

	// Source 1: SPARK Directives
	for _, directive := range sparkDirectives {
		baseScore := 0.7 // directives are high-priority
		c := ImprovementCandidate{
			Source:     SourceSPARK,
			Target:     directive,
			BaseScore:  baseScore,
			ModeMultiplier: modeDirectiveMultiplier(mode),
		}
		candidates = append(candidates, c)
	}

	// Source 2: Free Will Proposals
	for _, prop := range freeWillProposals {
		// Score = confidence/100 - risk*0.5
		baseScore := (prop.Confidence / 100.0) - (prop.Risk * 0.5)
		baseScore = clamp(baseScore, 0.0, 1.0)

		// Skip if risk > 0.8
		riskFilter := prop.Risk > 0.8

		c := ImprovementCandidate{
			Source:     SourceFreeWill,
			Target:     prop.Target,
			BaseScore:  baseScore,
			ModeMultiplier: modeProposalMultiplier(mode),
			RiskFilter: riskFilter,
		}
		candidates = append(candidates, c)
	}

	// Source 3: Harness Failures
	if harnessFailures != nil && harnessFailures.FailingCount > 0 {
		failingRatio := float64(harnessFailures.FailingCount) / float64(harnessFailures.TotalCount)
		baseScore := failingRatio * 0.9
		c := ImprovementCandidate{
			Source:     SourceHarness,
			Target:     "harness_repairs",
			BaseScore:  baseScore,
			ModeMultiplier: modeHarnessMultiplier(mode),
		}
		candidates = append(candidates, c)
	}

	// Source 4: Research Buffer (low priority in Urgent mode)
	if mode != ModeUrgent && researchDocs > 0 {
		baseScore := 0.3 * (float64(researchDocs) / 5.0)
		baseScore = clamp(baseScore, 0.0, 1.0)
		c := ImprovementCandidate{
			Source:     SourceResearch,
			Target:     "research_synthesis",
			BaseScore:  baseScore,
			ModeMultiplier: modeResearchMultiplier(mode),
		}
		candidates = append(candidates, c)
	}

	// Score and filter
	best := (*ImprovementCandidate)(nil)
	bestScore := 0.0

	for i, cand := range candidates {
		// Apply risk filter
		if cand.RiskFilter {
			continue
		}

		// Compute final score
		finalScore := cand.BaseScore * cand.ModeMultiplier
		if finalScore < 0.15 {
			// Too low to consider
			continue
		}

		// Check loop guard
		key := cand.Source.String() + ":" + cand.Target
		if p.isRecentAction(key) {
			continue
		}

		// Track best
		if finalScore > bestScore {
			bestScore = finalScore
			best = &candidates[i]
		}
	}

	// Record selected action for loop guard
	if best != nil {
		key := best.Source.String() + ":" + best.Target
		p.recordAction(key)
	}

	return best
}

// Mode multipliers per source
func modeDirectiveMultiplier(mode EmotionalMode) float64 {
	switch mode {
	case ModeCalm:
		return 0.8
	case ModeCurious:
		return 1.0
	case ModeFocused:
		return 1.3
	case ModeUrgent:
		return 1.6
	case ModeRestrained:
		return 0.1
	default:
		return 1.0
	}
}

func modeProposalMultiplier(mode EmotionalMode) float64 {
	switch mode {
	case ModeCalm:
		return 0.8
	case ModeCurious:
		return 1.2
	case ModeFocused:
		return 1.1
	case ModeUrgent:
		return 0.9 // skip proposals when urgent, focus on directives
	case ModeRestrained:
		return 0.1
	default:
		return 1.0
	}
}

func modeHarnessMultiplier(mode EmotionalMode) float64 {
	switch mode {
	case ModeCalm:
		return 0.7
	case ModeCurious:
		return 1.0
	case ModeFocused:
		return 1.2
	case ModeUrgent:
		return 1.4
	case ModeRestrained:
		return 0.1
	default:
		return 1.0
	}
}

func modeResearchMultiplier(mode EmotionalMode) float64 {
	switch mode {
	case ModeCalm:
		return 1.0
	case ModeCurious:
		return 1.3
	case ModeFocused:
		return 1.0
	case ModeUrgent:
		return 0.0 // skip research in urgent mode
	case ModeRestrained:
		return 0.1
	default:
		return 1.0
	}
}

// Loop guard: check if action was recently selected
func (p *ImprovementPlanner) isRecentAction(key string) bool {
	for _, recent := range p.lastActions {
		if recent == key {
			return true
		}
	}
	return false
}

// Record action for future loop guarding
func (p *ImprovementPlanner) recordAction(key string) {
	if len(p.lastActions) >= p.maxLoopGuardDepth {
		// Remove oldest
		p.lastActions = p.lastActions[1:]
	}
	p.lastActions = append(p.lastActions, key)
}

// HarnessFailureInfo provides harness state for scoring.
type HarnessFailureInfo struct {
	FailingCount int
	TotalCount   int
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
