// Package mcp implements the Model Context Protocol (MCP) for Gorkbot.
// MCP enables Gorkbot to act as both a server (exposing capabilities) and a client (consuming resources).
package mcp

import (
	"context"
	"encoding/json"
	"time"
)

// MessageType identifies the MCP message type.
type MessageType string

const (
	MessageTypeRequest      MessageType = "request"
	MessageTypeResponse     MessageType = "response"
	MessageTypeNotification MessageType = "notification"
	MessageTypeError        MessageType = "error"
)

// Message is the base MCP message format.
type Message struct {
	Type      MessageType     `json:"type"`
	ID        string          `json:"id"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *ErrorDetail    `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// ErrorDetail provides error information.
type ErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// ServerCapabilities describes what the MCP server can do.
type ServerCapabilities struct {
	Tools     ToolCapability     `json:"tools"`
	Resources ResourceCapability `json:"resources"`
	Prompts   PromptCapability   `json:"prompts"`
}

// ToolCapability describes available tools.
type ToolCapability struct {
	ListChanged bool `json:"listChanged"`
	Count       int  `json:"count"`
}

// ResourceCapability describes available resources.
type ResourceCapability struct {
	Subscribe   bool `json:"subscribe"`
	ListChanged bool `json:"listChanged"`
	Count       int  `json:"count"`
}

// PromptCapability describes available prompts.
type PromptCapability struct {
	ListChanged bool `json:"listChanged"`
	Count       int  `json:"count"`
}

// Tool describes an available tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Resource describes an available resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// Prompt describes an available prompt template.
type Prompt struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ServerHandler defines the interface for MCP server implementations.
type ServerHandler interface {
	// Initialize handles the initial handshake.
	Initialize(ctx context.Context) error

	// ListTools returns available tools.
	ListTools(ctx context.Context) ([]Tool, error)

	// UseTool executes a tool.
	UseTool(ctx context.Context, name string, args json.RawMessage) (interface{}, error)

	// ListResources returns available resources.
	ListResources(ctx context.Context) ([]Resource, error)

	// ReadResource retrieves a resource.
	ReadResource(ctx context.Context, uri string) (interface{}, error)

	// ListPrompts returns available prompts.
	ListPrompts(ctx context.Context) ([]Prompt, error)

	// GetPrompt retrieves a prompt.
	GetPrompt(ctx context.Context, name string, args map[string]string) (interface{}, error)
}
