package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicGenerateWithToolsAndFetchModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/messages":
			if r.Header.Get("anthropic-version") == "" {
				t.Fatalf("expected anthropic version header")
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"thinking","text":"reason"},{"type":"tool_use","id":"t1","name":"calc","input":{"x":1}}],"stop_reason":"tool_use"}`))
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-5","display_name":"Claude Sonnet 4.5"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewAnthropicProvider("key", "claude-sonnet-4-5")
	p.BaseURL = srv.URL
	p.client = rewrittenClientForServer(srv)

	h := NewConversationHistory()
	h.AddUserMessage("hello")
	tools := []GrokToolSchema{{Type: "function", Function: GrokFunctionDef{Name: "calc", Description: "d", Parameters: []byte(`{"type":"object"}`)}}}
	res, err := p.GenerateWithTools(context.Background(), h, tools)
	if err != nil {
		t.Fatalf("generate with tools failed: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Function.Name != "calc" {
		t.Fatalf("unexpected tool calls: %#v", res)
	}

	origURL := anthropicModelsEndpoint
	origClient := newAnthropicRetryClient
	anthropicModelsEndpoint = srv.URL + "/models"
	newAnthropicRetryClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		anthropicModelsEndpoint = origURL
		newAnthropicRetryClient = origClient
	})

	models, err := FetchAnthropicModels(context.Background(), "key", false)
	if err != nil {
		t.Fatalf("fetch anthropic models failed: %v", err)
	}
	if len(models) != 1 || !models[0].Capabilities.SupportsThinking {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestFetchAnthropicModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()

	origURL := anthropicModelsEndpoint
	origClient := newAnthropicRetryClient
	anthropicModelsEndpoint = srv.URL
	newAnthropicRetryClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() {
		anthropicModelsEndpoint = origURL
		newAnthropicRetryClient = origClient
	})

	_, err := FetchAnthropicModels(context.Background(), "key", true)
	if err == nil || !strings.Contains(err.Error(), "API error") {
		t.Fatalf("expected API error, got %v", err)
	}
}
