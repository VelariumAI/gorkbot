package spark

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

func TestBuildCausalChainAllCategories(t *testing.T) {
	categories := []sense.FailureCategory{
		sense.CatHallucination,
		sense.CatToolFailure,
		sense.CatContextOverflow,
		sense.CatSanitizerReject,
		sense.CatProviderError,
	}
	for _, cat := range categories {
		chain := buildCausalChain(cat)
		if chain == "" {
			t.Errorf("empty causal chain for category %q", cat)
		}
	}
}

func TestProposeDirectivesHallucination(t *testing.T) {
	dir, _ := os.MkdirTemp("", "diag-test-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	idl := NewImprovementDebtLedger(50, dir)
	dk := NewDiagnosisKernel(nil, tii, idl)

	items := []IDLEntry{
		{ID: "h:1", Category: sense.CatHallucination, Severity: 0.8},
	}
	directives := dk.ProposeDirectives(sense.CatHallucination, items, 0.8)
	if len(directives) == 0 {
		t.Fatal("expected at least one directive for high-severity hallucination")
	}
	found := false
	for _, d := range directives {
		if d.Kind == DirectivePromptFix {
			found = true
		}
	}
	if !found {
		t.Error("expected DirectivePromptFix for hallucination with severity >= 0.6")
	}
}

func TestProposeDirectivesToolBanThreshold(t *testing.T) {
	dir, _ := os.MkdirTemp("", "diag-toolban-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	// Create a tool with <30% success + >5 invocations.
	// With α=0.1 from optimistic start (1.0), need ~13+ failures to go below 0.3.
	for i := 0; i < 15; i++ {
		tii.RecordFailure("fragile_tool", 5, "err")
	}
	idl := NewImprovementDebtLedger(50, dir)
	dk := NewDiagnosisKernel(nil, tii, idl)

	items := []IDLEntry{
		{ID: "ft:1", ToolName: "fragile_tool", Category: sense.CatToolFailure, Severity: 0.8},
	}
	directives := dk.ProposeDirectives(sense.CatToolFailure, items, 0.8)
	found := false
	for _, d := range directives {
		if d.Kind == DirectiveToolBan && d.ToolName == "fragile_tool" {
			found = true
		}
	}
	if !found {
		t.Error("expected DirectiveToolBan for tool with <30% success rate and >5 invocations")
	}
}

func TestApplyDirectiveCallbackFired(t *testing.T) {
	dir, _ := os.MkdirTemp("", "diag-apply-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	idl := NewImprovementDebtLedger(50, dir)
	dk := NewDiagnosisKernel(nil, tii, idl)

	var fallbackCalled, promptCalled, banCalled bool

	callbacks := DirectiveCallbacks{
		OnProviderFallback: func(_ string) { fallbackCalled = true },
		OnPromptAmend:      func(_, _ string) { promptCalled = true },
		OnToolBan:          func(_, _ string) { banCalled = true },
	}

	tests := []struct {
		dir      EvolutionaryDirective
		expected *bool
	}{
		{EvolutionaryDirective{Kind: DirectiveFallback, Rationale: "r", Magnitude: 0.5, CreatedAt: time.Now()}, &fallbackCalled},
		{EvolutionaryDirective{Kind: DirectivePromptFix, Rationale: "r", Magnitude: 0.5, CreatedAt: time.Now()}, &promptCalled},
		{EvolutionaryDirective{Kind: DirectiveToolBan, ToolName: "t", Rationale: "r", Magnitude: 0.5, CreatedAt: time.Now()}, &banCalled},
	}

	for _, tc := range tests {
		d := tc.dir
		dk.ApplyDirective(context.Background(), &d, callbacks)
		if !*tc.expected {
			t.Errorf("callback not fired for directive kind %d", tc.dir.Kind)
		}
		if !d.Applied {
			t.Errorf("directive.Applied not set after ApplyDirective for kind %d", tc.dir.Kind)
		}
	}
}
