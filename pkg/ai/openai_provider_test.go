package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIPingUsesOAuthTokenWhenPresent(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithAuth("api-key", "oauth-token", "")
	p.BaseURL = srv.URL

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if gotAuth != "Bearer oauth-token" {
		t.Fatalf("expected oauth bearer header, got %q", gotAuth)
	}
}

func TestOpenAIPingFallsBackToAPIKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithAuth("api-key", "", "")
	p.BaseURL = srv.URL

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if gotAuth != "Bearer api-key" {
		t.Fatalf("expected API key bearer header, got %q", gotAuth)
	}
}
