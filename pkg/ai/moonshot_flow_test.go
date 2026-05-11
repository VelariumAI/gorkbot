package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMoonshotFlowFetchGenerateStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("expected auth header")
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"moonshot-v1-8k"}]}`))
		case "/v1/chat/completions":
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			if strings.Contains(buf.String(), `"stream":true`) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"))
				_, _ = w.Write([]byte("data: [DONE]\n"))
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewMoonshotProvider("key", "moonshot-v1-8k")
	p.client = rewrittenClientForServer(srv)

	models, err := p.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("fetch models failed: %v", err)
	}
	if len(models) != 1 || models[0].Provider != "moonshot" {
		t.Fatalf("unexpected models: %#v", models)
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
}
