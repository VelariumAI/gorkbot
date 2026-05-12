package puteradapter

import (
	"fmt"
	"path"
	"strings"
)

// WorkspaceMode controls enforcement behavior for the adapter runtime.
type WorkspaceMode string

const (
	WorkspaceOff     WorkspaceMode = "off"
	WorkspaceAudit   WorkspaceMode = "audit"
	WorkspaceEnforce WorkspaceMode = "enforce"
)

// Config contains runtime configuration for the Puter workspace adapter.
type Config struct {
	Mode               WorkspaceMode
	Root               string
	PuterRepo          string
	PuterRef           string
	PuterDefaultBranch string
}

// DefaultConfig returns conservative defaults with workspace handling disabled.
func DefaultConfig() Config {
	return Config{
		Mode:               WorkspaceOff,
		Root:               "/Gorkbot",
		PuterRepo:          "VelariumAI/puter",
		PuterRef:           "f80016e4e6a6f8062b737a415a0b7c18008ade98",
		PuterDefaultBranch: "main",
	}
}

// ParseWorkspaceMode parses CLI workspace mode values.
func ParseWorkspaceMode(raw string) (WorkspaceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(WorkspaceOff):
		return WorkspaceOff, true
	case string(WorkspaceAudit):
		return WorkspaceAudit, true
	case string(WorkspaceEnforce):
		return WorkspaceEnforce, true
	default:
		return "", false
	}
}

// Validate checks the config shape and normalizes root formatting.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("nil puteradapter config")
	}
	if _, ok := ParseWorkspaceMode(string(c.Mode)); !ok {
		return fmt.Errorf("invalid puter workspace mode %q", c.Mode)
	}
	root := strings.TrimSpace(c.Root)
	if root == "" {
		return fmt.Errorf("puter root cannot be empty")
	}
	if !strings.HasPrefix(root, "/") {
		return fmt.Errorf("puter root must be absolute: %q", root)
	}
	c.Root = path.Clean(strings.ReplaceAll(root, "\\", "/"))
	if c.Root == "/" {
		return fmt.Errorf("puter root cannot be filesystem root")
	}
	if strings.TrimSpace(c.PuterRepo) == "" {
		return fmt.Errorf("puter repo cannot be empty")
	}
	if strings.TrimSpace(c.PuterRef) == "" {
		return fmt.Errorf("puter ref cannot be empty")
	}
	if strings.TrimSpace(c.PuterDefaultBranch) == "" {
		return fmt.Errorf("puter default branch cannot be empty")
	}
	return nil
}
