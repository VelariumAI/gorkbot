package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockServerHandler implements ServerHandler for testing.
type MockServerHandler struct {
	initCalled bool
}

func (m *MockServerHandler) Initialize(ctx context.Context) error {
	m.initCalled = true
	return nil
}

func (m *MockServerHandler) ListTools(ctx context.Context) ([]Tool, error) {
	return []Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}, nil
}

func (m *MockServerHandler) UseTool(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
	return map[string]string{"output": "success"}, nil
}

func (m *MockServerHandler) ListResources(ctx context.Context) ([]Resource, error) {
	return []Resource{
		{
			URI:         "file:///etc/config",
			Name:        "config",
			Description: "System config",
		},
	}, nil
}

func (m *MockServerHandler) ReadResource(ctx context.Context, uri string) (interface{}, error) {
	return map[string]string{"content": "resource content"}, nil
}

func (m *MockServerHandler) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return []Prompt{
		{
			Name:        "debug",
			Description: "Debug prompt",
		},
	}, nil
}

func (m *MockServerHandler) GetPrompt(ctx context.Context, name string, args map[string]string) (interface{}, error) {
	return map[string]string{"prompt": "debug output"}, nil
}

// TestServer_RegisterClient tests client registration.
func TestServer_RegisterClient(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	client, err := server.RegisterClient("client_123")
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "client_123", client.ID)
	assert.True(t, client.Connected)
}

// TestServer_RegisterClientDuplicate tests duplicate registration.
func TestServer_RegisterClientDuplicate(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	_, err := server.RegisterClient("client_123")
	assert.NoError(t, err)

	_, err = server.RegisterClient("client_123")
	assert.Error(t, err)
}

// TestServer_HandleInitialize tests initialize message.
func TestServer_HandleInitialize(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	msg := &Message{
		ID:     "msg_1",
		Method: "initialize",
	}

	resp, err := server.HandleMessage(context.Background(), "client_123", msg)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeResponse, resp.Type)
	assert.NotNil(t, resp.Result)
	assert.True(t, handler.initCalled)
}

// TestServer_HandleListTools tests list tools message.
func TestServer_HandleListTools(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	msg := &Message{
		ID:     "msg_2",
		Method: "tools/list",
	}

	resp, err := server.HandleMessage(context.Background(), "client_123", msg)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeResponse, resp.Type)

	var result struct {
		Tools []Tool `json:"tools"`
	}
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.Tools))
	assert.Equal(t, "read_file", result.Tools[0].Name)
}

// TestServer_HandleUseTool tests tool execution message.
func TestServer_HandleUseTool(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	params := map[string]interface{}{
		"name":      "read_file",
		"arguments": json.RawMessage(`{"file":"test.txt"}`),
	}
	paramsJSON, _ := json.Marshal(params)

	msg := &Message{
		ID:     "msg_3",
		Method: "tools/use",
		Params: paramsJSON,
	}

	resp, err := server.HandleMessage(context.Background(), "client_123", msg)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeResponse, resp.Type)
	assert.NotNil(t, resp.Result)
}

// TestServer_HandleUnknownMethod tests unknown method.
func TestServer_HandleUnknownMethod(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	msg := &Message{
		ID:     "msg_4",
		Method: "unknown/method",
	}

	resp, err := server.HandleMessage(context.Background(), "client_123", msg)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeError, resp.Type)
	assert.Equal(t, 404, resp.Error.Code)
}

// TestServer_GetConnectedClients tests getting client list.
func TestServer_GetConnectedClients(t *testing.T) {
	handler := &MockServerHandler{}
	server := NewServer(handler, slog.Default())

	server.RegisterClient("client_1")
	server.RegisterClient("client_2")

	clients := server.GetConnectedClients()
	assert.Equal(t, 2, len(clients))
}
