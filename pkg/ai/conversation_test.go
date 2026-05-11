package ai

import "testing"

func TestConversationBasics(t *testing.T) {
	ch := NewConversationHistory()
	ch.AddSystemMessage("sys")
	ch.AddUserMessage("u1")
	ch.AddAssistantMessage("a1")
	ch.UpsertSystemMessage("[MEM]", "[MEM] first")
	ch.UpsertSystemMessage("[MEM]", "[MEM] second")

	msgs := ch.GetMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[3].Role != "system" || msgs[3].Content != "[MEM] second" {
		t.Fatalf("expected upserted system message")
	}
	if ch.Count() != 4 {
		t.Fatalf("expected count 4")
	}
}

func TestConversationTruncateAndRecent(t *testing.T) {
	ch := NewConversationHistory()
	for i := 0; i < 5; i++ {
		ch.AddUserMessage("u")
	}
	if len(ch.GetRecentMessages(2)) != 2 {
		t.Fatalf("expected recent length 2")
	}
	ch.Truncate(3)
	if ch.Count() != 3 {
		t.Fatalf("expected count 3 after truncate")
	}
	ch.Truncate(0)
	if ch.Count() != 0 {
		t.Fatalf("expected empty after truncate(0)")
	}
}

func TestConversationSetAndClear(t *testing.T) {
	ch := NewConversationHistory()
	ch.SetMessages([]ConversationMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	})
	if ch.Count() != 2 {
		t.Fatalf("expected count 2 after SetMessages")
	}
	ch.Clear()
	if ch.Count() != 0 {
		t.Fatalf("expected empty history after Clear")
	}
}

func TestConversationEstimateAndTokenLimitTruncation(t *testing.T) {
	ch := NewConversationHistory()
	ch.AddSystemMessage("system-keep")
	long := ""
	for i := 0; i < 4000; i++ {
		long += "x"
	}
	ch.AddUserMessage(long)
	ch.AddAssistantMessage(long)

	if ch.EstimateTokens() <= 0 {
		t.Fatalf("expected positive token estimate")
	}

	ch.TruncateToTokenLimit(200)
	msgs := ch.GetMessages()
	if len(msgs) == 0 {
		t.Fatalf("expected messages preserved after token truncation")
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected system message preserved first")
	}
}

func TestRepairOrphanedPairs(t *testing.T) {
	ch := NewConversationHistory()
	ch.AddToolCallMessage([]ToolCallEntry{
		{ID: "call-1", ToolName: "search", Arguments: `{"q":"x"}`},
		{ID: "call-2", ToolName: "calc", Arguments: `{"x":1}`},
	})
	// Valid tool result for call-1
	ch.AddToolResultMessage("call-1", "search", "ok")
	// Orphan tool result with no matching assistant call (should be removed)
	ch.AddToolResultMessage("orphan", "ghost", "bad")

	repaired := ch.RepairOrphanedPairs()
	if repaired < 2 {
		t.Fatalf("expected at least 2 repairs (remove orphan + stub missing), got %d", repaired)
	}

	msgs := ch.GetMessages()
	seenCall1 := false
	seenCall2Stub := false
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "call-1" {
			seenCall1 = true
		}
		if m.Role == "tool" && m.ToolCallID == "call-2" {
			seenCall2Stub = true
		}
		if m.Role == "tool" && m.ToolCallID == "orphan" {
			t.Fatalf("orphan result was not removed")
		}
	}
	if !seenCall1 || !seenCall2Stub {
		t.Fatalf("expected both real and stubbed tool results to exist")
	}
}
