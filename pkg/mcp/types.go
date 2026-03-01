// Package mcp implements the Model Context Protocol (MCP) client.
//
// MCP is a standard protocol for connecting AI models to external tool servers.
// Each MCP server exposes a set of tools over a JSON-RPC 2.0 transport (stdio or SSE).
// This client discovers servers from a config file, starts them as child processes,
// and wraps their tools as standard Gorkbot tools so they appear alongside built-in ones.
package mcp

import (
	"encoding/json"
	"fmt"
)

// Transport describes how to connect to an MCP server.
type Transport string

const (
	TransportStdio Transport = "stdio" // spawn a child process, communicate over stdin/stdout
	TransportSSE   Transport = "sse"   // HTTP Server-Sent Events (for remote servers)
)

// ServerConfig describes one MCP server in the config file.
type ServerConfig struct {
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	Command   string            `json:"command,omitempty"` // for stdio: executable path
	Args      []string          `json:"args,omitempty"`    // for stdio: arguments
	URL       string            `json:"url,omitempty"`     // for SSE: server URL
	Env       map[string]string `json:"env,omitempty"`     // extra environment variables
	Disabled  bool              `json:"disabled,omitempty"`
}

// Config is the top-level config file structure (~/.config/gorkbot/mcp.json).
type Config struct {
	Servers []ServerConfig `json:"servers"`
	Version string         `json:"version"`
}

// ── JSON-RPC 2.0 wire types ──────────────────────────────────────────────────

// Request is a JSON-RPC 2.0 request sent to an MCP server.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response received from an MCP server.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// ── MCP-specific protocol types ──────────────────────────────────────────────

// InitializeParams is sent during the MCP handshake.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ClientCaps `json:"capabilities"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// ClientCaps describes what the client supports.
type ClientCaps struct {
	Sampling *struct{} `json:"sampling,omitempty"`
}

// ClientInfo identifies this MCP client to the server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to Initialize.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ServerCaps `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
}

// ServerCaps describes what the server supports.
type ServerCaps struct {
	Tools    *ToolsCap    `json:"tools,omitempty"`
	Prompts  *PromptsCap  `json:"prompts,omitempty"`
	Resources *ResourcesCap `json:"resources,omitempty"`
}

// ToolsCap indicates the server exposes callable tools.
type ToolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCap indicates the server exposes prompt templates.
type PromptsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCap indicates the server exposes readable resources.
type ResourcesCap struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolDefinition is returned by tools/list.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// ListToolsResult is the result of a tools/list RPC call.
type ListToolsResult struct {
	Tools  []ToolDefinition `json:"tools"`
	Cursor string           `json:"nextCursor,omitempty"`
}

// CallToolParams is the params for a tools/call RPC call.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolContent is one element in a tool call result.
type ToolContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// CallToolResult is the result of a tools/call RPC call.
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
