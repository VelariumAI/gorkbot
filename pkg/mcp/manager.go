package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/tools"
)

// Ensure mcpTool implements tools.Tool at compile time.
var _ tools.Tool = (*mcpTool)(nil)

// Manager discovers, starts, and manages all configured MCP server clients.
// It also wraps each server's tools as standard Gorkbot tools for seamless
// integration with the existing tool registry.
type Manager struct {
	configPath string
	logger     *slog.Logger

	mu      sync.RWMutex
	clients []*Client
}

// NewManager creates an MCP manager that reads servers from configDir/mcp.json.
func NewManager(configDir string, logger *slog.Logger) *Manager {
	return &Manager{
		configPath: filepath.Join(configDir, "mcp.json"),
		logger:     logger,
	}
}

// LoadAndStart reads mcp.json, starts all enabled servers, and performs the
// MCP handshake. Returns the number of servers successfully connected.
func (m *Manager) LoadAndStart(ctx context.Context) (int, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No MCP config — silently skip
		}
		return 0, fmt.Errorf("mcp: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0, fmt.Errorf("mcp: parse config: %w", err)
	}

	connected := 0
	for _, srv := range cfg.Servers {
		if srv.Disabled {
			m.logger.Info("MCP server disabled", "name", srv.Name)
			continue
		}
		if srv.Transport != TransportStdio {
			m.logger.Warn("MCP transport not yet supported", "name", srv.Name, "transport", srv.Transport)
			continue
		}
		if srv.Command == "" {
			m.logger.Warn("MCP server has no command", "name", srv.Name)
			continue
		}

		client := NewStdioClient(srv)
		if err := client.Start(ctx); err != nil {
			m.logger.Error("MCP server start failed", "name", srv.Name, "error", err)
			continue
		}
		if err := client.Handshake(ctx); err != nil {
			m.logger.Error("MCP server handshake failed", "name", srv.Name, "error", err)
			client.Stop()
			continue
		}
		if _, err := client.ListTools(ctx); err != nil {
			m.logger.Warn("MCP tools/list failed", "name", srv.Name, "error", err)
		}

		m.mu.Lock()
		m.clients = append(m.clients, client)
		m.mu.Unlock()

		toolCount := len(client.CachedTools())
		m.logger.Info("MCP server connected", "name", srv.Name, "tools", toolCount)
		connected++
	}

	return connected, nil
}

// RegisterTools wraps all MCP server tools as Gorkbot tools and registers them
// in the provided registry. The tool names are prefixed with "mcp_<server>_"
// to avoid collisions with built-in tools.
func (m *Manager) RegisterTools(reg *tools.Registry) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	registered := 0
	for _, client := range m.clients {
		for _, td := range client.CachedTools() {
			t := newMCPTool(client, td)
			reg.RegisterOrReplace(t)
			registered++
		}
	}
	return registered
}

// StopAll terminates all connected MCP servers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		c.Stop()
	}
	m.clients = nil
}

// Status returns a human-readable summary of connected servers and their tools.
func (m *Manager) Status() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.clients) == 0 {
		return "No MCP servers connected.\n\nCreate ~/.config/gorkbot/mcp.json to configure servers.\nExample:\n```json\n{\n  \"servers\": [\n    {\"name\": \"filesystem\", \"transport\": \"stdio\", \"command\": \"npx\", \"args\": [\"-y\", \"@modelcontextprotocol/server-filesystem\", \"/tmp\"]}\n  ]\n}\n```"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%d MCP server(s) connected:**\n\n", len(m.clients)))
	for _, c := range m.clients {
		tls := c.CachedTools()
		sb.WriteString(fmt.Sprintf("• **%s** — %d tool(s)\n", c.ServerName(), len(tls)))
		for _, t := range tls {
			desc := t.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - `mcp_%s_%s`: %s\n", c.ServerName(), t.Name, desc))
		}
	}
	return sb.String()
}

// ConfigPath returns the path to the MCP config file.
func (m *Manager) ConfigPath() string { return m.configPath }

// ── MCPTool wraps a single MCP server tool as a Gorkbot Tool ────────────────

type mcpTool struct {
	client      *Client
	def         ToolDefinition
	toolName    string // "mcp_<server>_<name>"
	description string
}

func newMCPTool(client *Client, def ToolDefinition) *mcpTool {
	name := fmt.Sprintf("mcp_%s_%s", sanitizeName(client.ServerName()), sanitizeName(def.Name))
	desc := def.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool '%s' from server '%s'", def.Name, client.ServerName())
	}
	return &mcpTool{client: client, def: def, toolName: name, description: desc}
}

func (t *mcpTool) Name() string                     { return t.toolName }
func (t *mcpTool) Description() string               { return t.description }
func (t *mcpTool) Category() tools.ToolCategory      { return tools.CategoryCustom }
func (t *mcpTool) RequiresPermission() bool          { return true }
func (t *mcpTool) DefaultPermission() tools.PermissionLevel { return tools.PermissionOnce }
func (t *mcpTool) OutputFormat() tools.OutputFormat { return tools.FormatText }

// Parameters returns the MCP tool's input schema as a raw JSON message.
// MCP servers already provide a JSON Schema object, so we forward it directly.
func (t *mcpTool) Parameters() json.RawMessage {
	if len(t.def.InputSchema) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return t.def.InputSchema
}

func (t *mcpTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	result, err := t.client.CallTool(ctx, t.def.Name, params)
	if err != nil {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("MCP tool call failed: %v", err),
		}, nil
	}

	if result.IsError {
		errText := ""
		for _, c := range result.Content {
			if c.Type == "text" {
				errText += c.Text
			}
		}
		return &tools.ToolResult{Success: false, Error: errText}, nil
	}

	var parts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return &tools.ToolResult{
		Success: true,
		Output:  strings.Join(parts, "\n"),
	}, nil
}

func sanitizeName(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return strings.ToLower(sb.String())
}
