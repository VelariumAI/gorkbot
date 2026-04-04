package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Server implements the MCP server role for Gorkbot.
// It exposes tools, resources, and prompts to external MCP clients.
type Server struct {
	handler ServerHandler
	logger  *slog.Logger
	mu      sync.RWMutex
	clients map[string]*Client // Connected clients by ID
}

// Client represents a connected MCP client.
type Client struct {
	ID        string
	Connected bool
	LastSeen  time.Time
}

// NewServer creates a new MCP server.
func NewServer(handler ServerHandler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		handler: handler,
		logger:  logger,
		clients: make(map[string]*Client),
	}
}

// RegisterClient registers a new client connection.
func (s *Server) RegisterClient(clientID string) (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.clients[clientID]; exists {
		return nil, fmt.Errorf("client already registered: %s", clientID)
	}

	client := &Client{
		ID:        clientID,
		Connected: true,
		LastSeen:  time.Now(),
	}

	s.clients[clientID] = client
	s.logger.Info("client registered", "client_id", clientID)

	return client, nil
}

// UnregisterClient removes a client.
func (s *Server) UnregisterClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, clientID)
	s.logger.Info("client unregistered", "client_id", clientID)
}

// HandleMessage processes an incoming MCP message from a client.
func (s *Server) HandleMessage(ctx context.Context, clientID string, msg *Message) (*Message, error) {
	// Update client last seen
	s.mu.Lock()
	if client, exists := s.clients[clientID]; exists {
		client.LastSeen = time.Now()
	}
	s.mu.Unlock()

	response := &Message{
		Type:      MessageTypeResponse,
		ID:        msg.ID,
		Timestamp: time.Now(),
	}

	switch msg.Method {
	case "initialize":
		return s.handleInitialize(ctx, msg, response)
	case "tools/list":
		return s.handleListTools(ctx, msg, response)
	case "tools/use":
		return s.handleUseTool(ctx, msg, response)
	case "resources/list":
		return s.handleListResources(ctx, msg, response)
	case "resources/read":
		return s.handleReadResource(ctx, msg, response)
	case "prompts/list":
		return s.handleListPrompts(ctx, msg, response)
	case "prompts/get":
		return s.handleGetPrompt(ctx, msg, response)
	case "ping":
		return s.handlePing(ctx, msg, response)
	default:
		return s.errorResponse(response, 404, fmt.Sprintf("unknown method: %s", msg.Method)), nil
	}
}

// handleInitialize processes an initialize request.
func (s *Server) handleInitialize(ctx context.Context, req, resp *Message) (*Message, error) {
	if err := s.handler.Initialize(ctx); err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	// Build capabilities from handler
	tools, _ := s.handler.ListTools(ctx)
	resources, _ := s.handler.ListResources(ctx)
	prompts, _ := s.handler.ListPrompts(ctx)

	caps := ServerCapabilities{
		Tools:     ToolCapability{Count: len(tools)},
		Resources: ResourceCapability{Count: len(resources)},
		Prompts:   PromptCapability{Count: len(prompts)},
	}

	result := map[string]interface{}{
		"protocolVersion": "2024-01",
		"capabilities": caps,
		"serverInfo": map[string]string{
			"name":    "Gorkbot",
			"version": "6.1",
		},
	}

	resultJSON, _ := json.Marshal(result)
	resp.Result = resultJSON
	return resp, nil
}

// handleListTools processes a tools/list request.
func (s *Server) handleListTools(ctx context.Context, req, resp *Message) (*Message, error) {
	tools, err := s.handler.ListTools(ctx)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	resultJSON, _ := json.Marshal(result)
	resp.Result = resultJSON
	return resp, nil
}

// handleUseTool processes a tools/use request.
func (s *Server) handleUseTool(ctx context.Context, req, resp *Message) (*Message, error) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(resp, 400, "invalid parameters"), nil
	}

	result, err := s.handler.UseTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"content": result,
	})
	resp.Result = resultJSON
	return resp, nil
}

// handleListResources processes a resources/list request.
func (s *Server) handleListResources(ctx context.Context, req, resp *Message) (*Message, error) {
	resources, err := s.handler.ListResources(ctx)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	result := map[string]interface{}{
		"resources": resources,
	}

	resultJSON, _ := json.Marshal(result)
	resp.Result = resultJSON
	return resp, nil
}

// handleReadResource processes a resources/read request.
func (s *Server) handleReadResource(ctx context.Context, req, resp *Message) (*Message, error) {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(resp, 400, "invalid parameters"), nil
	}

	content, err := s.handler.ReadResource(ctx, params.URI)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"contents": content,
	})
	resp.Result = resultJSON
	return resp, nil
}

// handleListPrompts processes a prompts/list request.
func (s *Server) handleListPrompts(ctx context.Context, req, resp *Message) (*Message, error) {
	prompts, err := s.handler.ListPrompts(ctx)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	result := map[string]interface{}{
		"prompts": prompts,
	}

	resultJSON, _ := json.Marshal(result)
	resp.Result = resultJSON
	return resp, nil
}

// handleGetPrompt processes a prompts/get request.
func (s *Server) handleGetPrompt(ctx context.Context, req, resp *Message) (*Message, error) {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(resp, 400, "invalid parameters"), nil
	}

	content, err := s.handler.GetPrompt(ctx, params.Name, params.Arguments)
	if err != nil {
		return s.errorResponse(resp, 500, err.Error()), nil
	}

	resultJSON, _ := json.Marshal(content)
	resp.Result = resultJSON
	return resp, nil
}

// handlePing processes a ping request.
func (s *Server) handlePing(ctx context.Context, req, resp *Message) (*Message, error) {
	resultJSON, _ := json.Marshal(map[string]string{
		"status": "pong",
	})
	resp.Result = resultJSON
	return resp, nil
}

// errorResponse creates an error response.
func (s *Server) errorResponse(msg *Message, code int, errMsg string) *Message {
	msg.Error = &ErrorDetail{
		Code:    code,
		Message: errMsg,
	}
	msg.Type = MessageTypeError
	return msg
}

// GetConnectedClients returns the list of connected clients.
func (s *Server) GetConnectedClients() []Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var clients []Client
	for _, c := range s.clients {
		if c.Connected {
			clients = append(clients, *c)
		}
	}
	return clients
}
