package engine

import (
	"context"
	"strings"
	"testing"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/providers"
)

func TestGlobalProviderManagerSetGet(t *testing.T) {
	orig := GetProviderManager()
	defer SetProviderManager(orig)

	pm := &providers.Manager{}
	SetProviderManager(pm)
	if got := GetProviderManager(); got != pm {
		t.Fatalf("expected provider manager roundtrip")
	}
}

func TestProviderRoutingMethods_NoCoordinator(t *testing.T) {
	orch := &Orchestrator{}

	if err := orch.SetPrimary(context.Background(), "xai", "grok-3"); err == nil {
		t.Fatalf("expected SetPrimary error when coordinator missing")
	}
	if err := orch.SetSecondary(context.Background(), "google", "gemini-2.0-flash"); err == nil {
		t.Fatalf("expected SetSecondary error when coordinator missing")
	}
	if got := orch.ResolveConsultant(context.Background(), "task"); got != nil {
		t.Fatalf("expected nil consultant when coordinator missing")
	}
	if got := orch.GetProviderStatus(); !strings.Contains(got, "not initialized") {
		t.Fatalf("unexpected provider status: %s", got)
	}
	if got := orch.SetProviderKey(context.Background(), "xai", "key"); !strings.Contains(got, "not initialized") {
		t.Fatalf("unexpected SetProviderKey response: %s", got)
	}
}

func TestProviderRoutingMethods_WithCoordinatorFallbackPaths(t *testing.T) {
	coord := engproviders.NewProviderCoordinator(nil, nil, nil, nil, nil, nil)
	orch := &Orchestrator{ProviderCoord: coord}

	if got := orch.GetProviderStatus(); !strings.Contains(strings.ToLower(got), "not initialized") {
		t.Fatalf("unexpected provider status: %s", got)
	}

	if got := orch.SetProviderKey(context.Background(), "xai", "invalid-key"); !strings.Contains(strings.ToLower(got), "failed to set provider key") {
		t.Fatalf("unexpected SetProviderKey message: %s", got)
	}

	if got := orch.ResolveConsultant(context.Background(), "hello"); got != nil {
		t.Fatalf("expected nil consultant when coordinator has no static/dynamic source")
	}
}
