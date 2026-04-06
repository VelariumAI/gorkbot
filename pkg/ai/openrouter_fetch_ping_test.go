package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterFetchModelsAndPingPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			if r.Header.Get("Authorization") != "Bearer key" {
				t.Fatalf("expected auth header")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"tiny/model","context_length":1024,"pricing":{"prompt":"0.000001","completion":"0.000001"}},{"id":"anthropic/claude-3.7","context_length":200000,"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origBase := openRouterAPIBaseURL
	origPing := newOpenRouterPingClient
	origRetry := newOpenRouterRetryClient
	openRouterAPIBaseURL = srv.URL + "/api/v1"
	newOpenRouterPingClient = func() *http.Client { return srv.Client() }
	newOpenRouterRetryClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		openRouterAPIBaseURL = origBase
		newOpenRouterPingClient = origPing
		newOpenRouterRetryClient = origRetry
	})

	p := NewOpenRouterProvider("key", "anthropic/claude-opus-4-6")
	models, err := p.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("fetch models failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected tiny model to be filtered out, got %d models", len(models))
	}
	if !models[0].Capabilities.SupportsThinking {
		t.Fatalf("expected thinking-capable model")
	}

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestOpenRouterPingUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	defer srv.Close()

	origBase := openRouterAPIBaseURL
	origPing := newOpenRouterPingClient
	openRouterAPIBaseURL = srv.URL
	newOpenRouterPingClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		openRouterAPIBaseURL = origBase
		newOpenRouterPingClient = origPing
	})

	err := NewOpenRouterProvider("key", "model").Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}
