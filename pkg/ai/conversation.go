package ai

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ToolCallEntry records a single native function call from an AI assistant message.
type ToolCallEntry struct {
	ID        string `json:"id"`        // xAI call_id
	ToolName  string `json:"tool_name"` // function name
	Arguments string `json:"arguments"` // JSON-encoded args string
}

// ConversationMessage represents a single message in the conversation
type ConversationMessage struct {
	Role      string // "system", "user", "assistant", "tool"
	Content   string
	Timestamp time.Time
	// Native function calling (populated only for the native-calling path).
	ToolCalls  []ToolCallEntry // role:"assistant" with tool calls (Content may be empty)
	ToolCallID string          // role:"tool" — correlates to ToolCallEntry.ID
	ToolName   string          // role:"tool" — tool name for the result
}

// ConversationHistory manages the conversation context
type ConversationHistory struct {
	messages []ConversationMessage
	mu       sync.RWMutex
}

// NewConversationHistory creates a new conversation history
func NewConversationHistory() *ConversationHistory {
	return &ConversationHistory{
		messages: make([]ConversationMessage, 0),
	}
}

// AddMessage adds a message to the conversation history
func (ch *ConversationHistory) AddMessage(role, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.messages = append(ch.messages, ConversationMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// AddSystemMessage adds a system message
func (ch *ConversationHistory) AddSystemMessage(content string) {
	ch.AddMessage("system", content)
}

// UpsertSystemMessage replaces the first system message whose content begins
// with tag, or appends a new system message when none is found.
// This allows dynamic memory context to be refreshed every turn without
// growing history: one pinned system message stays current instead of
// accumulating one system message per turn.
func (ch *ConversationHistory) UpsertSystemMessage(tag, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	for i, m := range ch.messages {
		if m.Role == "system" && strings.HasPrefix(m.Content, tag) {
			ch.messages[i].Content = content
			ch.messages[i].Timestamp = time.Now()
			return
		}
	}
	ch.messages = append(ch.messages, ConversationMessage{
		Role:      "system",
		Content:   content,
		Timestamp: time.Now(),
	})
}

// AddUserMessage adds a user message
func (ch *ConversationHistory) AddUserMessage(content string) {
	ch.AddMessage("user", content)
}

// AddAssistantMessage adds an assistant message
func (ch *ConversationHistory) AddAssistantMessage(content string) {
	ch.AddMessage("assistant", content)
}

// AddToolCallMessage adds an assistant message that contains native tool calls.
// Content is typically empty — the intent is expressed via ToolCalls.
func (ch *ConversationHistory) AddToolCallMessage(calls []ToolCallEntry) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.messages = append(ch.messages, ConversationMessage{
		Role:      "assistant",
		ToolCalls: calls,
		Timestamp: time.Now(),
	})
}

// AddToolResultMessage adds a tool result message (role:"tool") for the native path.
func (ch *ConversationHistory) AddToolResultMessage(toolCallID, toolName, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.messages = append(ch.messages, ConversationMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Timestamp:  time.Now(),
	})
}

// GetMessages returns all messages (thread-safe copy)
func (ch *ConversationHistory) GetMessages() []ConversationMessage {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	// Return a copy to prevent external modification
	messagesCopy := make([]ConversationMessage, len(ch.messages))
	copy(messagesCopy, ch.messages)
	return messagesCopy
}

// GetRecentMessages returns the last N messages
func (ch *ConversationHistory) GetRecentMessages(n int) []ConversationMessage {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if n <= 0 || n >= len(ch.messages) {
		messagesCopy := make([]ConversationMessage, len(ch.messages))
		copy(messagesCopy, ch.messages)
		return messagesCopy
	}

	start := len(ch.messages) - n
	messagesCopy := make([]ConversationMessage, n)
	copy(messagesCopy, ch.messages[start:])
	return messagesCopy
}

// Clear removes all messages from history
func (ch *ConversationHistory) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.messages = make([]ConversationMessage, 0)
}

// SetMessages completely replaces the internal message list.
func (ch *ConversationHistory) SetMessages(msgs []ConversationMessage) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.messages = make([]ConversationMessage, len(msgs))
	copy(ch.messages, msgs)
}

// Count returns the number of messages
func (ch *ConversationHistory) Count() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	return len(ch.messages)
}

// Truncate keeps only the most recent N messages
func (ch *ConversationHistory) Truncate(n int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if n <= 0 {
		ch.messages = make([]ConversationMessage, 0)
		return
	}

	if n >= len(ch.messages) {
		return // Already within limit
	}

	// Keep the most recent N messages
	ch.messages = ch.messages[len(ch.messages)-n:]
}

// EstimateTokens estimates token count (rough approximation: ~4 chars/token).
// Includes ToolCalls.Arguments so native function-calling messages are counted.
func (ch *ConversationHistory) EstimateTokens() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	totalChars := 0
	for _, msg := range ch.messages {
		totalChars += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			totalChars += len(tc.Arguments) + len(tc.ToolName) + 20 // +20 overhead per call
		}
	}

	return totalChars / 4
}

