package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DynamicToolParam describes a single parameter of a dynamically created tool.
type DynamicToolParam struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// DynamicToolConfig is the JSON-persisted definition of a runtime-created tool.
// Tools defined this way are registered immediately into the live registry and
// reloaded automatically on the next startup — no rebuild required.
type DynamicToolConfig struct {
	Name               string                      `json:"name"`
	Description        string                      `json:"description"`
	Category           string                      `json:"category"`
	Command            string                      `json:"command"`
	Parameters         map[string]DynamicToolParam `json:"parameters"`
	RequiresPermission bool                        `json:"requires_permission"`
	DefaultPermission  string                      `json:"default_permission"`
}

// DynamicToolsFile is the top-level JSON file that persists all dynamic tools.
type DynamicToolsFile struct {
	Version string              `json:"version"`
	Tools   []DynamicToolConfig `json:"tools"`
	// PendingRebuild lists tools that were modified/overridden at runtime and
	// need a `go build` before the changes become part of the compiled binary.
	PendingRebuild []string `json:"pending_rebuild,omitempty"`
}

// DynamicScriptTool is a runtime-registered tool backed by a DynamicToolConfig.
// It executes a bash command template with parameter substitution.
type DynamicScriptTool struct {
	BaseTool
	config DynamicToolConfig
}

// NewDynamicScriptTool creates a DynamicScriptTool from a DynamicToolConfig.
func NewDynamicScriptTool(cfg DynamicToolConfig) *DynamicScriptTool {
	return &DynamicScriptTool{
		BaseTool: BaseTool{
			name:               cfg.Name,
			description:        cfg.Description,
			category:           categoryFromString(cfg.Category),
			requiresPermission: cfg.RequiresPermission,
			defaultPermission:  permissionFromString(cfg.DefaultPermission),
		},
		config: cfg,
	}
}

func (t *DynamicScriptTool) Parameters() json.RawMessage {
	props := map[string]interface{}{}
	required := []string{}

	// Sort for deterministic output
	names := make([]string, 0, len(t.config.Parameters))
	for name := range t.config.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		param := t.config.Parameters[name]
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

// wrapWithResourceLimits wraps a command with ulimit to prevent resource exhaustion.
// Enforces: 512MB virtual memory, 30s CPU time max.
func wrapWithResourceLimits(cmd string) string {
	return fmt.Sprintf("ulimit -v 524288 -t 30 2>/dev/null; %s", cmd)
}

func (t *DynamicScriptTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := t.config.Command

	for name, paramCfg := range t.config.Parameters {
		placeholder := "{{" + name + "}}"
		var value string

		if v, ok := params[name].(string); ok {
			value = v
		} else if paramCfg.Default != "" {
			value = paramCfg.Default
		} else if paramCfg.Required {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("required parameter %q is missing", name),
			}, fmt.Errorf("required param %s missing", name)
		}

		cmd = strings.ReplaceAll(cmd, placeholder, shellescape(value))
	}

	// Wrap with resource limits to prevent exhaustion
	cmd = wrapWithResourceLimits(cmd)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{"command": cmd})
}

// categoryFromString converts a category string to a ToolCategory constant.
func categoryFromString(s string) ToolCategory {
	switch strings.ToLower(s) {
	case "shell":
		return CategoryShell
	case "file":
		return CategoryFile
	case "git":
		return CategoryGit
	case "web":
		return CategoryWeb
	case "system":
		return CategorySystem
	case "communication":
		return CategoryCommunication
	case "meta":
		return CategoryMeta
	case "ai":
		return CategoryAI
	case "database":
		return CategoryDatabase
	case "network":
		return CategoryNetwork
	case "media":
		return CategoryMedia
	case "android":
		return CategoryAndroid
	case "package":
		return CategoryPackage
	default:
		return CategoryCustom
	}
}

// permissionFromString converts a permission string to a PermissionLevel constant.
func permissionFromString(s string) PermissionLevel {
	switch strings.ToLower(s) {
	case "always":
		return PermissionAlways
	case "session":
		return PermissionSession
	case "never":
		return PermissionNever
	default:
		return PermissionOnce
	}
}

