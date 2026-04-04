package engine

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type bgTestProvider struct {
	generate func(ctx context.Context, prompt string) (string, error)
}

func (p *bgTestProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if p.generate != nil {
		return p.generate(ctx, prompt)
	}
	return "ok", nil
}

func (p *bgTestProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	return p.Generate(ctx, "")
}

func (p *bgTestProvider) Stream(ctx context.Context, prompt string, out io.Writer) error { return nil }

func (p *bgTestProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}

func (p *bgTestProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: "test", ContextSize: 4096}
}
func (p *bgTestProvider) Name() string                   { return "test" }
func (p *bgTestProvider) ID() registry.ProviderID        { return registry.ProviderID("test") }
func (p *bgTestProvider) Ping(ctx context.Context) error { return nil }

func (p *bgTestProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{{ID: "m1", Name: "m1"}}, nil
}

func (p *bgTestProvider) WithModel(model string) ai.AIProvider { return p }

func TestBackgroundAgentStatusStringAndElapsed(t *testing.T) {
	if AgentPending.String() != "pending" || AgentCancelled.String() != "cancelled" {
		t.Fatalf("unexpected status string mapping")
	}
	if BackgroundAgentStatus(123).String() != "unknown" {
		t.Fatalf("expected unknown status")
	}

	a := &BackgroundAgent{StartedAt: time.Now().Add(-20 * time.Millisecond)}
	if a.Elapsed() <= 0 {
		t.Fatalf("expected positive elapsed for running agent")
	}
	a.DoneAt = a.StartedAt.Add(10 * time.Millisecond)
	if a.Elapsed() != 10*time.Millisecond {
		t.Fatalf("expected fixed elapsed for done agent")
	}
}

func TestBackgroundAgentManager_SpawnCollectSuccess(t *testing.T) {
	var mu sync.Mutex
	var doneID, doneLabel, doneResult string
	var doneErr error

	mgr := NewBackgroundAgentManager(1, "default-model", func(agentID, label, result string, err error) {
		mu.Lock()
		defer mu.Unlock()
		doneID, doneLabel, doneResult, doneErr = agentID, label, result, err
	})

	provider := &bgTestProvider{generate: func(ctx context.Context, prompt string) (string, error) {
		if !strings.Contains(prompt, "task") {
			t.Fatalf("expected prompt to contain original task, got: %q", prompt)
		}
		return "done", nil
	}}

	id := mgr.Spawn(context.Background(), BackgroundAgentSpec{
		Label:        "research",
		Prompt:       "task",
		SystemPrompt: "system",
	}, provider)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := mgr.Collect(ctx, id)
	if err != nil {
		t.Fatalf("collect failed: %v", err)
	}
	if result != "done" {
		t.Fatalf("unexpected result: %q", result)
	}

	status, ok := mgr.Status(id)
	if !ok || status != AgentDone {
		t.Fatalf("expected done status, got %v ok=%v", status, ok)
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("expected one agent in list")
	}
	if len(mgr.Running()) != 0 {
		t.Fatalf("expected no running agents")
	}

	mu.Lock()
	defer mu.Unlock()
	if doneID != id || doneLabel != "research" || doneResult != "done" || doneErr != nil {
		t.Fatalf("unexpected done callback values: id=%s label=%s result=%s err=%v", doneID, doneLabel, doneResult, doneErr)
	}
}

func TestBackgroundAgentManager_FailureAndCancelPaths(t *testing.T) {
	mgr := NewBackgroundAgentManager(1, "default-model", nil)

	idNoProvider := mgr.Spawn(context.Background(), BackgroundAgentSpec{Prompt: "x"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := mgr.Collect(ctx, idNoProvider)
	if err == nil || !strings.Contains(err.Error(), "no AI provider available") {
		t.Fatalf("expected missing-provider failure, got: %v", err)
	}

	block := make(chan struct{})
	provider := &bgTestProvider{generate: func(ctx context.Context, prompt string) (string, error) {
		<-block
		return "later", nil
	}}
	id1 := mgr.Spawn(context.Background(), BackgroundAgentSpec{Prompt: "1"}, provider)
	id2 := mgr.Spawn(context.Background(), BackgroundAgentSpec{Prompt: "2"}, provider)

	time.Sleep(30 * time.Millisecond)
	running := mgr.Running()
	if len(running) == 0 {
		t.Fatalf("expected running or pending agents")
	}

	mgr.Cancel(id1)
	mgr.CancelAll()
	close(block)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_, _ = mgr.Collect(ctx2, id2)

	for _, id := range []string{id1, id2} {
		status, ok := mgr.Status(id)
		if !ok {
			t.Fatalf("missing agent %s", id)
		}
		if status != AgentCancelled && status != AgentDone {
			t.Fatalf("unexpected terminal status for %s: %s", id, status.String())
		}
	}

	if _, err := mgr.Collect(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found collect error")
	}
}

func TestBackgroundAgentManager_ContextHelpers(t *testing.T) {
	base := context.Background()
	if got := BackgroundAgentManagerFromContext(base); got != nil {
		t.Fatalf("expected nil manager from empty context")
	}

	mgr := NewBackgroundAgentManager(0, "grok-3", nil)
	ctx := BackgroundAgentManagerToContext(base, mgr)
	if got := BackgroundAgentManagerFromContext(ctx); got != mgr {
		t.Fatalf("manager round-trip through context failed")
	}

	block := make(chan struct{})
	id := mgr.Spawn(context.Background(), BackgroundAgentSpec{Prompt: "x"}, &bgTestProvider{
		generate: func(ctx context.Context, prompt string) (string, error) {
			<-block
			return "done", nil
		},
	})
	cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer ccancel()
	_, err := mgr.Collect(cctx, id)
	close(block)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got: %v", err)
	}
}
