// Package api — WebSocket Client for Real-time Updates
// Phase 3: Real-time messaging with automatic reconnection
package api

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketClient manages WebSocket connections for real-time updates.
type WebSocketClient struct {
	url          string
	conn         *websocket.Conn
	logger       *slog.Logger
	reconnect    bool
	maxRetries   int
	retryDelay   time.Duration
	handlers     map[string][]EventHandler
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	connected    bool
	messageQueue chan Message
}

// Message represents a WebSocket message.
type Message struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Time    int64                  `json:"time,omitempty"`
}

// EventHandler is a callback for WebSocket events.
type EventHandler func(msg *Message) error

// NewWebSocketClient creates a new WebSocket client.
func NewWebSocketClient(url string, logger *slog.Logger) *WebSocketClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketClient{
		url:          url,
		logger:       logger,
		reconnect:    true,
		maxRetries:   5,
		retryDelay:   1 * time.Second,
		handlers:     make(map[string][]EventHandler),
		ctx:          ctx,
		cancel:       cancel,
		messageQueue: make(chan Message, 100),
	}
}

// Connect establishes a WebSocket connection.
func (wc *WebSocketClient) Connect() error {
	var conn *websocket.Conn
	var err error

	for attempt := 0; attempt <= wc.maxRetries; attempt++ {
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}

		conn, _, err = dialer.Dial(wc.url, nil)
		if err == nil {
			break
		}

		if attempt < wc.maxRetries {
			backoff := time.Duration(1<<uint(attempt)) * wc.retryDelay
			wc.logger.Warn("WebSocket connection failed, retrying",
				"attempt", attempt+1,
				"backoff", backoff,
				"error", err)

			select {
			case <-time.After(backoff):
			case <-wc.ctx.Done():
				return wc.ctx.Err()
			}
		}
	}

	if err != nil {
		return fmt.Errorf("connect websocket: %w", err)
	}

	wc.conn = conn
	wc.connected = true

	wc.logger.Info("WebSocket connected", "url", wc.url)

	// Start reading messages
	go wc.readLoop()

	// Start processing message queue
	go wc.processQueue()

	return nil
}

// Disconnect closes the WebSocket connection.
func (wc *WebSocketClient) Disconnect() error {
	wc.cancel()
	wc.connected = false

	if wc.conn != nil {
		return wc.conn.Close()
	}

	return nil
}

// IsConnected checks if the WebSocket is connected.
func (wc *WebSocketClient) IsConnected() bool {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	return wc.connected
}

// Send sends a message through the WebSocket.
func (wc *WebSocketClient) Send(msg *Message) error {
	if !wc.IsConnected() {
		return fmt.Errorf("websocket not connected")
	}

	wc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return wc.conn.WriteJSON(msg)
}

// On registers a handler for a specific message type.
func (wc *WebSocketClient) On(msgType string, handler EventHandler) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.handlers[msgType] = append(wc.handlers[msgType], handler)
}

// readLoop continuously reads messages from the WebSocket.
func (wc *WebSocketClient) readLoop() {
	for {
		select {
		case <-wc.ctx.Done():
			return
		default:
		}

		var msg Message
		wc.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		err := wc.conn.ReadJSON(&msg)

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				wc.logger.Error("WebSocket error", "error", err)
			}

			wc.connected = false

			if wc.reconnect {
				wc.logger.Info("WebSocket reconnecting...")
				time.Sleep(2 * time.Second)
				if err := wc.Connect(); err != nil {
					wc.logger.Error("Reconnection failed", "error", err)
				}
			}

			return
		}

		// Queue message for processing
		select {
		case wc.messageQueue <- msg:
		case <-wc.ctx.Done():
			return
		default:
			wc.logger.Warn("Message queue full, dropping message", "type", msg.Type)
		}
	}
}

// processQueue handles queued messages.
func (wc *WebSocketClient) processQueue() {
	for {
		select {
		case msg := <-wc.messageQueue:
			wc.handleMessage(&msg)
		case <-wc.ctx.Done():
			return
		}
	}
}

// handleMessage dispatches messages to registered handlers.
func (wc *WebSocketClient) handleMessage(msg *Message) {
	wc.mu.RLock()
	handlers, ok := wc.handlers[msg.Type]
	wc.mu.RUnlock()

	if !ok {
		wc.logger.Warn("No handler for message type", "type", msg.Type)
		return
	}

	for _, handler := range handlers {
		if err := handler(msg); err != nil {
			wc.logger.Error("Handler error",
				"type", msg.Type,
				"error", err)
		}
	}
}

// ────────────────────────────────────────────────────────────
// Message Types
// ────────────────────────────────────────────────────────────

// TokenMessage represents a token stream message (chat response streaming).
type TokenMessage struct {
	RunID    string `json:"run_id"`
	Token    string `json:"token"`
	Sequence int    `json:"sequence"`
}

// RunStatusMessage represents a run status update.
type RunStatusMessage struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"` // running, complete, error
	Error  string `json:"error,omitempty"`
}

// ToolExecutionMessage represents tool execution notification.
type ToolExecutionMessage struct {
	RunID      string                 `json:"run_id"`
	ToolName   string                 `json:"tool_name"`
	Status     string                 `json:"status"` // start, complete, error
	Output     string                 `json:"output,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Duration   int64                  `json:"duration_ms,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// MemoryInjectionMessage represents memory injection notification.
type MemoryInjectionMessage struct {
	RunID      string  `json:"run_id"`
	MemoryID   string  `json:"memory_id"`
	Type       string  `json:"type"` // engram, fact, pattern
	Relevance  float64 `json:"relevance"`
	Content    string  `json:"content"`
}

// ArtifactGenerationMessage represents artifact generation notification.
type ArtifactGenerationMessage struct {
	RunID    string `json:"run_id"`
	ArtifactID string `json:"artifact_id"`
	Type     string `json:"type"` // code, image, document
	Content  string `json:"content"`
	Language string `json:"language,omitempty"`
}

// MetricsUpdateMessage represents metrics update.
type MetricsUpdateMessage struct {
	Timestamp int64                  `json:"timestamp"`
	Metrics   map[string]interface{} `json:"metrics"`
}

// HealthCheckMessage represents a health check.
type HealthCheckMessage struct {
	Status string `json:"status"`
	Time   int64  `json:"time"`
}
