package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGrokFlowGenerateStreamAndTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("missing auth header")
		}
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		body := buf.String()
		if strings.Contains(body, `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"))
			_, _ = w.Write([]byte("data: [DONE]\n"))
			return
		}
		if strings.Contains(body, `"tools"`) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function","function":{"name":"calc","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer srv.Close()

	p := NewGrokProvider("key", "grok-3-mini")
	p.Client = rewrittenClientForServer(srv)
	p.SetConvID("conv-1")

	out, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected generate output: %q", out)
	}
	meta := p.GetMetadata()
	if meta.ID == "" || meta.Name == "" {
		t.Fatalf("expected metadata")
	}
	if p.WithModel("grok-3").(*GrokProvider).Model != "grok-3" {
		t.Fatalf("with model should return selected model")
	}

	h := NewConversationHistory()
	h.AddUserMessage("hello")
	outHist, err := p.GenerateWithHistory(context.Background(), h)
	if err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	if outHist != "ok" {
		t.Fatalf("unexpected generate with history output: %q", outHist)
	}
	var stream bytes.Buffer
	if err := p.StreamWithHistory(context.Background(), h, &stream); err != nil {
		t.Fatalf("stream failed: %v", err)
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

	tools := []GrokToolSchema{{
		Type:     "function",
		Function: GrokFunctionDef{Name: "calc", Description: "d", Parameters: []byte(`{"type":"object"}`)},
	}}
	res, err := p.GenerateWithTools(context.Background(), h, tools)
	if err != nil {
		t.Fatalf("generate with tools failed: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Function.Name != "calc" {
		t.Fatalf("unexpected tool result: %#v", res)
	}
	if p.LastUsage().TotalTokens != 3 {
		t.Fatalf("expected usage to be tracked")
	}
}

func TestGrokFetchModelsFallbackOnCancelledContext(t *testing.T) {
	p := NewGrokProvider("key", "grok-3")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	models, err := p.FetchModels(ctx)
	if err != nil {
		t.Fatalf("expected fallback models, got error: %v", err)
	}
	if len(models) == 0 {
		t.Fatalf("expected fallback models")
	}
}
