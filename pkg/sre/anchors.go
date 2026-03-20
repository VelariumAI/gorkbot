package sre

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// AnchorPhase labels which reasoning phase an anchor belongs to.
type AnchorPhase string

const (
	AnchorPhaseGround     AnchorPhase = "ground"
	AnchorPhaseHypothesis AnchorPhase = "hypothesis"
	AnchorPhasePrune      AnchorPhase = "prune"
	AnchorPhaseConverge   AnchorPhase = "converge"
)

// PhaseFromSRE maps SREPhase int to AnchorPhase string.
func PhaseFromSRE(p SREPhase) AnchorPhase {
	switch p {
	case SREPhaseHypothesis:
		return AnchorPhaseHypothesis
	case SREPhasePrune:
		return AnchorPhasePrune
	case SREPhaseConverge:
		return AnchorPhaseConverge
	default:
		return AnchorPhaseGround
	}
}

// Anchor is a pinned semantic fact in the working memory.
type Anchor struct {
	Key       string
	Content   string
	Phase     AnchorPhase
	Priority  float64
	CreatedAt time.Time
}

// AnchorLayer is a phase-aware working memory built on top of sense.AgeMem.
// Supports per-phase snapshots to enable backtracking when CorrectionEngine fires.
type AnchorLayer struct {
	ageMem    *sense.AgeMem
	mu        sync.RWMutex
	anchors   []*Anchor
	snapshots map[AnchorPhase][]*Anchor
	logger    *slog.Logger
}

func NewAnchorLayer(ageMem *sense.AgeMem, logger *slog.Logger) *AnchorLayer {
	if logger == nil {
		logger = slog.Default()
	}
	return &AnchorLayer{
		ageMem:    ageMem,
		anchors:   []*Anchor{},
		snapshots: make(map[AnchorPhase][]*Anchor),
		logger:    logger,
	}
}

// Add stores key/content for the given phase.
// Writes to AgeMem STM always; LTM if priority >= 0.8.
func (al *AnchorLayer) Add(key, content string, phase AnchorPhase, priority float64) {
	al.mu.Lock()
	defer al.mu.Unlock()

	anchor := &Anchor{
		Key:       key,
		Content:   content,
		Phase:     phase,
		Priority:  priority,
		CreatedAt: time.Now(),
	}
	al.anchors = append(al.anchors, anchor)

	// Write to AgeMem if available
	if al.ageMem != nil {
		memContent := fmt.Sprintf("[%s] %s: %s", phase, key, content)
		persist := priority >= 0.8
		al.ageMem.Store(key, memContent, priority, nil, persist)
	}
}

// AddFromWorldModel ingests all WorldModelState fields as priority-1.0 ground anchors.
func (al *AnchorLayer) AddFromWorldModel(ws *WorldModelState) {
	if ws == nil {
		return
	}

	for _, entity := range ws.Entities {
		al.Add("entity_"+entity, entity, AnchorPhaseGround, 1.0)
	}

	for _, constraint := range ws.Constraints {
		al.Add("constraint_"+constraint, constraint, AnchorPhaseGround, 1.0)
	}

	for _, fact := range ws.Facts {
		al.Add("fact_"+fact, fact, AnchorPhaseGround, 1.0)
	}

	for k, v := range ws.Anchors {
		al.Add(k, v, AnchorPhaseGround, 1.0)
	}
}

// Commit snapshots the current anchor list for the given phase.
// Called at each phase transition and at task start (grounding phase).
func (al *AnchorLayer) Commit(phase AnchorPhase) {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Deep copy current anchors
	snapshot := make([]*Anchor, len(al.anchors))
	for i, a := range al.anchors {
		acopy := *a
		snapshot[i] = &acopy
	}
	al.snapshots[phase] = snapshot
}

// Backtrack restores anchor state to the last Commit(toPhase) snapshot.
// Returns false if no snapshot exists for toPhase.
func (al *AnchorLayer) Backtrack(toPhase AnchorPhase) bool {
	al.mu.Lock()
	defer al.mu.Unlock()

	snapshot, ok := al.snapshots[toPhase]
	if !ok {
		return false
	}

	// Restore
	al.anchors = make([]*Anchor, len(snapshot))
	for i, a := range snapshot {
		acopy := *a
		al.anchors[i] = &acopy
	}
	return true
}

func (al *AnchorLayer) List() []*Anchor {
	al.mu.RLock()
	defer al.mu.RUnlock()
	result := make([]*Anchor, len(al.anchors))
	copy(result, al.anchors)
	return result
}

func (al *AnchorLayer) ListByPhase(phase AnchorPhase) []*Anchor {
	al.mu.RLock()
	defer al.mu.RUnlock()
	var result []*Anchor
	for _, a := range al.anchors {
		if a.Phase == phase {
			result = append(result, a)
		}
	}
	return result
}

// FormatBlock returns "[SRE_WORKING_MEMORY]\n..." for UpsertSystemMessage injection.
// maxAnchors=0 → all anchors.
func (al *AnchorLayer) FormatBlock(maxAnchors int) string {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if len(al.anchors) == 0 {
		return "[SRE_WORKING_MEMORY]\n(no anchors)"
	}

	block := "[SRE_WORKING_MEMORY]\n"
	count := len(al.anchors)
	if maxAnchors > 0 && count > maxAnchors {
		count = maxAnchors
	}

	for i := 0; i < count; i++ {
		a := al.anchors[i]
		block += fmt.Sprintf("  [%s] %s: %s\n", a.Phase, a.Key, a.Content)
	}

	if maxAnchors > 0 && len(al.anchors) > maxAnchors {
		block += fmt.Sprintf("  ... and %d more\n", len(al.anchors)-maxAnchors)
	}

	return block
}

// ContentStrings returns all Content values — used by CorrectionEngine for keyword matching.
func (al *AnchorLayer) ContentStrings() []string {
	al.mu.RLock()
	defer al.mu.RUnlock()
	result := make([]string, len(al.anchors))
	for i, a := range al.anchors {
		result[i] = a.Content
	}
	return result
}

// Clear resets all anchors and snapshots. Called at task start.
func (al *AnchorLayer) Clear() {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.anchors = []*Anchor{}
	al.snapshots = make(map[AnchorPhase][]*Anchor)
}
