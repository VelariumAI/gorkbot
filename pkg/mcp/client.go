package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ManagedConnection represents a connection to a remote MCP server.
type ManagedConnection struct {
	url         string
	logger      *slog.Logger
	connected   int32
	lastError   string
	mu          sync.RWMutex
	messageID   int64
	pendingReqs map[string]*pendingRequest
	timeout     time.Duration
}

// pendingRequest tracks an in-flight request.
type pendingRequest struct {
	done   chan *Message
	timer  *time.Timer
}

// NewManagedConnection creates a new managed connection to an MCP server.
func NewManagedConnection(serverURL string, logger *slog.Logger) (*ManagedConnection, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Validate URL
	if _, err := url.Parse(serverURL); err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	return &ManagedConnection{
		url:         serverURL,
		logger:      logger,
		timeout:     30 * time.Second,
		pendingReqs: make(map[string]*pendingRequest),
	}, nil
}

// Connect establishes a connection to the MCP server.
func (mc *ManagedConnection) Connect(ctx context.Context) error {
	// Simulate connection establishment
	// In production, this would be WebSocket or HTTP connection
	
	if !atomic.CompareAndSwapInt32(&mc.connected, 0, 1) {
		return fmt.Errorf("already connected to %s", mc.url)
	}

	mc.logger.Info("connected to MCP server", "url", mc.url)
	return nil
}

// Disconnect closes the connection.
func (mc *ManagedConnection) Disconnect() error {
	if !atomic.CompareAndSwapInt32(&mc.connected, 1, 0) {
		return fmt.Errorf("not connected to %s", mc.url)
	}

	mc.mu.Lock()
	// Cancel all pending requests
	for id, req := range mc.pendingReqs {
		if req.timer != nil {
			req.timer.Stop()
		}
		delete(mc.pendingReqs, id)
	}
	mc.mu.Unlock()

	mc.logger.Info("disconnected from MCP server", "url", mc.url)
	return nil
}

// IsConnected returns true if connected to the server.
func (mc *ManagedConnection) IsConnected() bool {
	return atomic.LoadInt32(&mc.connected) == 1
}

// ListTools retrieves available tools from the server.
func (mc *ManagedConnection) ListTools(ctx context.Context) ([]Tool, error) {
	msg := &Message{
		Type:      MessageTypeRequest,
		ID:        mc.nextMessageID(),
		Method:    "tools/list",
		Timestamp: time.Now(),
	}

	resp, err := mc.sendRequest(ctx, msg)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode tools: %w", err)
	}

	return result.Tools, nil
}

// UseTool executes a tool on the remote server.
func (mc *ManagedConnection) UseTool(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	paramsJSON, _ := json.Marshal(params)

	msg := &Message{
		Type:      MessageTypeRequest,
		ID:        mc.nextMessageID(),
		Method:    "tools/use",
		Params:    paramsJSON,
		Timestamp: time.Now(),
	}

	resp, err := mc.sendRequest(ctx, msg)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode tool result: %w", err)
	}

	return result, nil
}

// ListResources retrieves available resources from the server.
func (mc *ManagedConnection) ListResources(ctx context.Context) ([]Resource, error) {
	msg := &Message{
		Type:      MessageTypeRequest,
		ID:        mc.nextMessageID(),
		Method:    "resources/list",
		Timestamp: time.Now(),
	}

	resp, err := mc.sendRequest(ctx, msg)
	if err != nil {
		return nil, err
	}

	var result struct {
		Resources []Resource `json:"resources"`
	}

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode resources: %w", err)
	}

	return result.Resources, nil
}

// ReadResource retrieves a specific resource from the server.
func (mc *ManagedConnection) ReadResource(ctx context.Context, uri string) (interface{}, error) {
	params := map[string]string{
		"uri": uri,
	}

	paramsJSON, _ := json.Marshal(params)

	msg := &Message{
		Type:      MessageTypeRequest,
		ID:        mc.nextMessageID(),
		Method:    "resources/read",
		Params:    paramsJSON,
		Timestamp: time.Now(),
	}

	resp, err := mc.sendRequest(ctx, msg)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to decode resource: %w", err)
	}

	return result, nil
}

// sendRequest sends a request and waits for a response with timeout.
func (mc *ManagedConnection) sendRequest(ctx context.Context, msg *Message) (*Message, error) {
	if !mc.IsConnected() {
		return nil, fmt.Errorf("not connected to %s", mc.url)
	}

	done := make(chan *Message, 1)
	timer := time.AfterFunc(mc.timeout, func() {
		// Timeout cleanup
		mc.mu.Lock()
		delete(mc.pendingReqs, msg.ID)
		mc.mu.Unlock()
		done <- &Message{
			Type: MessageTypeError,
			Error: &ErrorDetail{
				Code:    408,
				Message: "request timeout",
			},
		}
	})

	mc.mu.Lock()
	mc.pendingReqs[msg.ID] = &pendingRequest{
		done:  done,
		timer: timer,
	}
	mc.mu.Unlock()

	// Simulate sending (in production, would write to connection)
	mc.logger.Debug("sending MCP request", "method", msg.Method, "id", msg.ID)

	select {
	case resp := <-done:
		if resp.Type == MessageTypeError {
			return nil, fmt.Errorf("error from server: %s (code %d)", resp.Error.Message, resp.Error.Code)
		}
		if timer != nil {
			timer.Stop()
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// nextMessageID generates a unique message ID.
func (mc *ManagedConnection) nextMessageID() string {
	id := atomic.AddInt64(&mc.messageID, 1)
	return fmt.Sprintf("msg_%d", id)
}

// GetURL returns the server URL.
func (mc *ManagedConnection) GetURL() string {
	return mc.url
}

// GetLastError returns the last error (if any).
func (mc *ManagedConnection) GetLastError() string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.lastError
}
