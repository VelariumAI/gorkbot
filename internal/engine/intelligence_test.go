package engine

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/adaptive"
)

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1, 0, 0}, nil
}
func (m *mockEmbedder) Dims() int    { return 3 }
func (m *mockEmbedder) Name() string { return "mock" }

func TestNewIntelligenceLayer_AndCoreMethods(t *testing.T) {
	hal := platform.HALProfile{Platform: "linux", FreeRAMMB: 8192}
	il, err := NewIntelligenceLayer(hal, t.TempDir())
	if err != nil {
		t.Fatalf("new intelligence layer failed: %v", err)
	}
	if il.Router == nil || il.Store == nil || il.Analyzer == nil || il.Reframer == nil || il.FallbackRouting == nil {
		t.Fatalf("expected all intelligence components initialized")
	}

	if got, ok := il.RouteSource("slack:alerts"); ok || got != "" {
		t.Fatalf("expected no route binding initially")
	}
	if err := il.FallbackRouting.Add("slack:.*", "ops-agent"); err != nil {
		t.Fatalf("failed to add fallback routing rule: %v", err)
	}
	if got, ok := il.RouteSource("slack:alerts"); !ok || got != "ops-agent" {
		t.Fatalf("unexpected route binding result: %q ok=%v", got, ok)
	}

	dec := il.Route("hello world")
	if dec.Timestamp.IsZero() {
		t.Fatalf("expected route decision timestamp")
	}

	if il.HeuristicContext("nothing yet") != "" {
		t.Fatalf("expected empty heuristic context when store is empty")
	}

	il.ObserveFailed("search", map[string]interface{}{"q": "bad"}, "error from tool")
	il.ObserveSuccess("search", map[string]interface{}{"q": "good"})
	if il.Store.Len() == 0 {
		t.Fatalf("expected heuristic to be learned after fail->success cycle")
	}
	if got := il.HeuristicContext("search query tuning"); got == "" {
		t.Fatalf("expected heuristic context after learning")
	}

	if !il.IsHighRisk("please rm -rf /tmp/abc") {
		t.Fatalf("expected high-risk prompt detection")
	}
	if il.IsHighRisk("just say hello") {
		t.Fatalf("did not expect high-risk classification")
	}
}

func TestIntelligenceLayer_SetEmbedderWithProjection(t *testing.T) {
	hal := platform.HALProfile{Platform: "linux", FreeRAMMB: 512}
	il, err := NewIntelligenceLayer(hal, t.TempDir())
	if err != nil {
		t.Fatalf("new intelligence layer failed: %v", err)
	}

	il.SetEmbedderWithProjection(&mockEmbedder{}, hal)

	name := il.Router.EmbedderName()
	if name == "none" || name == "" {
		t.Fatalf("expected router to report active embedder")
	}

	// Ensure store can still be queried after projection wiring.
	il.Store.Add(&adaptive.Heuristic{Context: "ctx", Constraint: "c", Error: "e", Confidence: 0.9})
	_ = il.Store.Query("ctx", 1)
}

func TestIntelligenceLayer_RouteSourceNilFallback(t *testing.T) {
	il := &IntelligenceLayer{FallbackRouting: nil}
	if got, ok := il.RouteSource("anything"); ok || got != "" {
		t.Fatalf("expected no route when fallback table is nil")
	}
}
