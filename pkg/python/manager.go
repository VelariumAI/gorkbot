package python

/*
# Python Plugin System for Gorkbot

This package provides foundational support for embedding Python tools/plugins
into the Go-based Gorkbot system.

## Architecture Overview

┌─────────────────────────────────────────────────────────────────┐
│                        Gorkbot (Go)                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐  │
│  │  Tool Registry  │  │   Orchestrator   │  │  Dispatcher  │  │
│  └────────┬────────┘  └────────┬────────┘  └──────┬───────┘  │
│           │                    │                   │           │
│           ▼                    ▼                   ▼           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              Python Plugin Manager (this pkg)              ││
│  │  ┌─────────────┐  ┌──────────────┐  ┌───────────────────┐  ││
│  │  │   Loader   │  │ Tool Wrapper  │  │  Execution Engine │  ││
│  │  └─────────────┘  └──────────────┘  └───────────────────┘  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                    │
│                              ▼                                    │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              Embedded Python Interpreter                    ││
│  │              (via embedded-python or cgo)                   ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘

## Integration Approaches

### Option A: CGO with Python (Recommended for production)
- Use cgo to link against libpython
- Full access to Python C API
- Higher performance for Python tools
- More complex build setup

### Option B: Python subprocess (Simpler, sandboxed)
- Spawn python3 processes
- JSON via stdin/stdout
- Easier to build, slightly slower
- Good for plugin isolation

### Option C: Embedded Python (gevent/embedded-python)
- Embed Python interpreter in Go binary
- No external Python dependency
- More complex memory management
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/tools"
)

const (
	// PluginDir is the directory where Python plugins are stored
	PluginDir = "plugins/python"

	// PluginManifest is the filename for plugin metadata
	PluginManifestFile = "manifest.json"

	// DefaultPythonCmd is the Python interpreter command
	DefaultPythonCmd = "python3"
)

// ToolResult represents the result from a Python tool execution
type ToolResult struct {
	Success bool                   `json:"success"`
	Output  string                 `json:"output"`
	Error   string                 `json:"error,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ToolParam describes a parameter for a Python tool
type ToolParam struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// PluginManifest defines the metadata for a Python plugin
type PluginManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	EntryPoint  string            `json:"entry_point"` // Python file with execute() function
	Parameters  map[string]ToolParam `json:"parameters"`
	Requires    []string          `json:"requires,omitempty"` // pip dependencies
	Category    string            `json:"category"`
}

// PythonTool is a Go wrapper that makes a Python script appear as a Go Tool
type PythonTool struct {
	name               string
	description        string
	category           tools.ToolCategory
	requiresPermission bool
	defaultPermission  tools.PermissionLevel
	pluginPath         string
	entryPoint         string
	params             map[string]ToolParam
	mu                 sync.Mutex
	pythonCmd          string
}

// NewPythonTool creates a new PythonTool from a manifest
func NewPythonTool(manifest PluginManifest, pluginDir string) *PythonTool {
	return &PythonTool{
		name:               manifest.Name,
		description:        manifest.Description,
		category:           tools.ToolCategory(manifest.Category),
		requiresPermission: true, // Always require permission for external scripts
		defaultPermission:  tools.PermissionOnce,
		pluginPath:         filepath.Join(pluginDir, manifest.Name),
		entryPoint:         manifest.EntryPoint,
		params:             manifest.Parameters,
		pythonCmd:          DefaultPythonCmd,
	}
}

// Name returns the tool's unique identifier
func (t *PythonTool) Name() string {
	return t.name
}

// Description returns a human-readable description
func (t *PythonTool) Description() string {
	return t.description
}

// Category returns the tool's category
func (t *PythonTool) Category() tools.ToolCategory {
	return t.category
}

// OutputFormat returns the tool's output format
func (t *PythonTool) OutputFormat() tools.OutputFormat {
	return tools.FormatText
}

// Parameters returns JSON schema for the tool's parameters
func (t *PythonTool) Parameters() json.RawMessage {
	props := make(map[string]interface{})
	required := []string{}

	for name, param := range t.params {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Default != "" {
			prop["default"] = param.Default
		}
		props[name] = prop
		if param.Required {
			required = append(required, name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	data, _ := json.Marshal(schema)
	return data
}

// Execute runs the Python tool with given parameters
func (t *PythonTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Prepare input for Python script
	input := map[string]interface{}{
		"action":   "execute",
		"tool":     t.name,
		"params":   params,
		"timeout":  300, // 5 minute default timeout
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to marshal input: %v", err),
		}, err
	}

	// Build command
	cmd := exec.CommandContext(ctx, t.pythonCmd, t.entryPoint)
	cmd.Dir = t.pluginPath
	cmd.Stdin = strings.NewReader(string(inputJSON))

	// Set up environment
	env := os.Environ()
	env = append(env, "GORKBOT_PLUGIN=1")
	cmd.Env = env

	// Capture output
	output, err := cmd.Output()
	if err != nil {
		// Check if it's a execution error or a Python error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &tools.ToolResult{
				Success: false,
				Error:   string(exitErr.Stderr),
			}, nil
		}
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("execution failed: %v", err),
		}, err
	}

	// Parse Python output
	var result ToolResult
	if err := json.Unmarshal(output, &result); err != nil {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to parse output: %v", err),
		}, err
	}

	return &tools.ToolResult{
		Success: result.Success,
		Output:  result.Output,
		Error:   result.Error,
		Data:    result.Data,
	}, nil
}

// RequiresPermission indicates if this tool needs user approval
func (t *PythonTool) RequiresPermission() bool {
	return t.requiresPermission
}

// DefaultPermission returns the default permission level
func (t *PythonTool) DefaultPermission() tools.PermissionLevel {
	return t.defaultPermission
}

// SetPythonCmd sets the Python interpreter command
func (t *PythonTool) SetPythonCmd(cmd string) {
	t.pythonCmd = cmd
}

// Manager manages Python plugins for the tool registry
type Manager struct {
	pluginDir     string
	pythonCmd     string
	registry      *tools.Registry
	loadedPlugins map[string]*PythonTool
	mu            sync.RWMutex
	enabled       bool
}

// NewManager creates a new Python plugin manager
func NewManager(pluginDir string, registry *tools.Registry) *Manager {
	return &Manager{
		pluginDir:     pluginDir,
		pythonCmd:     DefaultPythonCmd,
		registry:      registry,
		loadedPlugins: make(map[string]*PythonTool),
		enabled:       true,
	}
}

// SetPythonCmd sets the Python interpreter to use
func (m *Manager) SetPythonCmd(cmd string) {
	m.pythonCmd = cmd
}

// Enable enables or disables the Python plugin system
func (m *Manager) Enable(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// IsEnabled returns whether the plugin system is enabled
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// DiscoverPlugins scans the plugin directory for Python plugins
func (m *Manager) DiscoverPlugins() ([]PluginManifest, error) {
	var manifests []PluginManifest

	entries, err := os.ReadDir(m.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return manifests, nil
		}
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(m.pluginDir, entry.Name(), PluginManifestFile)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // Skip directories without manifest
		}

		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue // Skip invalid manifests
		}

		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

// LoadPlugin loads a single Python plugin
func (m *Manager) LoadPlugin(manifest PluginManifest) (*PythonTool, error) {
	tool := NewPythonTool(manifest, m.pluginDir)
	tool.SetPythonCmd(m.pythonCmd)

	// Verify the entry point exists
	entryPath := filepath.Join(m.pluginDir, manifest.Name, manifest.EntryPoint)
	if _, err := os.Stat(entryPath); err != nil {
		return nil, fmt.Errorf("entry point not found: %w", err)
	}

	// Check for required dependencies
	if len(manifest.Requires) > 0 {
		if err := m.installDependencies(manifest.Requires); err != nil {
			return nil, fmt.Errorf("failed to install dependencies: %w", err)
		}
	}

	m.mu.Lock()
	m.loadedPlugins[manifest.Name] = tool
	m.mu.Unlock()

	return tool, nil
}

// LoadAllPlugins discovers and loads all Python plugins
func (m *Manager) LoadAllPlugins() error {
	manifests, err := m.DiscoverPlugins()
	if err != nil {
		return err
	}

	for _, manifest := range manifests {
		_, err := m.LoadPlugin(manifest)
		if err != nil {
			return err
		}
	}

	return nil
}

// RegisterAll registers all loaded plugins with the tool registry
func (m *Manager) RegisterAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, tool := range m.loadedPlugins {
		if err := m.registry.Register(tool); err != nil {
			// Tool might already exist, try to replace
			m.registry.RegisterOrReplace(tool)
		}
	}

	return nil
}

// UnloadPlugin removes a plugin from the registry
func (m *Manager) UnloadPlugin(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tool, exists := m.loadedPlugins[name]; exists {
		delete(m.loadedPlugins, name)
		// Note: Can't unregister from registry directly
		_ = tool // Plugin unloaded from memory
	}
}

// GetLoadedPlugins returns all loaded plugin names
func (m *Manager) GetLoadedPlugins() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.loadedPlugins))
	for name := range m.loadedPlugins {
		names = append(names, name)
	}
	return names
}

// installDependencies installs Python dependencies via pip
func (m *Manager) installDependencies(requires []string) error {
	for _, pkg := range requires {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, m.pythonCmd, "-m", "pip", "install", pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to install %s: %s: %w", pkg, output, err)
		}
	}
	return nil
}

// CheckPython checks if Python is available and returns version info
func CheckPython(pythonCmd string) (version string, available bool, err error) {
	cmd := exec.Command(pythonCmd, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	return string(output), true, nil
}
