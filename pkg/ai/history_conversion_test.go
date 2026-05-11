package ai

import (
	"net/http"
	"testing"
)

func TestOpenAIConvertHistory(t *testing.T) {
	h := NewConversationHistory()
	h.AddSystemMessage("sys")
	h.AddUserMessage("u1")
	h.AddAssistantMessage("a1")
	h.AddToolResultMessage("tc-1", "tool", "tool-out")
	h.AddMessage("user", "") // should be skipped

	p := NewOpenAIProviderWithAuth("k", "", "gpt-4o")
	msgs := p.convertHistory(h)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages after skip, got %d", len(msgs))
	}
	if msgs[3].Role != "user" {
		t.Fatalf("expected tool role converted to user")
	}
}

func TestOpenRouterAndMoonshotConvertHistory(t *testing.T) {
	h := NewConversationHistory()
	h.AddSystemMessage("sys")
	h.AddToolResultMessage("tc-1", "tool", "tool-out")
	h.AddMessage("assistant", "")

	orMsgs := NewOpenRouterProvider("k", "").convertHistory(h)
	msMsgs := NewMoonshotProvider("k", "").convertHistory(h)
	if len(orMsgs) != 2 || len(msMsgs) != 2 {
		t.Fatalf("expected converted histories to skip empty assistant")
	}
	if orMsgs[1].Role != "user" || msMsgs[1].Role != "user" {
		t.Fatalf("expected tool role converted to user")
	}
}

func TestAnthropicConvertHistoryAndHeaders(t *testing.T) {
	h := NewConversationHistory()
	// Start with assistant to trigger prepended synthetic user.
	h.AddAssistantMessage("a1")
	h.AddAssistantMessage("a2")
	h.AddSystemMessage("sys1")
	h.AddSystemMessage("sys2")
	h.AddToolResultMessage("tc-1", "tool", "tool-out")

	p := NewAnthropicProviderWithAuth("api-key", "oauth-token", "claude-sonnet-4-5")
	sys, msgs := p.convertHistory(h)
	if sys != "sys1\n\nsys2" {
		t.Fatalf("unexpected merged system msg: %q", sys)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected converted anthropic messages")
	}
	if msgs[0].Role != "user" {
		t.Fatalf("expected synthetic leading user message")
	}

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	p.setHeaders(req)
	if req.Header.Get("Authorization") == "" || req.Header.Get("x-api-key") != "" {
		t.Fatalf("expected bearer auth header in oauth mode")
	}
}

func TestGrokHistoryConversions(t *testing.T) {
	h := NewConversationHistory()
	h.AddSystemMessage("sys")
	h.AddToolCallMessage([]ToolCallEntry{
		{ID: "c1", ToolName: "search", Arguments: `{"q":"x"}`},
	})
	h.AddToolResultMessage("c1", "search", "result")
	h.AddMessage("assistant", "")
	h.AddUserMessage("u1")

	p := NewGrokProvider("k", "grok-3-mini")
	msgs := p.convertHistoryToMessages(h)
	if len(msgs) == 0 {
		t.Fatalf("expected grok converted messages")
	}
	// Ensure tool result transformed to user context text.
	foundToolSurface := false
	for _, m := range msgs {
		if m.Role == "user" && len(m.Content) > 0 && m.Content[0] == '[' {
			foundToolSurface = true
			break
		}
	}
	if !foundToolSurface {
		t.Fatalf("expected transformed tool result in user role")
	}

	native := p.convertHistoryToNativeMsgs(h)
	if len(native) == 0 {
		t.Fatalf("expected native msg conversion")
	}
}
