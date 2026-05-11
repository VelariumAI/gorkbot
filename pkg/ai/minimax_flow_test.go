package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiniMaxFlowGenerateAndStreamWrappers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/anthropic/v1/messages":
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			if strings.Contains(buf.String(), `"stream":true`) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n"))
				_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n"))
				return
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := NewMiniMaxProvider("key", "MiniMax-M1")
	m.inner.BaseURL = srv.URL + "/anthropic/v1"
	m.inner.client = rewrittenClientForServer(srv)

	out, err := m.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected generate output: %q", out)
	}

	h := NewConversationHistory()
	h.AddUserMessage("hello")
	out2, err := m.GenerateWithHistory(context.Background(), h)
	if err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	if out2 != "ok" {
		t.Fatalf("unexpected generate with history output: %q", out2)
	}

	var s1 bytes.Buffer
	if err := m.Stream(context.Background(), "hello", &s1); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if s1.String() != "hi" {
		t.Fatalf("unexpected stream output: %q", s1.String())
	}

	var s2 bytes.Buffer
	if err := m.StreamWithHistory(context.Background(), h, &s2); err != nil {
		t.Fatalf("stream with history failed: %v", err)
	}
	if s2.String() != "hi" {
		t.Fatalf("unexpected stream-with-history output: %q", s2.String())
	}
}

func TestMiniMaxFetchModelsFallbackOnCancelledContext(t *testing.T) {
	m := NewMiniMaxProvider("key", "MiniMax-M1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	models, err := m.FetchModels(ctx)
	if err != nil {
		t.Fatalf("expected fallback without error, got: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected fallback model list")
	}
}
