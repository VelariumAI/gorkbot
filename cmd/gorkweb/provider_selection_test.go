package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type fakeProvider struct {
	id    registry.ProviderID
	name  string
	model string
}

func (f *fakeProvider) Generate(context.Context, string) (string, error) { return "", nil }
func (f *fakeProvider) GenerateWithHistory(context.Context, *ai.ConversationHistory) (string, error) {
	return "", nil
}
func (f *fakeProvider) Stream(context.Context, string, io.Writer) error { return nil }
func (f *fakeProvider) StreamWithHistory(context.Context, *ai.ConversationHistory, io.Writer) error {
	return nil
}
func (f *fakeProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: f.model, Name: f.name}
}
func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) ID() registry.ProviderID {
	return f.id
}
func (f *fakeProvider) Ping(context.Context) error { return nil }
func (f *fakeProvider) WithModel(model string) ai.AIProvider {
	cp := *f
	cp.model = model
	return &cp
}
func (f *fakeProvider) FetchModels(context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{{
		ID:       registry.ModelID(f.model),
		Provider: f.id,
		Name:     f.model,
		Status:   registry.StatusActive,
	}}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testRegistry(t *testing.T, providers ...*fakeProvider) *registry.ModelRegistry {
	t.Helper()
	r := registry.NewModelRegistry(testLogger())
	for _, p := range providers {
		if err := r.RegisterProvider(context.Background(), p); err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}
	return r
}

func TestSelectProvidersBranches(t *testing.T) {
	logger := testLogger()

	if _, _, err := SelectProviders(nil, "", "", logger); err == nil {
		t.Fatalf("expected nil registry error")
	}

	empty := registry.NewModelRegistry(logger)
	if _, _, err := SelectProviders(empty, "", "", logger); err == nil {
		t.Fatalf("expected no providers error")
	}

	p1 := &fakeProvider{id: "xai", name: "Grok", model: "grok-3"}
	p2 := &fakeProvider{id: "google", name: "Gemini", model: "gemini-2.5-pro"}
	reg := testRegistry(t, p1, p2)

	primary, consultant, err := SelectProviders(reg, "", "", logger)
	if err != nil {
		t.Fatalf("auto select failed: %v", err)
	}
	if primary == nil || consultant == nil || primary == consultant {
		t.Fatalf("expected distinct primary and consultant")
	}

	primary2, consultant2, err := SelectProviders(reg, "google", "xai", logger)
	if err != nil {
		t.Fatalf("override select failed: %v", err)
	}
	if primary2.ID() != "google" || consultant2.ID() != "xai" {
		t.Fatalf("unexpected override selection: %s %s", primary2.ID(), consultant2.ID())
	}

	if _, _, err := SelectProviders(reg, "missing", "", logger); err == nil {
		t.Fatalf("expected missing primary override error")
	}
	if _, _, err := SelectProviders(reg, "xai", "missing", logger); err == nil {
		t.Fatalf("expected missing consultant override error")
	}
	if _, _, err := SelectProviders(reg, "xai", "xai", logger); err == nil {
		t.Fatalf("expected same provider conflict error")
	}
}

func TestValidateProviderConfigAndNames(t *testing.T) {
	logger := testLogger()
	good := &fakeProvider{id: "xai", name: "Grok", model: "grok-3"}
	bad := &fakeProvider{id: "google", name: "", model: "gemini-2.5-pro"}

	if err := ValidateProviderConfig(nil, nil, logger); err == nil {
		t.Fatalf("expected primary required error")
	}
	if err := ValidateProviderConfig(bad, nil, logger); err == nil {
		t.Fatalf("expected invalid primary metadata error")
	}
	if err := ValidateProviderConfig(good, bad, logger); err == nil {
		t.Fatalf("expected invalid consultant metadata error")
	}
	if err := ValidateProviderConfig(good, nil, logger); err != nil {
		t.Fatalf("expected valid primary config, got %v", err)
	}

	p, c := GetProviderNames(good, nil)
	if p != "Grok" || c != "" {
		t.Fatalf("unexpected names: %q %q", p, c)
	}
	p2, c2 := GetProviderNames(good, good)
	if p2 != "Grok" || c2 != "Grok" {
		t.Fatalf("unexpected names: %q %q", p2, c2)
	}
}
