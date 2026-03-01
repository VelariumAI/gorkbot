package mel

import (
	"fmt"
	"strings"
	"sync"
)

// failRecord stores a pending failure observation waiting for a correction.
type failRecord struct {
	toolName string
	params   map[string]interface{}
	errMsg   string
}

// BifurcationAnalyzer observes tool failure→success cycles and generates
// heuristics for the VectorStore when a correction is detected.
type BifurcationAnalyzer struct {
	mu      sync.Mutex
	pending map[string]*failRecord // keyed by toolName
	store   *VectorStore
}

// NewBifurcationAnalyzer creates an analyzer backed by the given store.
func NewBifurcationAnalyzer(store *VectorStore) *BifurcationAnalyzer {
	return &BifurcationAnalyzer{
		pending: make(map[string]*failRecord),
		store:   store,
	}
}

// ObserveFailed records a tool failure for bifurcation analysis.
// Call this immediately after a tool returns an error result.
func (b *BifurcationAnalyzer) ObserveFailed(toolName string, params map[string]interface{}, errMsg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending[toolName] = &failRecord{
		toolName: toolName,
		params:   params,
		errMsg:   errMsg,
	}
}

// ObserveSuccess completes the bifurcation cycle. If a prior failure exists
// for the same tool, the parameter diff is used to generate a heuristic.
func (b *BifurcationAnalyzer) ObserveSuccess(toolName string, params map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	fail, ok := b.pending[toolName]
	if !ok {
		return
	}
	delete(b.pending, toolName)

	// Diff params: find keys that changed between failure and success.
	changed := diffParams(fail.params, params)
	if len(changed) == 0 {
		return // No diff — not a meaningful bifurcation.
	}

	// Build heuristic from the diff.
	context := "using " + toolName + " with " + strings.Join(changed, ", ")
	constraint := "the corrected parameter values: " + strings.Join(changed, " and ")
	errText := fail.errMsg
	if len(errText) > 100 {
		errText = errText[:100]
	}

	tags := append(tokenize(toolName), tokenize(context)...)

	h := &Heuristic{
		Context:     context,
		Constraint:  constraint,
		Error:       errText,
		ContextTags: tags,
		Confidence:  0.6,
		UseCount:    1,
	}

	b.store.Add(h)
}

// diffParams returns "key: old→new" strings for keys that changed.
func diffParams(old, next map[string]interface{}) []string {
	var diffs []string
	for k, newV := range next {
		oldV, exists := old[k]
		if !exists {
			diffs = append(diffs, k+": (added)")
			continue
		}
		oldStr := fmt.Sprintf("%v", oldV)
		newStr := fmt.Sprintf("%v", newV)
		if oldStr != newStr {
			if len(oldStr) > 30 {
				oldStr = oldStr[:30]
			}
			if len(newStr) > 30 {
				newStr = newStr[:30]
			}
			diffs = append(diffs, k+": "+oldStr+"→"+newStr)
		}
	}
	return diffs
}
