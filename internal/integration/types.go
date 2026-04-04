// Package integration provides third-party integrations for Gorkbot.
// It handles routing messages from external platforms (Telegram, Discord, Email, SMS)
// to the Gorkbot API and returning responses back to the source.
package integration

import (
	"context"
	"time"
)

// Message represents a unified message from any external source.
type Message struct {
	ID          string            // Unique message ID
	Source      string            // "telegram", "discord", "email", "sms"
	SourceID    string            // User/channel ID on the source platform
	Username    string            // Display name
	Text        string            // Message content
	Timestamp   time.Time         // When message was received
	Metadata    map[string]string // Platform-specific metadata
	ReplyTo     string            // Optional: ID of message being replied to
	Attachments []Attachment      // Optional: files, images, etc.
}

// Attachment represents a file or media attachment.
type Attachment struct {
	Filename string
	MimeType string
	Size     int64
	URL      string // Downloadable URL
	Data     []byte // Optional: inline data
}

// Response represents a response to send back to a user.
type Response struct {
	SourceID string            // Target user/channel
	Text     string            // Response text
	Metadata map[string]string // Platform-specific metadata
	Files    []Attachment      // Optional: files to send
}

// Connector is the interface all platform integrations must implement.
type Connector interface {
	// Name returns the connector's name ("telegram", "discord", "email").
	Name() string

	// Start initializes the connector and begins listening for messages.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the connector.
	Stop(ctx context.Context) error

	// Send sends a response back to the source platform.
	Send(ctx context.Context, resp *Response) error

	// MessageChan returns the channel where incoming messages are sent.
	MessageChan() <-chan *Message

	// IsHealthy returns true if the connector is functioning properly.
	IsHealthy(ctx context.Context) bool
}

// ConnectorRegistry manages all active connectors.
type ConnectorRegistry struct {
	connectors map[string]Connector
}

// NewConnectorRegistry creates a new registry.
func NewConnectorRegistry() *ConnectorRegistry {
	return &ConnectorRegistry{
		connectors: make(map[string]Connector),
	}
}

// Register adds a connector to the registry.
func (cr *ConnectorRegistry) Register(connector Connector) {
	cr.connectors[connector.Name()] = connector
}

// Get returns a connector by name.
func (cr *ConnectorRegistry) Get(name string) Connector {
	return cr.connectors[name]
}

// List returns all registered connectors.
func (cr *ConnectorRegistry) List() []Connector {
	var result []Connector
	for _, c := range cr.connectors {
		result = append(result, c)
	}
	return result
}

// StartAll starts all registered connectors.
func (cr *ConnectorRegistry) StartAll(ctx context.Context) error {
	for _, connector := range cr.connectors {
		if err := connector.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StopAll stops all registered connectors.
func (cr *ConnectorRegistry) StopAll(ctx context.Context) error {
	for _, connector := range cr.connectors {
		if err := connector.Stop(ctx); err != nil {
			// Continue stopping others even if one fails
			continue
		}
	}
	return nil
}
