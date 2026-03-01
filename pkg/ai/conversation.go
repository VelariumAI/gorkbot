package ai

import (
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
	Role      string    // "system", "user", "assistant", "tool"
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

// EstimateTokens estimates token count (rough approximation)
// Average ~4 characters per token
func (ch *ConversationHistory) EstimateTokens() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	totalChars := 0
	for _, msg := range ch.messages {
		totalChars += len(msg.Content)
	}

	return totalChars / 4
}

// TruncateToTokenLimit truncates history to fit within token limit
// Keeps system messages and most recent conversation
func (ch *ConversationHistory) TruncateToTokenLimit(maxTokens int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// Separate system messages from conversation
	systemMessages := []ConversationMessage{}
	conversationMessages := []ConversationMessage{}

	for _, msg := range ch.messages {
		if msg.Role == "system" {
			systemMessages = append(systemMessages, msg)
		} else {
			conversationMessages = append(conversationMessages, msg)
		}
	}

	// Calculate tokens for system messages
	systemTokens := 0
	for _, msg := range systemMessages {
		systemTokens += len(msg.Content) / 4
	}

	// Available tokens for conversation
	availableTokens := maxTokens - systemTokens
	if availableTokens <= 0 {
		// Only keep system messages if they exceed limit
		ch.messages = systemMessages
		return
	}

	// Keep most recent conversation messages that fit.
	// Use continue (not break) so a single oversized message doesn't silently
	// drop all older messages — we skip it and keep trying smaller older ones.
	keptMessages := []ConversationMessage{}
	currentTokens := 0

	for i := len(conversationMessages) - 1; i >= 0; i-- {
		msgTokens := len(conversationMessages[i].Content) / 4
		if currentTokens+msgTokens > availableTokens {
			continue // skip this oversized message, keep scanning older ones
		}
		keptMessages = append([]ConversationMessage{conversationMessages[i]}, keptMessages...)
		currentTokens += msgTokens
	}

	// Rebuild messages: system messages + kept conversation
	ch.messages = append(systemMessages, keptMessages...)
}
