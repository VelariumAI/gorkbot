package spark

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// ResearchModule manages a priority-queued set of research objectives.
// Objectives can be generated heuristically or (optionally) via an LLM.
type ResearchModule struct {
	mu       sync.Mutex
	queue    objectiveHeap
	maxSize  int
	seenIDs  map[string]struct{}
	llmProv  ai.AIProvider
	llmReady bool
}

// NewResearchModule creates a ResearchModule with the given capacity.
func NewResearchModule(maxSize int) *ResearchModule {
	rm := &ResearchModule{
		maxSize: maxSize,
		seenIDs: make(map[string]struct{}),
	}
	heap.Init(&rm.queue)
	return rm
}

// SetLLMProvider wires an AI provider for LLM-generated objectives.
func (rm *ResearchModule) SetLLMProvider(p ai.AIProvider) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.llmProv = p
	rm.llmReady = p != nil
}

// Push adds a ResearchObjective, deduplicating by ID.
func (rm *ResearchModule) Push(obj ResearchObjective) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, exists := rm.seenIDs[obj.ID]; exists {
		return
	}
	if len(rm.queue) >= rm.maxSize {
		removed := heap.Pop(&rm.queue).(ResearchObjective)
		delete(rm.seenIDs, removed.ID)
	}
	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now()
	}
	heap.Push(&rm.queue, obj)
	rm.seenIDs[obj.ID] = struct{}{}
}

// Top returns a pointer to the highest-priority objective without removing it.
// Returns nil if the queue is empty.
func (rm *ResearchModule) Top() *ResearchObjective {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.queue) == 0 {
		return nil
	}
	maxIdx := 0
	for i, obj := range rm.queue {
		if obj.Priority > rm.queue[maxIdx].Priority {
			maxIdx = i
		}
	}
	cp := rm.queue[maxIdx]
	return &cp
}

// PopTop removes and returns the highest-priority objective.
// Returns nil if the queue is empty.
func (rm *ResearchModule) PopTop() *ResearchObjective {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.queue) == 0 {
		return nil
	}
	maxIdx := 0
	for i, obj := range rm.queue {
		if obj.Priority > rm.queue[maxIdx].Priority {
			maxIdx = i
		}
	}
	obj := heap.Remove(&rm.queue, maxIdx).(ResearchObjective)
	delete(rm.seenIDs, obj.ID)
	return &obj
}

// Len returns the number of queued objectives.
func (rm *ResearchModule) Len() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return len(rm.queue)
}

// GenerateObjectives builds new objectives from heuristics and (optionally) LLM.
func (rm *ResearchModule) GenerateObjectives(ctx context.Context, state *SPARKState) []ResearchObjective {
	objs := rm.heuristicObjectives(state)
	if rm.llmReady {
		llmObjs := rm.llmObjectives(ctx, state)
		objs = append(objs, llmObjs...)
	}
	return objs
}

// heuristicObjectives produces objectives from simple state thresholds.
func (rm *ResearchModule) heuristicObjectives(state *SPARKState) []ResearchObjective {
	var objs []ResearchObjective
	now := time.Now()

	if state.MemoryPressure > 0.85 {
		objs = append(objs, ResearchObjective{
			ID:        fmt.Sprintf("heuristic:context_compression:%d", now.Unix()),
			Topic:     "context compression research — STM near capacity",
			Priority:  state.MemoryPressure,
			Source:    "heuristic",
			CreatedAt: now,
		})
	}

	if state.ActiveDirectives >= 30 {
		objs = append(objs, ResearchObjective{
			ID:        fmt.Sprintf("heuristic:tool_reliability:%d", now.Unix()),
			Topic:     "tool reliability audit — high improvement debt",
			Priority:  float64(state.ActiveDirectives) / 50.0,
			Source:    "heuristic",
			CreatedAt: now,
		})
	}

	for _, entry := range state.TIISnapshot {
		if entry.Invocations > 10 && entry.SuccessRate < 0.4 {
			objs = append(objs, ResearchObjective{
				ID:        fmt.Sprintf("heuristic:tool_failure:%s:%d", entry.ToolName, now.Unix()),
				Topic:     fmt.Sprintf("tool failure investigation: %s (sr=%.2f)", entry.ToolName, entry.SuccessRate),
				Priority:  1.0 - entry.SuccessRate,
				Source:    "heuristic",
				CreatedAt: now,
			})
		}
	}

	if state.DriveScore < 0.4 {
		objs = append(objs, ResearchObjective{
			ID:        fmt.Sprintf("heuristic:prompt_quality:%d", now.Unix()),
			Topic:     "prompt quality / chain-of-thought research — low drive score",
			Priority:  1.0 - state.DriveScore,
			Source:    "heuristic",
			CreatedAt: now,
		})
	}

	return objs
}

