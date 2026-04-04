package engine

import (
	"context"
	"io"
	"strings"
	"testing"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type gateProvider struct {
	resp string
	err  error
}

func (p *gateProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.resp, p.err
}
func (p *gateProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	return p.resp, p.err
}
func (p *gateProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return nil
}
func (p *gateProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}
func (p *gateProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: "test-model", ContextSize: 8192}
}
func (p *gateProvider) Name() string                   { return "test-provider" }
func (p *gateProvider) ID() registry.ProviderID        { return registry.ProviderID("test") }
func (p *gateProvider) Ping(ctx context.Context) error { return nil }
func (p *gateProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{{ID: "test-model", Name: "test-model"}}, nil
}
func (p *gateProvider) WithModel(model string) ai.AIProvider { return p }

func TestEvaluateRules_Categories(t *testing.T) {
	if got := evaluateRules("hello").Category; got != "quick" {
		t.Fatalf("expected quick category, got %q", got)
	}
	if got := evaluateRules("please debug this golang function with stacktrace").Category; got != "deep" && got != "code" {
		t.Fatalf("expected deep/code category, got %q", got)
	}
	sec := evaluateRules("run nmap scan against target")
	if sec.Category != "security" || len(sec.SpawnAgents) == 0 {
		t.Fatalf("expected security with recon spawn")
	}
	research := evaluateRules("search docs for the API")
	if research.Category != "research" || len(research.SpawnAgents) == 0 {
		t.Fatalf("expected research with doc-search spawn")
	}
	if got := evaluateRules("design a frontend ui layout").Category; got != "visual" {
		t.Fatalf("expected visual category, got %q", got)
	}
	if got := evaluateRules("write a poem").Category; got != "creative" {
		t.Fatalf("expected creative category, got %q", got)
	}
}

func TestRunIntentGate(t *testing.T) {
	orch := &Orchestrator{NativeLLMEnabled: false}
	res := orch.RunIntentGate(context.Background(), "hello")
	if res == nil || res.Category != "quick" {
		t.Fatalf("expected rules-based quick result, got %+v", res)
	}

	orch2 := &Orchestrator{NativeLLMEnabled: true}
	if got := orch2.RunIntentGate(context.Background(), "anything"); got != nil {
		t.Fatalf("expected nil when native mode has no providers")
	}

	provider := &gateProvider{resp: `{"category":"research","spawn_agents":[{"label":"doc-search","prompt":"find docs"}]}`}
	coord := engproviders.NewProviderCoordinator(nil, provider, nil, nil, nil, nil)
	orch3 := &Orchestrator{NativeLLMEnabled: true, ProviderCoord: coord}
	out := orch3.RunIntentGate(context.Background(), "find docs")
	if out == nil || out.Category != "research" || len(out.SpawnAgents) != 1 {
		t.Fatalf("expected parsed provider gate result, got %+v", out)
	}
}

func TestIntrospection_NilAndBasicPaths(t *testing.T) {
	orch := &Orchestrator{}
	if !strings.Contains(orch.GetAuditStats("summary", ""), "not available") {
		t.Fatalf("expected missing registry message")
	}
	if !strings.Contains(orch.GetRoutingStats(), "not initialized") {
		t.Fatalf("expected missing routing message")
	}
	if !strings.Contains(orch.GetHeuristics("x", 3), "not initialized") {
		t.Fatalf("expected missing heuristics store message")
	}
	if !strings.Contains(orch.GetMemoryState(""), "not initialized") {
		t.Fatalf("expected missing memory components message")
	}
	if !strings.Contains(orch.GetProviderStatus(), "not initialized") {
		t.Fatalf("expected missing provider coordinator message")
	}
}

func TestIntrospection_RuntimeAndSystemState(t *testing.T) {
	hal := platform.HALProfile{Platform: "linux", FreeRAMMB: 8192}
	il, err := NewIntelligenceLayer(hal, t.TempDir())
	if err != nil {
		t.Fatalf("failed to build intelligence layer: %v", err)
	}
	il.Route("quick test")

	orch := &Orchestrator{
		Intelligence:     il,
		ContextMgr:       NewContextManager(1000, nil),
		BackgroundAgents: NewBackgroundAgentManager(1, "grok-3", nil),
		HITLGuard:        &HITLGuard{Enabled: true},
	}
	orch.ContextMgr.SetInputTokens(250)

	runtime := orch.GetRuntimeStatus()
	if !strings.Contains(runtime, "Gorkbot Runtime Status") || !strings.Contains(runtime, "Context") {
		t.Fatalf("unexpected runtime status output: %s", runtime)
	}

	sys := orch.GetSystemState()
	if !strings.Contains(sys, "Gorkbot System Diagnostic") || !strings.Contains(sys, "HITL Guard") {
		t.Fatalf("unexpected system state output: %s", sys)
	}

	routing := orch.GetRoutingStats()
	if !strings.Contains(routing, "ARC Router Statistics") || !strings.Contains(routing, "Per-class breakdown") {
		t.Fatalf("unexpected routing stats output: %s", routing)
	}
}
