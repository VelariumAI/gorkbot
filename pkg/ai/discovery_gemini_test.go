package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchGeminiModelsWithAuth_ApiKeyAndOAuth(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		oauth     string
		assertReq func(t *testing.T, r *http.Request)
	}{
		{
			name:   "api-key",
			apiKey: "k",
			assertReq: func(t *testing.T, r *http.Request) {
				if got := r.URL.Query().Get("key"); got != "k" {
					t.Fatalf("expected key query param, got %q", got)
				}
				if auth := r.Header.Get("Authorization"); auth != "" {
					t.Fatalf("did not expect authorization header, got %q", auth)
				}
			},
		},
		{
			name:  "oauth",
			oauth: "oauth-token",
			assertReq: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
					t.Fatalf("expected oauth bearer auth, got %q", got)
				}
				if got := r.URL.RawQuery; strings.Contains(got, "key=") {
					t.Fatalf("did not expect key query with oauth, got %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.assertReq(t, r)
				_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-pro","displayName":"Gemini 2.5 Pro","description":"d","inputTokenLimit":1048576,"supportedGenerationMethods":["generateContent"]},{"name":"models/embed-1","displayName":"Embed","description":"e","inputTokenLimit":8192,"supportedGenerationMethods":["embedContent"]}]}`))
			}))
			defer srv.Close()

			orig := newDiscoveryHTTPClient
			newDiscoveryHTTPClient = func() *http.Client { return rewrittenClientForServer(srv) }
			t.Cleanup(func() { newDiscoveryHTTPClient = orig })

			models, err := FetchGeminiModelsWithAuth(context.Background(), tt.apiKey, tt.oauth)
			if err != nil {
				t.Fatalf("fetch gemini models failed: %v", err)
			}
			if len(models) != 1 {
				t.Fatalf("expected filtered generate model only, got %d", len(models))
			}
			if models[0].ID != "gemini-2.5-pro" || !models[0].Capabilities.SupportsThinking {
				t.Fatalf("unexpected model payload: %#v", models[0])
			}
		})
	}
}
