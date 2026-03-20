package spark

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// FRCResult groups the causal chain and proposed directives for one failure category.
type FRCResult struct {
	Category    sense.FailureCategory
	CausalChain string
	Severity    float64
	Directives  []EvolutionaryDirective
}

// DiagnosisKernel implements FRC (Failure Root-Cause) ontology analysis.
// It reads IDL entries, groups them by FailureCategory, builds causal chains,
// and proposes EvolutionaryDirectives.
type DiagnosisKernel struct {
	analyzer *sense.TraceAnalyzer // nil-safe
	tii      *EfficiencyEngine
	idl      *ImprovementDebtLedger
}

// NewDiagnosisKernel creates a DiagnosisKernel.  analyzer may be nil.
func NewDiagnosisKernel(analyzer *sense.TraceAnalyzer, tii *EfficiencyEngine, idl *ImprovementDebtLedger) *DiagnosisKernel {
	return &DiagnosisKernel{analyzer: analyzer, tii: tii, idl: idl}
}

// AnalyzeFRC fetches the top IDL entries, groups by category, and returns FRC results.
func (d *DiagnosisKernel) AnalyzeFRC(_ context.Context) []FRCResult {
	items := d.idl.Top(10)
	if len(items) == 0 {
		return nil
	}

	// Group by FailureCategory.
	grouped := make(map[sense.FailureCategory][]IDLEntry)
	for _, item := range items {
		grouped[item.Category] = append(grouped[item.Category], item)
	}

	var results []FRCResult
	for cat, catItems := range grouped {
		chain := buildCausalChain(cat)
		sev := avgSeverity(catItems)
		directives := d.ProposeDirectives(cat, catItems, sev)
		results = append(results, FRCResult{
			Category:    cat,
			CausalChain: chain,
			Severity:    sev,
			Directives:  directives,
		})
	}
	return results
}

// buildCausalChain returns the FRC causal chain string for a failure category.
func buildCausalChain(cat sense.FailureCategory) string {
	switch cat {
	case sense.CatHallucination:
		return "overconfident param generation → structurally invalid tool args → " +
			"tool rejection/parse error → AI retries identically → " +
			"fix: inject parameter schema constraints"
	case sense.CatToolFailure:
		return "missing external dependency → non-zero exit → error not surfaced clearly → " +
			"retry loop drains turn budget → " +
			"fix: pre-flight capability check"
	case sense.CatContextOverflow:
		return "unrestricted tool output → history exceeds token limit → " +
			"hard truncation removes reasoning → re-fetch loop → " +
			"fix: output size caps + compress-before-truncate"
	case sense.CatSanitizerReject:
		return "external content triggers InputSanitizer → tool blocked → " +
			"non-actionable error → LLM tries alternative path → " +
			"fix: user-facing error with specific violation type"
	case sense.CatProviderError:
		return "429/5xx → error without retry → cascade not triggered → session stall → " +
			"fix: automatic cascade on transient errors"
	default:
		return fmt.Sprintf("unknown failure category: %s", cat)
	}
}

// ProposeDirectives generates EvolutionaryDirectives from a failure group.
func (d *DiagnosisKernel) ProposeDirectives(cat sense.FailureCategory, items []IDLEntry, severity float64) []EvolutionaryDirective {
	var directives []EvolutionaryDirective
	now := time.Now()

	switch cat {
	case sense.CatHallucination:
		if severity >= 0.6 {
			directives = append(directives, EvolutionaryDirective{
				Kind:      DirectivePromptFix,
				Rationale: "Repeated hallucination detected — inject parameter schema constraints into system prompt",
				Magnitude: severity,
				CreatedAt: now,
			})
		}

	case sense.CatToolFailure:
		for _, item := range items {
			entry := d.tii.GetEntry(item.ToolName)
			if entry != nil && entry.SuccessRate < 0.3 && entry.Invocations > 5 {
				directives = append(directives, EvolutionaryDirective{
					Kind:      DirectiveToolBan,
					ToolName:  item.ToolName,
					Rationale: fmt.Sprintf("Tool %q has <30%% success rate over %d calls — suspend pending investigation", item.ToolName, entry.Invocations),
					Magnitude: 1.0 - entry.SuccessRate,
					CreatedAt: now,
				})
			} else {
				directives = append(directives, EvolutionaryDirective{
					Kind:      DirectiveRetry,
					ToolName:  item.ToolName,
					Rationale: fmt.Sprintf("Tool %q failures may be transient — retry with adjusted params", item.ToolName),
					Magnitude: severity,
					CreatedAt: now,
				})
			}
		}

	case sense.CatContextOverflow:
		directives = append(directives, EvolutionaryDirective{
			Kind:      DirectivePromptFix,
			Rationale: "Context overflow pattern detected — add output_limit guidance and compress-before-truncate directive",
			Magnitude: severity,
			CreatedAt: now,
		})

	case sense.CatProviderError:
		directives = append(directives, EvolutionaryDirective{
			Kind:      DirectiveFallback,
			Rationale: "Provider error pattern detected — trigger automatic cascade on transient errors",
			Magnitude: severity,
			CreatedAt: now,
		})

	case sense.CatSanitizerReject:
		directives = append(directives, EvolutionaryDirective{
			Kind:      DirectivePromptFix,
			Rationale: "Sanitizer reject pattern — improve error messaging to surface specific violation type to user",
			Magnitude: severity,
			CreatedAt: now,
		})
	}

	return directives
}

// ApplyDirective executes the appropriate callback for a directive.
// Returns true if the directive was applied.
func (d *DiagnosisKernel) ApplyDirective(_ context.Context, dir *EvolutionaryDirective, callbacks DirectiveCallbacks) bool {
	if dir.Applied {
		return false
	}
	switch dir.Kind {
	case DirectiveFallback:
		if callbacks.OnProviderFallback != nil {
			callbacks.OnProviderFallback(dir.Rationale)
		}
	case DirectivePromptFix:
		if callbacks.OnPromptAmend != nil {
			callbacks.OnPromptAmend(dir.ToolName, dir.Rationale)
		}
	case DirectiveToolBan:
		if callbacks.OnToolBan != nil {
			callbacks.OnToolBan(dir.ToolName, dir.Rationale)
		}
	case DirectiveRetry, DirectiveResearch:
		// Handled by ResearchModule or upstream caller.
	default:
		return false
	}
	now := time.Now()
	dir.AppliedAt = &now
	dir.Applied = true
	return true
}

// avgSeverity computes the mean severity across a slice of IDL entries.
func avgSeverity(items []IDLEntry) float64 {
	if len(items) == 0 {
		return 0
	}
	var sum float64
	for _, item := range items {
		sum += item.Severity
	}
	return sum / float64(len(items))
}

// toolNamesFromItems extracts unique tool names from IDL entries.
func toolNamesFromItems(items []IDLEntry) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, item := range items {
		if item.ToolName != "" {
			if _, ok := seen[item.ToolName]; !ok {
				seen[item.ToolName] = struct{}{}
				names = append(names, item.ToolName)
			}
		}
	}
	return names
}

// formatToolList returns a comma-separated list of tool names (unused outside package, kept for completeness).
func formatToolList(items []IDLEntry) string {
	return strings.Join(toolNamesFromItems(items), ", ")
}
