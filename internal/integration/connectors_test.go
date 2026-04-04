package integration

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockConnector is a test implementation of Connector.
type MockConnector struct {
	name        string
	msgChan     chan *Message
	healthy     bool
	sendError   error
	stopErr     error
}

func NewMockConnector(name string) *MockConnector {
	return &MockConnector{
		name:    name,
		msgChan: make(chan *Message, 10),
		healthy: true,
	}
}

func (mc *MockConnector) Name() string                          { return mc.name }
func (mc *MockConnector) Start(ctx context.Context) error      { return nil }
func (mc *MockConnector) Stop(ctx context.Context) error       { return mc.stopErr }
func (mc *MockConnector) MessageChan() <-chan *Message       { return mc.msgChan }
func (mc *MockConnector) IsHealthy(ctx context.Context) bool  { return mc.healthy }
func (mc *MockConnector) Send(ctx context.Context, resp *Response) error {
	if mc.sendError != nil {
		return mc.sendError
	}
	return nil
}

// TestConnectorRegistry tests the registry functionality.
func TestConnectorRegistry_Register(t *testing.T) {
	registry := NewConnectorRegistry()
	connector := NewMockConnector("test")

	registry.Register(connector)
	retrieved := registry.Get("test")

	assert.NotNil(t, retrieved)
	assert.Equal(t, "test", retrieved.Name())
}

func TestConnectorRegistry_List(t *testing.T) {
	registry := NewConnectorRegistry()
	registry.Register(NewMockConnector("test1"))
	registry.Register(NewMockConnector("test2"))

	connectors := registry.List()
	assert.Equal(t, 2, len(connectors))
}

func TestConnectorRegistry_StartAllStopAll(t *testing.T) {
	registry := NewConnectorRegistry()
	registry.Register(NewMockConnector("test1"))
	registry.Register(NewMockConnector("test2"))

	ctx := context.Background()
	err := registry.StartAll(ctx)
	assert.NoError(t, err)

	err = registry.StopAll(ctx)
	assert.NoError(t, err)
}

// TestTelegramConnector tests basic Telegram connector functionality.
func TestTelegramConnector_Name(t *testing.T) {
	tc := NewTelegramConnector("test_token", slog.Default())
	assert.Equal(t, "telegram", tc.Name())
}

func TestTelegramConnector_MessageChan(t *testing.T) {
	tc := NewTelegramConnector("test_token", slog.Default())
	msgChan := tc.MessageChan()
	assert.NotNil(t, msgChan)
}

// TestDiscordConnector tests basic Discord connector functionality.
func TestDiscordConnector_Name(t *testing.T) {
	dc := NewDiscordConnector("http://localhost:8080/webhook", 8080, slog.Default())
	assert.Equal(t, "discord", dc.Name())
}

func TestDiscordConnector_MessageChan(t *testing.T) {
	dc := NewDiscordConnector("http://localhost:8080/webhook", 8080, slog.Default())
	msgChan := dc.MessageChan()
	assert.NotNil(t, msgChan)
}

// TestEmailConnector tests basic Email connector functionality.
func TestEmailConnector_Name(t *testing.T) {
	ec := NewEmailConnector("smtp.example.com", 587, "user", "pass", "noreply@example.com", slog.Default())
	assert.Equal(t, "email", ec.Name())
}

func TestEmailConnector_MessageChan(t *testing.T) {
	ec := NewEmailConnector("smtp.example.com", 587, "user", "pass", "noreply@example.com", slog.Default())
	msgChan := ec.MessageChan()
	assert.NotNil(t, msgChan)
}

// TestMessage_Structure tests the Message struct.
func TestMessage_Structure(t *testing.T) {
	msg := &Message{
		ID:       "msg_123",
		Source:   "telegram",
		SourceID: "user_456",
		Username: "john_doe",
		Text:     "Hello, Gorkbot!",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	assert.Equal(t, "msg_123", msg.ID)
	assert.Equal(t, "telegram", msg.Source)
	assert.Equal(t, "user_456", msg.SourceID)
	assert.Equal(t, "john_doe", msg.Username)
	assert.Equal(t, "Hello, Gorkbot!", msg.Text)
	assert.Equal(t, "value", msg.Metadata["key"])
}

// TestResponse_Structure tests the Response struct.
func TestResponse_Structure(t *testing.T) {
	resp := &Response{
		SourceID: "user_456",
		Text:     "Hello, user!",
		Metadata: map[string]string{
			"channel": "general",
		},
		Files: []Attachment{
			{
				Filename: "report.pdf",
				Size:     1024,
			},
		},
	}

	assert.Equal(t, "user_456", resp.SourceID)
	assert.Equal(t, "Hello, user!", resp.Text)
	assert.Equal(t, 1, len(resp.Files))
	assert.Equal(t, "report.pdf", resp.Files[0].Filename)
}

// TestConnectorIntegration tests multiple connectors working together.
func TestConnectorIntegration_MultipleConnectors(t *testing.T) {
	registry := NewConnectorRegistry()

	// Create mock connectors
	tg := NewMockConnector("telegram")
	dc := NewMockConnector("discord")
	ec := NewMockConnector("email")

	registry.Register(tg)
	registry.Register(dc)
	registry.Register(ec)

	ctx := context.Background()

	// Start all
	err := registry.StartAll(ctx)
	require.NoError(t, err)

	// Send response via each
	resp := &Response{
		SourceID: "user_123",
		Text:     "Test message",
	}

	for _, connector := range registry.List() {
		err := connector.Send(ctx, resp)
		assert.NoError(t, err)
	}

	// Stop all
	err = registry.StopAll(ctx)
	require.NoError(t, err)
}

// BenchmarkMessage creation
func BenchmarkMessage_Create(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Message{
			ID:        "msg_123",
			Source:    "telegram",
			SourceID:  "user_456",
			Username:  "john_doe",
			Text:      "Hello, Gorkbot!",
			Timestamp: time.Now(),
			Metadata:  make(map[string]string),
		}
	}
}

// BenchmarkResponse creation
func BenchmarkResponse_Create(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Response{
			SourceID: "user_456",
			Text:     "Hello, user!",
			Metadata: make(map[string]string),
		}
	}
}