// llmObjectives calls the LLM to generate structured research objectives.
func (rm *ResearchModule) llmObjectives(ctx context.Context, state *SPARKState) []ResearchObjective {
	rm.mu.Lock()
	prov := rm.llmProv
	rm.mu.Unlock()
	if prov == nil {
		return nil
	}

	summary := fmt.Sprintf(
		"System state: memory_pressure=%.2f active_directives=%d drive_score=%.2f tii_entries=%d",
		state.MemoryPressure, state.ActiveDirectives, state.DriveScore, len(state.TIISnapshot),
	)
	prompt := fmt.Sprintf(
		`Given this AI system state: %s

Generate 3 research objectives as a JSON array:
[{"id":"obj_1","topic":"...","priority":0.8},{"id":"obj_2","topic":"...","priority":0.6},{"id":"obj_3","topic":"...","priority":0.4}]

Return ONLY valid JSON, no other text.`, summary)

	hist := ai.NewConversationHistory()
	hist.AddUserMessage(prompt)
	resp, err := prov.GenerateWithHistory(ctx, hist)
	if err != nil {
		return nil
	}

	// Extract JSON array from response.
	start := strings.Index(resp, "[")
	end := strings.LastIndex(resp, "]")
	if start < 0 || end <= start {
		return nil
	}
	jsonStr := resp[start : end+1]

	var raw []struct {
		ID       string  `json:"id"`
		Topic    string  `json:"topic"`
		Priority float64 `json:"priority"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}

	now := time.Now()
	objs := make([]ResearchObjective, 0, len(raw))
	for _, r := range raw {
		if r.ID == "" || r.Topic == "" {
			continue
		}
		objs = append(objs, ResearchObjective{
			ID:        "llm:" + r.ID,
			Topic:     r.Topic,
			Priority:  clamp01(r.Priority),
			Source:    "llm",
			CreatedAt: now,
		})
	}
	return objs
}

// FormatObjectivesBlock formats the top-N objectives for prompt injection.
func (rm *ResearchModule) FormatObjectivesBlock(n int) string {
	rm.mu.Lock()
	cp := make([]ResearchObjective, len(rm.queue))
	copy(cp, rm.queue)
	rm.mu.Unlock()

	sortObjectivesByPriority(cp)
	if n > 0 && len(cp) > n {
		cp = cp[:n]
	}
	if len(cp) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[SPARK: Active Research Objectives]\n")
	for i, obj := range cp {
		sb.WriteString(fmt.Sprintf("%d. [%.2f] %s\n", i+1, obj.Priority, obj.Topic))
	}
	return sb.String()
}

// ─── objectiveHeap (min-heap on Priority) ─────────────────────────────────

type objectiveHeap []ResearchObjective

func (h objectiveHeap) Len() int           { return len(h) }
func (h objectiveHeap) Less(i, j int) bool { return h[i].Priority < h[j].Priority } // min-heap
func (h objectiveHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *objectiveHeap) Push(x interface{}) {
	*h = append(*h, x.(ResearchObjective))
}

func (h *objectiveHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func sortObjectivesByPriority(objs []ResearchObjective) {
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].Priority > objs[j].Priority
	})
}
