package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
)

// permPattern holds a glob pattern and its associated permission level.
type permPattern struct {
	Pattern string          `json:"pattern"`
	Level   PermissionLevel `json:"level"`
}

// PermissionManager manages tool permissions with persistent storage
type PermissionManager struct {
	permissions map[string]PermissionLevel
	patterns    []permPattern // glob-pattern permissions, checked after exact miss
	configPath  string
	mu          sync.RWMutex
}

// PermissionConfig is the persistent storage format
type PermissionConfig struct {
	Permissions map[string]PermissionLevel `json:"permissions"`
	Patterns    []permPattern              `json:"patterns,omitempty"`
	Version     string                     `json:"version"`
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager(configDir string) (*PermissionManager, error) {
	configPath := filepath.Join(configDir, "tool_permissions.json")

	pm := &PermissionManager{
		permissions: make(map[string]PermissionLevel),
		configPath:  configPath,
	}

	// Load existing permissions
	if err := pm.load(); err != nil {
		// If file doesn't exist, that's okay
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load permissions: %w", err)
		}
	}

	return pm, nil
}

// GetPermission returns the permission level for a tool.
// Exact-name match takes priority; glob patterns are checked on miss.
func (pm *PermissionManager) GetPermission(toolName string) PermissionLevel {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if perm, exists := pm.permissions[toolName]; exists {
		return perm
	}

	// Fall through to glob-pattern matching.
	for _, p := range pm.patterns {
		if ok, _ := path.Match(p.Pattern, toolName); ok {
			return p.Level
		}
	}

	// Default to asking user
	return PermissionOnce
}

// SetPatternPermission sets a glob-pattern permission rule.
// Pattern syntax follows path.Match (e.g. "file_*", "git_*").
func (pm *PermissionManager) SetPatternPermission(pattern string, level PermissionLevel) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Update existing entry if pattern already present.
	for i, p := range pm.patterns {
		if p.Pattern == pattern {
			pm.patterns[i].Level = level
			return pm.save()
		}
	}
	pm.patterns = append(pm.patterns, permPattern{Pattern: pattern, Level: level})
	return pm.save()
}

// SetPermission sets the permission level for a tool
func (pm *PermissionManager) SetPermission(toolName string, level PermissionLevel) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.permissions[toolName] = level
	return pm.save()
}

// RevokePermission removes a tool's permission (reverts to default)
func (pm *PermissionManager) RevokePermission(toolName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.permissions, toolName)
	return pm.save()
}

// RevokeAll removes all permissions
func (pm *PermissionManager) RevokeAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.permissions = make(map[string]PermissionLevel)
	return pm.save()
}

// ListPermissions returns all configured permissions
func (pm *PermissionManager) ListPermissions() map[string]PermissionLevel {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Return a copy to prevent external modification
	perms := make(map[string]PermissionLevel, len(pm.permissions))
	for k, v := range pm.permissions {
		perms[k] = v
	}
	return perms
}

// load reads permissions from disk
func (pm *PermissionManager) load() error {
	data, err := os.ReadFile(pm.configPath)
	if err != nil {
		return err
	}

	var config PermissionConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse permissions: %w", err)
	}

	pm.permissions = config.Permissions
	pm.patterns = config.Patterns
	return nil
}

// save writes permissions to disk
func (pm *PermissionManager) save() error {
	config := PermissionConfig{
		Permissions: pm.permissions,
		Patterns:    pm.patterns,
		Version:     "1.0",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(pm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write with secure permissions
	if err := os.WriteFile(pm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write permissions: %w", err)
	}

	return nil
}

// GetConfigPath returns the path to the permissions file
func (pm *PermissionManager) GetConfigPath() string {
	return pm.configPath
}

// RemovePermission is an alias for RevokePermission
func (pm *PermissionManager) RemovePermission(toolName string) error {
	return pm.RevokePermission(toolName)
}

// ClearAll is an alias for RevokeAll
func (pm *PermissionManager) ClearAll() error {
	return pm.RevokeAll()
}
