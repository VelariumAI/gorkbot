package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiFlowGenerateAndStreamWithOAuth(t *testing.T) {
	var sawCachedContent bool
	var sawAuth bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "Bearer oauth-token" {
			sawAuth = true
		}

		switch {
		case strings.Contains(r.URL.Path, ":generateContent"):
			var req GeminiRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode generate request: %v", err)
			}
			if req.CachedContent == "cachedContents/abc" {
				sawCachedContent = true
			}
			_ = json.NewEncoder(w).Encode(GeminiResponse{
				Candidates: []struct {
					Content struct {
						Parts []GeminiPart `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				}{
					{Content: struct {
						Parts []GeminiPart `json:"parts"`
					}{Parts: []GeminiPart{{Text: "secret-thought", Thought: true}, {Text: "answer"}}}},
				},
			})
		case strings.Contains(r.URL.Path, ":streamGenerateContent"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"t\",\"thought\":true},{\"text\":\"hi\"}]}}]}\n"))
			_, _ = w.Write([]byte("data: [DONE]\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewGeminiProviderWithAuth("api-key", "oauth-token", "gemini-2.5-pro", false)
	p.Client = rewrittenClientForServer(srv)
	p.SetCachedContent("cachedContents/abc")

	h := NewConversationHistory()
	h.AddSystemMessage("system")
	h.AddUserMessage("hello")

	out, err := p.GenerateWithHistory(context.Background(), h)
	if err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	if out != "answer" {
		t.Fatalf("unexpected output: %q", out)
	}

	out2, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out2 != "answer" {
		t.Fatalf("unexpected generate output: %q", out2)
	}

	var streamBuf bytes.Buffer
	if err := p.StreamWithHistory(context.Background(), h, &streamBuf); err != nil {
		t.Fatalf("stream with history failed: %v", err)
	}
	if streamBuf.String() != "hi" {
		t.Fatalf("unexpected stream output: %q", streamBuf.String())
	}
	var streamBuf2 bytes.Buffer
	if err := p.Stream(context.Background(), "hello", &streamBuf2); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if streamBuf2.String() != "hi" {
		t.Fatalf("unexpected stream output: %q", streamBuf2.String())
	}

	if !sawAuth {
		t.Fatalf("expected oauth auth header on gemini requests")
	}
	if !sawCachedContent {
		t.Fatalf("expected cachedContent to be forwarded")
	}
}

func TestGeminiFetchModelsFallbacks(t *testing.T) {
	p := NewGeminiProvider("", "", false)
	models, err := p.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("fetch models with empty auth should not error: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected safe fallback models")
	}

	p2 := NewGeminiProvider("api-key", "gemini-2.0-flash", false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	models2, err := p2.FetchModels(ctx)
	if err != nil {
		t.Fatalf("fetch models with cancelled ctx should still fallback: %v", err)
	}
	if len(models2) == 0 {
		t.Fatalf("expected fallback models on cancelled ctx")
	}
}