// TruncateToTokenLimit truncates history to fit within token limit.
// It prioritizes system messages and then keeps the most recent conversation
// messages. If a message is too large to fit in the remaining budget, it is
// truncated (keeping head and tail) rather than being dropped entirely.
func (ch *ConversationHistory) TruncateToTokenLimit(maxTokens int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 1. Preserve System Messages
	systemMessages := []ConversationMessage{}
	conversationMessages := []ConversationMessage{}

	for _, msg := range ch.messages {
		if msg.Role == "system" {
			systemMessages = append(systemMessages, msg)
		} else {
			conversationMessages = append(conversationMessages, msg)
		}
	}

	systemTokens := 0
	for _, msg := range systemMessages {
		systemTokens += len(msg.Content) / 4
	}

	availableTokens := maxTokens - systemTokens
	if availableTokens <= 0 {
		ch.messages = systemMessages
		return
	}

	// 2. Keep most recent conversation messages, but truncate them if they are too big.
	keptMessages := []ConversationMessage{}
	currentTokens := 0

	// We iterate backwards from newest to oldest.
	for i := len(conversationMessages) - 1; i >= 0; i-- {
		msg := conversationMessages[i]
		msgTokens := len(msg.Content) / 4
		for _, tc := range msg.ToolCalls {
			msgTokens += (len(tc.Arguments) + len(tc.ToolName)) / 4
		}

		// Minimum tokens to give to a message before we stop trying to keep it.
		const minMsgTokens = 100

		if currentTokens+minMsgTokens > availableTokens {
			break // No more room for even a tiny snippet.
		}

		remainingForThisMsg := availableTokens - currentTokens
		if msgTokens > remainingForThisMsg {
			// Sophisticated Truncation: Keep head and tail of the oversized message.
			// head: 60%, tail: 40% of the remaining space.
			targetChars := remainingForThisMsg * 4
			if targetChars > 200 {
				headChars := (targetChars * 6) / 10
				tailChars := targetChars - headChars - 100 // leave room for notice
				if tailChars < 0 {
					tailChars = 0
				}
				
				notice := fmt.Sprintf("\n\n[... %d characters truncated by Gorkbot Context Manager ...]\n\n", len(msg.Content)-targetChars)
				msg.Content = msg.Content[:headChars] + notice + msg.Content[len(msg.Content)-tailChars:]
				msgTokens = len(msg.Content) / 4
			} else {
				// Too small for head/tail, just take the head.
				msg.Content = msg.Content[:targetChars] + " [...]"
				msgTokens = len(msg.Content) / 4
			}
		}

		keptMessages = append([]ConversationMessage{msg}, keptMessages...)
		currentTokens += msgTokens
		
		if currentTokens >= availableTokens {
			break
		}
	}

	ch.messages = append(systemMessages, keptMessages...)
}

// RepairOrphanedPairs detects and repairs broken tool-call/result pairs that
// would cause LLM API 400 errors. Two invariants must hold:
//  1. Every role:"tool" message must have a corresponding assistant message
//     with a ToolCallEntry whose ID == message.ToolCallID.
//  2. Every assistant message with ToolCalls must have a corresponding
//     role:"tool" result for each call.
//
// Repair strategy:
//   - Orphaned tool results (no matching assistant call): removed.
//   - Orphaned assistant tool-call entries (result was dropped): stubbed with
//     a synthetic tool result: "[Tool result unavailable — pruned during context compression]"
//
// Returns the number of messages repaired (removed or stubbed).
func (ch *ConversationHistory) RepairOrphanedPairs() int {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	repaired := 0

	// Step 1: Build a map of all tool_call IDs that appear in assistant messages.
	// key: call ID, value: index of the assistant message in ch.messages
	assistantCallIDs := make(map[string]int) // call_id -> message index
	for i, msg := range ch.messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					assistantCallIDs[tc.ID] = i
				}
			}
		}
	}

	// Step 2: Build a map of all tool result IDs from role:"tool" messages.
	resultIDs := make(map[string]bool)
	for _, msg := range ch.messages {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			resultIDs[msg.ToolCallID] = true
		}
	}

	// Step 3: Remove role:"tool" messages whose ToolCallID doesn't exist in
	// the assistant call map.
	filtered := make([]ConversationMessage, 0, len(ch.messages))
	for _, msg := range ch.messages {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			if _, ok := assistantCallIDs[msg.ToolCallID]; !ok {
				// Orphaned tool result — drop it.
				repaired++
				continue
			}
		}
		filtered = append(filtered, msg)
	}
	ch.messages = filtered

	// Step 4: For each assistant ToolCallEntry whose ID is NOT in the result
	// map, insert a synthetic role:"tool" message right after that assistant
	// message.
	//
	// We iterate from the end so inserting doesn't shift the indices of
	// unprocessed earlier messages.
	for i := len(ch.messages) - 1; i >= 0; i-- {
		msg := ch.messages[i]
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		// Collect all call IDs that are missing a result, preserving order.
		var missing []ToolCallEntry
		for _, tc := range msg.ToolCalls {
			if tc.ID != "" && !resultIDs[tc.ID] {
				missing = append(missing, tc)
			}
		}
		if len(missing) == 0 {
			continue
		}
		// Build synthetic result messages (one per missing call).
		synthetics := make([]ConversationMessage, len(missing))
		for j, tc := range missing {
			synthetics[j] = ConversationMessage{
				Role:       "tool",
				Content:    "[Tool result unavailable — pruned during context compression]",
				ToolCallID: tc.ID,
				ToolName:   tc.ToolName,
				Timestamp:  msg.Timestamp,
			}
			repaired++
		}
		// Insert synthetics right after position i.
		tail := make([]ConversationMessage, len(ch.messages[i+1:]))
		copy(tail, ch.messages[i+1:])
		ch.messages = append(ch.messages[:i+1], synthetics...)
		ch.messages = append(ch.messages, tail...)
	}

	return repaired
}
