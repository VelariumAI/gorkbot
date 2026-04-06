package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicFlowPingGenerateStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			// Ping path uses ?limit=1
			if r.Header.Get("Authorization") != "Bearer oauth-token" {
				t.Fatalf("expected oauth bearer auth")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/messages":
			if r.Header.Get("Authorization") != "Bearer oauth-token" {
				t.Fatalf("expected oauth bearer auth")
			}
			if r.Header.Get("anthropic-version") == "" {
				t.Fatalf("expected anthropic version header")
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithAuth("api-key", "oauth-token", "claude-sonnet-4-5")
	p.BaseURL = srv.URL

	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	h := NewConversationHistory()
	h.AddUserMessage("hello")
	out, err := p.GenerateWithHistory(context.Background(), h)
	if err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected generate output: %q", out)
	}
	out2, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out2 != "ok" {
		t.Fatalf("unexpected generate output: %q", out2)
	}

	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n"))
	}))
	defer streamSrv.Close()
	p.BaseURL = streamSrv.URL

	var buf bytes.Buffer
	if err := p.StreamWithHistory(context.Background(), h, &buf); err != nil {
		t.Fatalf("stream with history failed: %v", err)
	}
	if buf.String() != "hi" {
		t.Fatalf("unexpected streamed output: %q", buf.String())
	}
	var buf2 bytes.Buffer
	if err := p.Stream(context.Background(), "hello", &buf2); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if buf2.String() != "hi" {
		t.Fatalf("unexpected streamed output: %q", buf2.String())
	}
}

func TestAnthropicFetchModelsFallbackOnCancelledContext(t *testing.T) {
	p := NewAnthropicProvider("api-key", "claude-sonnet-4-5")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	models, err := p.FetchModels(ctx)
	if err != nil {
		t.Fatalf("expected safe fallback, got err: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected safe fallback model list")
	}
}
