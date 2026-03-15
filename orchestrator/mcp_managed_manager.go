package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/tools"
)

type Config struct {
	Servers []ServerConfig `json:"servers"`
}

// ServerConfig defines a single MCP server entry in mcp.json.
type ServerConfig struct {
	Name         string            `json:"name"`
	Transport    string            `json:"transport,omitempty"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	Env          map[string]string `json:"env,omitempty"`
	ShaperPath   string            `json:"shaper_path,omitempty"`
	Disabled     bool              `json:"disabled,omitempty"`
	Description  string            `json:"description,omitempty"`
	AuthProvider string            `json:"auth_provider,omitempty"`
}

func envToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		if k != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

type TokenProvider interface {
	GetAccessToken() string
}

// ManagedManager orchestrates multiple high-resiliency MCP servers concurrently.
type ManagedManager struct {
	configPath     string
	configDir      string
	logger         *slog.Logger
	registry       *tools.Registry
	servers        map[string]*ManagedMCPServer
	failedServers  map[string]string // name → error message for servers that failed to boot
	tokenProviders map[string]TokenProvider
	mu             sync.RWMutex
}

func NewManagedManager(configDir string, reg *tools.Registry, logger *slog.Logger) *ManagedManager {
	return &ManagedManager{
		configPath:     filepath.Join(configDir, "mcp.json"),
		configDir:      configDir,
		logger:         logger,
		registry:       reg,
		servers:        make(map[string]*ManagedMCPServer),
		failedServers:  make(map[string]string),
		tokenProviders: make(map[string]TokenProvider),
	}
}

func (m *ManagedManager) RegisterTokenProvider(name string, tp TokenProvider) {
	m.mu.Lock()
	m.tokenProviders[name] = tp
	m.mu.Unlock()
}

func (m *ManagedManager) LoadAndStart(ctx context.Context) (int, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0, err
	}

	// Clear stale failure records before each load.
	m.mu.Lock()
	m.failedServers = make(map[string]string)
	m.mu.Unlock()

	var wg sync.WaitGroup
	var startedCount int32
	var mu sync.Mutex

	for _, sc := range cfg.Servers {
		if sc.Disabled {
			continue
		}
		if skipOnPlatform(sc) {
			m.logger.Warn("Skipping platform-incompatible MCP server", "name", sc.Name)
			continue
		}
		if hasPlaceholderPath(sc) {
			m.logger.Warn("Skipping MCP server with unresolved placeholder path", "name", sc.Name)
			continue
		}

		wg.Add(1)
		go func(sc ServerConfig) {
			defer wg.Done()

			// Create a per-server timeout context to prevent global hang
			srvCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			envVars := envToSlice(sc.Env)
			envVars = append(envVars, "GORKBOT_CONFIG_DIR="+m.configDir)

			if sc.AuthProvider != "" {
				m.mu.RLock()
				tp, ok := m.tokenProviders[sc.AuthProvider]
				m.mu.RUnlock()
				if ok {
					if token := tp.GetAccessToken(); token != "" {
						envVars = append(envVars, "GOOGLE_ACCESS_TOKEN="+token)
					}
				}
			}

			// Pass the full ServerConfig + envVars so the ManagedMCPServer can
			// create a fresh ProcessManager on each recovery (new dead channel).
			managed := NewManagedMCPServer(sc, envVars, m.logger)

			if err := managed.Start(srvCtx); err != nil {
				m.logger.Error("MCP server boot failed", "name", sc.Name, "err", err)
				mu.Lock()
				m.failedServers[sc.Name] = err.Error()
				mu.Unlock()
				return
			}

			managed.SyncWith(m.registry)

			mu.Lock()
			m.servers[sc.Name] = managed
			startedCount++
			mu.Unlock()
		}(sc)
	}

	wg.Wait()
	return int(startedCount), nil
}

func (m *ManagedManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, srv := range m.servers {
		m.logger.Info("Stopping MCP server", "name", name)
		_ = srv.Stop()
	}
	m.servers = make(map[string]*ManagedMCPServer)
}

func (m *ManagedManager) Status() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.servers) == 0 {
		return "No Managed MCP servers connected."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%d Hybrid MCP server(s) online:**\n\n", len(m.servers)))
	for name, srv := range m.servers {
		sb.WriteString(fmt.Sprintf("• **%s** — %d tool(s) (Resilient Mode)\n", name, len(srv.toolDefs)))
		for _, t := range srv.toolDefs {
			sb.WriteString(fmt.Sprintf("  - `mcp_%s_%s`\n", name, t.Name))
		}
	}
	return sb.String()
}

func (m *ManagedManager) ConfigPath() string {
	return m.configPath
}

// ServerStatusInfo is a lightweight snapshot of a single MCP server's state.
type ServerStatusInfo struct {
	Name      string
	Running   bool
	ToolCount int
	Error     string // non-empty when Running=false
}

// GetServerStatuses returns the runtime state of every server that was
// configured during the most recent LoadAndStart call — both running servers
// and those that failed to boot.
func (m *ManagedManager) GetServerStatuses() []ServerStatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ServerStatusInfo, 0, len(m.servers)+len(m.failedServers))
	for name, srv := range m.servers {
		out = append(out, ServerStatusInfo{
			Name:      name,
			Running:   true,
			ToolCount: len(srv.toolDefs),
		})
	}
	for name, errMsg := range m.failedServers {
		out = append(out, ServerStatusInfo{
			Name:  name,
			Error: errMsg,
		})
	}
	return out
}

func (m *ManagedManager) Reload(ctx context.Context, reg *tools.Registry) (int, error) {
	m.StopAll()
	m.mu.Lock()
	m.registry = reg
	m.mu.Unlock()
	return m.LoadAndStart(ctx)
}

// GetSystemContext returns an MCP-awareness block for injection into the AI
// system prompt. It lists every running server, its purpose, and usage
// decision rules so the AI knows exactly when and how to invoke MCP tools.
func (m *ManagedManager) GetSystemContext() string {
	m.mu.RLock()
	if len(m.servers) == 0 {
		m.mu.RUnlock()
		return ""
	}
	type entry struct {
		name      string
		toolCount int
		tools     []string
	}
	entries := make([]entry, 0, len(m.servers))
	for name, srv := range m.servers {
		toolNames := make([]string, 0, len(srv.toolDefs))
		for _, t := range srv.toolDefs {
			toolNames = append(toolNames, fmt.Sprintf("mcp_%s_%s", name, t.Name))
		}
		entries = append(entries, entry{name: name, toolCount: len(srv.toolDefs), tools: toolNames})
	}
	m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n### MCP INTEGRATION (Model Context Protocol)\n")
	sb.WriteString("The following MCP servers are **currently running** and their tools are registered.\n")
	sb.WriteString("MCP tools appear in AVAILABLE TOOLS prefixed `mcp_<server>_<toolname>`.\n")
	sb.WriteString("They are first-class tools — call them exactly like any native tool.\n\n")

	sb.WriteString("**Active MCP Servers:**\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("• **%s** (%d tools) — %s\n", e.name, e.toolCount, mcpServerPurpose(e.name)))
	}

	sb.WriteString("\n**When to use MCP tools (decision guide):**\n")
	sb.WriteString("- **sequential-thinking**: Use for tasks requiring structured multi-step reasoning — complex plans, debugging chains, research synthesis. Call `mcp_sequential-thinking_sequentialthinking` with each thought step.\n")
	sb.WriteString("- **memory**: Use `mcp_memory_create_entities` / `mcp_memory_search_nodes` / `mcp_memory_add_observations` for persistent cross-session facts that must survive restarts. Prefer this over `record_engram` for structured entity data.\n")
	sb.WriteString("- **filesystem**: Use for bulk directory reads across allowed paths (`/home/project/gorky`, `~/.config/gorkbot`, `~`). For single-file ops, native `read_file` / `write_file` / `bash` are more flexible.\n")
	sb.WriteString("- **time**: Use `mcp_time_get_current_time` when you need the precise current time or timezone conversion. Do NOT guess the time.\n")
	sb.WriteString("- **fetch**: Use `mcp_fetch_fetch` when `web_fetch` is rate-limited or you need raw HTTP body with custom headers.\n")
	sb.WriteString("- **brave-search**: Use when a web search is needed and `web_search` is unavailable. **Requires `BRAVE_API_KEY` in .env.**\n")
	sb.WriteString("- **github**: Use for GitHub API operations (list repos, create issues/PRs, read code, search). **Requires `GITHUB_PERSONAL_ACCESS_TOKEN` in .env.** If you get auth errors, tell the user to run `gorkbot setup`.\n")
	sb.WriteString("- **puppeteer**: Use when a website requires JavaScript rendering or browser interaction. For plain HTTP use `web_fetch` or `mcp_fetch_fetch` instead.\n")
	sb.WriteString("- **notebooklm**: Use to manage Google NotebookLM knowledge bases. `mcp_notebooklm_create_notebook`, `mcp_notebooklm_add_source`, `mcp_notebooklm_chat_with_notebook`. **Requires `GEMINI_API_KEY`.** Google Drive sources additionally require OAuth — tell user to run `/auth notebooklm login`.\n")
	sb.WriteString("- **gorkbot-introspect**: Use `mcp_gorkbot-introspect_*` to query Gorkbot's live capabilities, loaded tools, active providers, and session state. Use instead of `gorkbot_status` when you need fine-grained subsystem info.\n")
	sb.WriteString("- **gorkbot-android** / **gorkbot-termux**: Use for Android device-specific operations not covered by native tools (ADB, Termux APIs, sensors).\n")

	sb.WriteString("\n**Auth / credential requirements:**\n")
	sb.WriteString("- `github` tools → `GITHUB_PERSONAL_ACCESS_TOKEN` in .env\n")
	sb.WriteString("- `brave-search` tools → `BRAVE_API_KEY` in .env\n")
	sb.WriteString("- `notebooklm` AI features → `GEMINI_API_KEY` in .env\n")
	sb.WriteString("- `notebooklm` Google Drive sources → user must run `/auth notebooklm login` (Device Flow)\n")
	sb.WriteString("Missing credentials? Tell the user: run `gorkbot setup` to configure keys, or `/auth notebooklm login` for Google OAuth.\n")

	return sb.String()
}

// mcpServerPurpose returns a concise human-readable description for a known
// MCP server name. Falls back to a generic label for unknown servers.
func mcpServerPurpose(name string) string {
	purposes := map[string]string{
		"sequential-thinking": "structured multi-step reasoning chains",
		"filesystem":          "file I/O within allowed project/config/home directories",
		"memory":              "persistent cross-session key-value entity graph",
		"time":                "current time & timezone lookups",
		"fetch":               "raw HTTP fetch for any URL",
		"github":              "GitHub API — issues, PRs, repos, code search (needs GITHUB_TOKEN)",
		"brave-search":        "web search via Brave Search API (needs BRAVE_API_KEY)",
		"puppeteer":           "headless browser automation with JS rendering",
		"gorkbot-introspect":  "Gorkbot self-introspection — capabilities, tools, config",
		"gorkbot-android":     "Android device control via ADB/Termux APIs",
		"gorkbot-termux":      "Termux environment management and system APIs",
		"notebooklm":          "Google NotebookLM — notebooks, sources, AI chat (needs GEMINI_API_KEY)",
	}
	if p, ok := purposes[strings.ToLower(name)]; ok {
		return p
	}
	return "external MCP integration"
}

// skipOnPlatform returns true if a ServerConfig contains Windows-only path
// markers (backslash drive letters, %VAR%) when running on a non-Windows host,
// or Linux-only markers on Windows. These are the tell-tale signs of a
// platform-specific entry that should not be started on the wrong OS.
func skipOnPlatform(sc ServerConfig) bool {
	goos := runtime.GOOS
	for _, arg := range sc.Args {
		// Windows-style path: C:\ or %USERNAME% → skip on non-Windows
		if (strings.Contains(arg, `C:\`) || strings.Contains(arg, `%USERNAME%`) || strings.Contains(arg, `%APPDATA%`)) && goos != "windows" {
			return true
		}
	}
	return false
}

// hasPlaceholderPath returns true when a server's args contain an unresolved
// template placeholder (e.g. "/path/to/" or "<GORKBOT_PROJECT_DIR>").
func hasPlaceholderPath(sc ServerConfig) bool {
	markers := []string{"/path/to/", "<GORKBOT_PROJECT_DIR>", "<project_dir>", "your_path_here"}
	for _, arg := range sc.Args {
		for _, m := range markers {
			if strings.Contains(arg, m) {
				return true
			}
		}
	}
	return false
}
