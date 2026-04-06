package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchGeminiModelsWrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-pro","displayName":"Gemini 2.5 Pro","description":"d","inputTokenLimit":1048576,"supportedGenerationMethods":["generateContent"]}]}`))
	}))
	defer srv.Close()

	orig := newDiscoveryHTTPClient
	newDiscoveryHTTPClient = func() *http.Client { return rewrittenClientForServer(srv) }
	t.Cleanup(func() { newDiscoveryHTTPClient = orig })

	models, err := FetchGeminiModels(context.Background(), "k")
	if err != nil {
		t.Fatalf("FetchGeminiModels failed: %v", err)
	}
	if len(models) != 1 || models[0].Provider != "google" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestGeminiPingOAuthAndUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "key=") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("bad key"))
			return
		}
		if r.Header.Get("Authorization") != "Bearer oauth" {
			t.Fatalf("expected oauth bearer")
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	origBase := geminiAPIBaseURL
	origPing := newGeminiPingClient
	geminiAPIBaseURL = srv.URL
	newGeminiPingClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		geminiAPIBaseURL = origBase
		newGeminiPingClient = origPing
	})

	if err := NewGeminiProviderWithAuth("", "oauth", "gemini-2.5-pro", false).Ping(context.Background()); err != nil {
		t.Fatalf("oauth ping failed: %v", err)
	}
	err := NewGeminiProvider("key", "gemini-2.0-flash", false).Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func TestGrokPingSuccessAndUnauthorized(t *testing.T) {
	unauthorized := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("expected bearer auth")
		}
		if r.URL.Path == "/v1/models" {
			if unauthorized {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("bad"))
				return
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origBase := grokAPIBaseURL
	origPing := newGrokPingClient
	grokAPIBaseURL = srv.URL
	newGrokPingClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		grokAPIBaseURL = origBase
		newGrokPingClient = origPing
	})

	p := NewGrokProvider("key", "grok-3")
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	unauthorized = true
	err := p.Ping(context.Background())
	if err == nil {
		t.Fatalf("expected unauthorized error")
	}
}

func TestMoonshotPingUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()

	origBase := moonshotAPIBaseURL
	origPing := newMoonshotPingClient
	moonshotAPIBaseURL = srv.URL
	newMoonshotPingClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		moonshotAPIBaseURL = origBase
		newMoonshotPingClient = origPing
	})

	err := NewMoonshotProvider("key", "moonshot-v1-8k").Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func TestFetchOpenAIModelsOpenAISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("expected auth header")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"text-embedding-3-small"},{"id":"gpt-4o"},{"id":"o3-mini"}]}`))
	}))
	defer srv.Close()

	origURL := openAIModelsEndpoint
	origClient := newOpenAIFetchClient
	openAIModelsEndpoint = srv.URL
	newOpenAIFetchClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		openAIModelsEndpoint = origURL
		newOpenAIFetchClient = origClient
	})

	models, err := FetchOpenAIModels_OpenAI(context.Background(), "key")
	if err != nil {
		t.Fatalf("fetch openai models failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected filtered chat models, got %d", len(models))
	}
}