// dynamicToolsFilePath returns the path to the dynamic tools JSON config.
func dynamicToolsFilePath(configDir string) string {
	return filepath.Join(configDir, "dynamic_tools.json")
}

// readDynamicFile reads the persistent dynamic-tools file, returning an empty
// file struct (not an error) when the file does not yet exist.
func readDynamicFile(configDir string) (DynamicToolsFile, error) {
	var file DynamicToolsFile
	data, err := os.ReadFile(dynamicToolsFilePath(configDir))
	if err != nil {
		if os.IsNotExist(err) {
			return file, nil
		}
		return file, fmt.Errorf("failed to read dynamic tools file: %w", err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("failed to parse dynamic tools file: %w", err)
	}
	return file, nil
}

// writeDynamicFile atomically writes the dynamic-tools file, creating the
// parent directory if it does not yet exist.
func writeDynamicFile(configDir string, file DynamicToolsFile) error {
	file.Version = "1.0"
	path := dynamicToolsFilePath(configDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dynamic tools: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// LoadDynamicTools reads persisted dynamic tools (and any pending rebuild list)
// from disk and registers them into the live registry.
// Called at startup; silently succeeds if no file exists yet.
func (r *Registry) LoadDynamicTools(configDir string) error {
	file, err := readDynamicFile(configDir)
	if err != nil {
		return err
	}

	for _, cfg := range file.Tools {
		r.RegisterOrReplace(NewDynamicScriptTool(cfg))
	}

	// Restore any pending rebuild list from the previous session
	if len(file.PendingRebuild) > 0 {
		r.mu.Lock()
		r.pendingRebuild = file.PendingRebuild
		r.mu.Unlock()
	}

	return nil
}

// SaveDynamicTool persists a new tool config to disk and registers it into
// the live registry immediately — no rebuild or restart needed.
func (r *Registry) SaveDynamicTool(cfg DynamicToolConfig) error {
	// Register live immediately
	r.RegisterOrReplace(NewDynamicScriptTool(cfg))

	configDir := r.GetConfigDir()
	if configDir == "" {
		return fmt.Errorf("configDir not set on registry; tool registered live but not persisted")
	}

	file, err := readDynamicFile(configDir)
	if err != nil {
		return err
	}

	// Upsert: remove existing entry with same name, append new
	updated := make([]DynamicToolConfig, 0, len(file.Tools)+1)
	for _, t := range file.Tools {
		if t.Name != cfg.Name {
			updated = append(updated, t)
		}
	}
	file.Tools = append(updated, cfg)

	// Preserve current pending rebuild list
	r.mu.RLock()
	file.PendingRebuild = r.pendingRebuild
	r.mu.RUnlock()

	return writeDynamicFile(configDir, file)
}

// MarkPendingRebuild records that toolName requires a Go rebuild to become a
// permanent compiled-in tool. The list is persisted so it survives restarts.
func (r *Registry) MarkPendingRebuild(toolName string) {
	r.mu.Lock()
	for _, name := range r.pendingRebuild {
		if name == toolName {
			r.mu.Unlock()
			return // already tracked
		}
	}
	r.pendingRebuild = append(r.pendingRebuild, toolName)
	pending := r.pendingRebuild
	r.mu.Unlock()

	// Best-effort persist
	configDir := r.GetConfigDir()
	if configDir == "" {
		return
	}
	file, err := readDynamicFile(configDir)
	if err != nil {
		return
	}
	file.PendingRebuild = pending
	_ = writeDynamicFile(configDir, file)
}

// GetPendingRebuild returns the current list of tools awaiting a Go rebuild.
func (r *Registry) GetPendingRebuild() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.pendingRebuild) == 0 {
		return nil
	}
	result := make([]string, len(r.pendingRebuild))
	copy(result, r.pendingRebuild)
	return result
}

// ClearPendingRebuild clears the pending rebuild list both in memory and on disk
// (called after a successful rebuild).
func (r *Registry) ClearPendingRebuild() error {
	r.mu.Lock()
	r.pendingRebuild = nil
	r.mu.Unlock()

	configDir := r.GetConfigDir()
	if configDir == "" {
		return nil
	}
	file, err := readDynamicFile(configDir)
	if err != nil {
		return err
	}
	file.PendingRebuild = nil
	return writeDynamicFile(configDir, file)
}
