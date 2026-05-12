package puteradapter

import (
	"fmt"
	"net/netip"
	"net/url"
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

// DeploymentMode describes where a Puter runtime is hosted.
type DeploymentMode string

const (
	DeploymentLocal      DeploymentMode = "local"
	DeploymentSelfHosted DeploymentMode = "self_hosted"
	DeploymentSaaS       DeploymentMode = "saas"
)

// Config contains runtime configuration for the Puter workspace adapter.
type Config struct {
	Mode WorkspaceMode
	Root string
	// PuterRepo/PuterRef pin the VelariumAI/puter fork that defines the expected API contract.
	PuterRepo          string
	PuterRef           string
	PuterDefaultBranch string
	// DeploymentMode selects where the contract is served (local, self_hosted, saas).
	// This does not weaken workspace/capability governance or grant general network bypass.
	DeploymentMode DeploymentMode
	// Endpoint is the dedicated configured Puter substrate target, not a generic gateway override.
	Endpoint string
	// AllowInsecureSaaSEndpoint allows http:// SaaS endpoints only when explicitly opted in.
	AllowInsecureSaaSEndpoint bool
}

// DefaultConfig returns conservative defaults with workspace handling disabled.
func DefaultConfig() Config {
	return Config{
		Mode:               WorkspaceOff,
		Root:               "/Gorkbot",
		PuterRepo:          "VelariumAI/puter",
		PuterRef:           "f80016e4e6a6f8062b737a415a0b7c18008ade98",
		PuterDefaultBranch: "main",
		DeploymentMode:     DeploymentLocal,
		Endpoint:           "",
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

// ParseDeploymentMode parses runtime deployment mode values.
func ParseDeploymentMode(raw string) (DeploymentMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(DeploymentLocal):
		return DeploymentLocal, true
	case string(DeploymentSelfHosted):
		return DeploymentSelfHosted, true
	case string(DeploymentSaaS):
		return DeploymentSaaS, true
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
	deploymentMode, ok := ParseDeploymentMode(string(c.DeploymentMode))
	if !ok {
		return fmt.Errorf("invalid puter deployment mode %q", c.DeploymentMode)
	}
	c.DeploymentMode = deploymentMode
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
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		c.Endpoint = ""
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid puter endpoint %q: %w", endpoint, err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("puter endpoint must use http or https: %q", endpoint)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("puter endpoint host cannot be empty: %q", endpoint)
	}
	if parsed.User != nil {
		return fmt.Errorf("puter endpoint must not embed credentials")
	}
	switch c.DeploymentMode {
	case DeploymentLocal:
		if !isLocalLoopbackHost(host) {
			return fmt.Errorf("local deployment endpoint must target loopback host, got %q", host)
		}
	case DeploymentSelfHosted:
		// Explicit host/network selection is allowed by operator policy.
	case DeploymentSaaS:
		if scheme != "https" && !c.AllowInsecureSaaSEndpoint {
			return fmt.Errorf("saas deployment endpoint must use https unless explicitly allowed")
		}
	}
	c.Endpoint = endpoint
	return nil
}

func isLocalLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsLoopback()
}
