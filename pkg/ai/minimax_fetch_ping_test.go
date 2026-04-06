package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchMiniMaxModelsAndProviderPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer key" {
				t.Fatalf("expected bearer auth")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"MiniMax-M2.5"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origURL := minimaxModelsEndpoint
	origPing := newMiniMaxPingClient
	origRetry := newMiniMaxRetryClient
	minimaxModelsEndpoint = srv.URL + "/v1/models"
	newMiniMaxPingClient = func() *http.Client { return srv.Client() }
	newMiniMaxRetryClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		minimaxModelsEndpoint = origURL
		newMiniMaxPingClient = origPing
		newMiniMaxRetryClient = origRetry
	})

	models, err := FetchMiniMaxModels(context.Background(), "key")
	if err != nil {
		t.Fatalf("fetch minimax models failed: %v", err)
	}
	if len(models) != 1 || models[0].ID != "MiniMax-M2.5" {
		t.Fatalf("unexpected minimax models: %#v", models)
	}

	p := NewMiniMaxProvider("key", "MiniMax-M1")
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestMiniMaxPingUnauthorizedAndFetchFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	defer srv.Close()

	origURL := minimaxModelsEndpoint
	origPing := newMiniMaxPingClient
	origRetry := newMiniMaxRetryClient
	minimaxModelsEndpoint = srv.URL
	newMiniMaxPingClient = func() *http.Client { return srv.Client() }
	newMiniMaxRetryClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		minimaxModelsEndpoint = origURL
		newMiniMaxPingClient = origPing
		newMiniMaxRetryClient = origRetry
	})

	err := NewMiniMaxProvider("key", "MiniMax-M1").Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid key error, got %v", err)
	}

	models, err := FetchMiniMaxModels(context.Background(), "key")
	if err != nil {
		t.Fatalf("fetch fallback should not error: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected fallback models")
	}
}
