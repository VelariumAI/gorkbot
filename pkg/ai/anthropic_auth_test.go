package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicPingUsesOAuthBearerWhenPresent(t *testing.T) {
	var gotAuth string
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithAuth("api-key", "oauth-token", "")
	p.BaseURL = srv.URL

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if gotAuth != "Bearer oauth-token" {
		t.Fatalf("expected oauth bearer header, got %q", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("expected x-api-key to be empty in oauth mode, got %q", gotAPIKey)
	}
}

func TestAnthropicPingUsesAPIKeyWithoutOAuth(t *testing.T) {
	var gotAuth string
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithAuth("api-key", "", "")
	p.BaseURL = srv.URL

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no bearer auth header in API-key mode, got %q", gotAuth)
	}
	if gotAPIKey != "api-key" {
		t.Fatalf("expected x-api-key header, got %q", gotAPIKey)
	}
}
