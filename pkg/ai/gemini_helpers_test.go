package ai

import (
	"net/http"
	"strings"
	"testing"
)

func TestGeminiAuthHelpers(t *testing.T) {
	g := NewGeminiProviderWithAuth("api-key", "oauth-token", "gemini-2.0-flash", true)
	if !g.usingOAuth() {
		t.Fatalf("expected usingOAuth true")
	}
	url := g.getURL(true)
	if !strings.Contains(url, "alt=sse") {
		t.Fatalf("expected sse query param in streaming URL")
	}
	if strings.Contains(url, "key=") {
		t.Fatalf("did not expect key query when oauth is used")
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	g.applyAuth(req)
	if got := req.Header.Get("Authorization"); got != "Bearer oauth-token" {
		t.Fatalf("unexpected auth header: %q", got)
	}
}

func TestGeminiURLUsesAPIKeyWhenNoOAuth(t *testing.T) {
	g := NewGeminiProviderWithAuth("api-key", "", "gemini-2.0-flash", false)
	url := g.getURL(false)
	if !strings.Contains(url, "key=api-key") {
		t.Fatalf("expected API key query param when no oauth token")
	}
}

func TestGeminiEffectiveTemp(t *testing.T) {
	g := NewGeminiProvider("k", "", false)
	if got := g.effectiveTemp(); got != 0.7 {
		t.Fatalf("expected default temp 0.7, got %v", got)
	}
	g2 := g.WithTemperature(0.0).(*GeminiProvider)
	if got := g2.effectiveTemp(); got != 0.0 {
		t.Fatalf("expected explicit temp 0.0, got %v", got)
	}
}

func TestGeminiHistoryAndResponseExtraction(t *testing.T) {
	g := NewGeminiProvider("k", "", true)
	h := NewConversationHistory()
	h.AddSystemMessage("sys")
	h.AddUserMessage("u1")
	h.AddAssistantMessage("a1")
	contents := g.convertHistoryToContents(h)
	if len(contents) != 2 {
		t.Fatalf("expected 2 non-system contents, got %d", len(contents))
	}
	if contents[1].Role != "model" {
		t.Fatalf("expected assistant role mapped to model, got %q", contents[1].Role)
	}

	resp := GeminiResponse{
		Candidates: []struct {
			Content struct {
				Parts []GeminiPart `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		}{
			{
				Content: struct {
					Parts []GeminiPart `json:"parts"`
				}{
					Parts: []GeminiPart{
						{Text: "thinking...", Thought: true},
						{Text: "final", Thought: false},
					},
				},
			},
		},
	}
	out := g.extractTextFromResponse(resp)
	if !strings.Contains(out, "final") {
		t.Fatalf("expected final output text")
	}
	if !strings.Contains(out, "[THOUGHT]") {
		t.Fatalf("expected thought text when VerboseThoughts enabled")
	}
}
