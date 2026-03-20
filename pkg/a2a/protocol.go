package a2a

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// MessageType defines the type of A2A message
type MessageType string

const (
	// MessageTypeQuery - Agent requests information/advice from another agent
	MessageTypeQuery MessageType = "query"

	// MessageTypeResponse - Agent responds to a query
	MessageTypeResponse MessageType = "response"

	// MessageTypeNotification - Agent sends a notification (no response expected)
	MessageTypeNotification MessageType = "notification"

	// MessageTypeToolRequest - Agent requests tool execution
	MessageTypeToolRequest MessageType = "tool_request"
)

// Message represents a message between agents
type Message struct {
	ID        string                 `json:"id"`
	Type      MessageType            `json:"type"`
	From      string                 `json:"from"` // "grok" or "gemini"
	To        string                 `json:"to"`   // "grok" or "gemini"
	Content   string                 `json:"content"`
	Context   map[string]interface{} `json:"context,omitempty"`
	ReplyTo   string                 `json:"reply_to,omitempty"` // ID of message being replied to
	Timestamp time.Time              `json:"timestamp"`
}

// Channel manages A2A communication between two agents
type Channel struct {
	grokProvider   ai.AIProvider
	geminiProvider ai.AIProvider
	messages       []Message
	pendingReplies map[string]chan Message
	mu             sync.RWMutex
	// journal, when non-nil, durably persists outbound messages so they
	// survive a crash and can be re-dispatched on the next startup.
	journal *PendingJournal
}

// NewChannel creates a new A2A communication channel
func NewChannel(grok, gemini ai.AIProvider) *Channel {
	return &Channel{
		grokProvider:   grok,
		geminiProvider: gemini,
		messages:       make([]Message, 0),
		pendingReplies: make(map[string]chan Message),
	}
}

// NewChannelWithJournal creates a Channel that durably journals every
// outbound message to pendingDir before delivery, enabling crash recovery
// via RecoverAndRedispatch.
func NewChannelWithJournal(grok, gemini ai.AIProvider, pendingDir string) (*Channel, error) {
	j, err := NewPendingJournal(pendingDir)
	if err != nil {
		return nil, err
	}
	return &Channel{
		grokProvider:   grok,
		geminiProvider: gemini,
		messages:       make([]Message, 0),
		pendingReplies: make(map[string]chan Message),
		journal:        j,
	}, nil
}

// RecoverAndRedispatch re-sends any messages that were journaled but not yet
// delivered (e.g. due to a crash). Call once on startup before accepting new
// messages. The provided dispatch function is responsible for actual delivery;
// a nil error return deletes the pending record.
func (c *Channel) RecoverAndRedispatch(dispatch func(msg Message) error) error {
	if c.journal == nil {
		return nil
	}
	msgs, err := c.journal.RecoverPending()
	if err != nil {
		// Log but don't abort — partial recovery is better than none.
		_ = err
	}
	for _, msg := range msgs {
		if deliverErr := dispatch(msg); deliverErr == nil {
			_ = c.journal.DeletePending(msg.ID)
		}
	}
	return nil
}

// SendQuery sends a query from one agent to another and waits for response
func (c *Channel) SendQuery(ctx context.Context, from, to, content string, context map[string]interface{}) (string, error) {
	msg := Message{
		ID:        generateMessageID(),
		Type:      MessageTypeQuery,
		From:      from,
		To:        to,
		Content:   content,
		Context:   context,
		Timestamp: time.Now(),
	}

	// Durably journal the outbound message before attempting delivery so it
	// can be recovered if the process crashes mid-flight.
	if c.journal != nil {
		_ = c.journal.WritePending(msg) // best-effort; non-fatal
	}

	// Store message in-memory.
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	c.mu.Unlock()

	// Get the target provider
	var provider ai.AIProvider
	if to == "gemini" {
		provider = c.geminiProvider
	} else {
		provider = c.grokProvider
	}

	// Format query with context
	fullQuery := formatQuery(msg)

	// Send to AI provider
	response, err := provider.Generate(ctx, fullQuery)
	if err != nil {
		// Leave the pending journal entry intact — RecoverAndRedispatch will
		// retry it on next startup.
		return "", fmt.Errorf("failed to get response from %s: %w", to, err)
	}

	// Delivery confirmed — remove pending record.
	if c.journal != nil {
		_ = c.journal.DeletePending(msg.ID)
	}

	// Store response
	responseMsg := Message{
		ID:        generateMessageID(),
		Type:      MessageTypeResponse,
		From:      to,
		To:        from,
		Content:   response,
		ReplyTo:   msg.ID,
		Timestamp: time.Now(),
	}

	c.mu.Lock()
	c.messages = append(c.messages, responseMsg)
	c.mu.Unlock()

	return response, nil
}

// SendNotification sends a one-way notification (no response expected)
func (c *Channel) SendNotification(from, to, content string) {
	msg := Message{
		ID:        generateMessageID(),
		Type:      MessageTypeNotification,
		From:      from,
		To:        to,
		Content:   content,
		Timestamp: time.Now(),
	}

	c.mu.Lock()
	c.messages = append(c.messages, msg)
	c.mu.Unlock()
}

// GetConversationHistory returns the message history
func (c *Channel) GetConversationHistory() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	history := make([]Message, len(c.messages))
	copy(history, c.messages)
	return history
}

// ClearHistory clears the message history
func (c *Channel) ClearHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = make([]Message, 0)
}

// formatQuery formats a message as a query to the AI
func formatQuery(msg Message) string {
	query := fmt.Sprintf("Agent %s asks: %s", msg.From, msg.Content)

	if len(msg.Context) > 0 {
		query += "\n\nContext:"
		for key, value := range msg.Context {
			query += fmt.Sprintf("\n- %s: %v", key, value)
		}
	}

	return query
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
