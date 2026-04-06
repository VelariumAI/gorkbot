package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchOpenAIModelsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("expected bearer authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"grok-3-mini","object":"model","created":1,"owned_by":"xai"}]}`))
	}))
	defer srv.Close()

	models, err := FetchOpenAIModels(context.Background(), srv.URL, "test-key")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected one model, got %d", len(models))
	}
	if models[0].ID != "grok-3-mini" {
		t.Fatalf("unexpected model id: %s", models[0].ID)
	}
}

func TestFetchOpenAIModelsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := FetchOpenAIModels(context.Background(), srv.URL, "bad-key"); err == nil {
		t.Fatalf("expected error on non-200 status")
	}
}

func TestDiscoveryWrappersCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Anthropic wrapper should bubble error from cancelled request path.
	if _, err := FetchAnthropicModels_Discovery(ctx, "token"); err == nil {
		t.Fatalf("expected anthropic discovery error for cancelled context")
	}

	// MiniMax wrapper should return fallback models on request failure.
	models, err := FetchMiniMaxModels_Discovery(ctx, "token")
	if err != nil {
		t.Fatalf("did not expect minimax wrapper error: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected minimax fallback models")
	}
}
