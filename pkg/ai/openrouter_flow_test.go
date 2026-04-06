package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterFlowGenerateAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("missing auth header")
		}
		if r.Header.Get("HTTP-Referer") == "" || r.Header.Get("X-Title") == "" {
			t.Fatalf("missing openrouter headers")
		}

		switch r.URL.Path {
		case "/api/v1/chat/completions":
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			if strings.Contains(buf.String(), `"stream":true`) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"))
				_, _ = w.Write([]byte("data: [DONE]\n"))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		case "/api/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"anthropic/claude-3.7","context_length":200000,"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewOpenRouterProvider("key", "anthropic/claude-opus-4-6")
	p.client = rewrittenClientForServer(srv)

	h := NewConversationHistory()
	h.AddSystemMessage("sys")
	h.AddUserMessage("hello")

	out, err := p.GenerateWithHistory(context.Background(), h)
	if err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
	out2, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out2 != "ok" {
		t.Fatalf("unexpected output: %q", out2)
	}

	var stream bytes.Buffer
	if err := p.StreamWithHistory(context.Background(), h, &stream); err != nil {
		t.Fatalf("stream with history failed: %v", err)
	}
	if stream.String() != "hi" {
		t.Fatalf("unexpected stream output: %q", stream.String())
	}
	var stream2 bytes.Buffer
	if err := p.Stream(context.Background(), "hello", &stream2); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if stream2.String() != "hi" {
		t.Fatalf("unexpected stream output: %q", stream2.String())
	}
	meta := p.GetMetadata()
	if meta.ID == "" || meta.Name == "" {
		t.Fatalf("expected metadata")
	}
}

func TestOpenRouterFetchModelsFallbackOnCancelledContext(t *testing.T) {
	p := NewOpenRouterProvider("key", "anthropic/claude-opus-4-6")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	models, err := p.FetchModels(ctx)
	if err != nil {
		t.Fatalf("expected fallback without error: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected fallback models")
	}
}
