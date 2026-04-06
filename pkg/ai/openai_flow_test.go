package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIFlowPingGenerateStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			if r.Header.Get("Authorization") != "Bearer oauth-token" {
				t.Fatalf("expected oauth bearer header")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/chat/completions":
			if r.Header.Get("Authorization") != "Bearer oauth-token" {
				t.Fatalf("expected oauth bearer header")
			}
			if r.Header.Get("Content-Type") == "" {
				t.Fatalf("expected content-type header")
			}
			if r.URL.RawQuery == "" {
				// no-op: we only care this endpoint is reached
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithAuth("api-key", "oauth-token", "gpt-4o")
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

	// Streaming server for SSE.
	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"))
		_, _ = w.Write([]byte("data: [DONE]\n"))
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

func TestOpenAIFetchModelsFallbackOnCancelledContext(t *testing.T) {
	p := NewOpenAIProvider("api-key", "gpt-4o")
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
