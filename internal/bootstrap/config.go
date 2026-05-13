package bootstrap

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/puteradapter"
)

// ResolveRenderGuardOnUnavailable applies CLI normalization/defaulting for
// renderer-guard unavailable behavior.
func ResolveRenderGuardOnUnavailable(raw string, mode governance.Mode) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value != "" &&
		value != governance.RenderGuardUnavailableBlock &&
		value != governance.RenderGuardUnavailableDowngrade &&
		value != governance.RenderGuardUnavailableAudit {
		return "", fmt.Errorf("invalid --render-guard-on-unavailable value %q (expected block|downgrade|audit)", raw)
	}
	if value == "" {
		if mode == governance.GOVERNANCE_AUDIT {
			value = governance.RenderGuardUnavailableAudit
		} else {
			value = governance.RenderGuardUnavailableDowngrade
		}
	}
	return value, nil
}

// ResolveResearchEgressMode applies CLI normalization/defaulting for research
// egress mode.
func ResolveResearchEgressMode(raw string, mode governance.Mode) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		if mode == governance.GOVERNANCE_OFF {
			value = "off"
		} else {
			value = "enforce"
		}
	}
	if value != "off" && value != "audit" && value != "enforce" {
		return "", fmt.Errorf("invalid --research-egress value %q (expected off|audit|enforce)", raw)
	}
	return value, nil
}

// BuildPuterConfig builds and validates Puter workspace adapter config from CLI
// values without changing defaults or validation semantics.
func BuildPuterConfig(workspaceMode, root, repo, ref, deploymentMode, endpoint string) (puteradapter.Config, error) {
	mode, ok := puteradapter.ParseWorkspaceMode(workspaceMode)
	if !ok {
		return puteradapter.Config{}, fmt.Errorf("invalid --puter-workspace value %q (expected off|audit|enforce)", workspaceMode)
	}
	deployment, ok := puteradapter.ParseDeploymentMode(deploymentMode)
	if !ok {
		return puteradapter.Config{}, fmt.Errorf("invalid --puter-deployment value %q (expected local|self_hosted|saas)", deploymentMode)
	}
	cfg := puteradapter.DefaultConfig()
	cfg.Mode = mode
	cfg.Root = root
	cfg.PuterRepo = repo
	cfg.PuterRef = ref
	cfg.DeploymentMode = deployment
	cfg.Endpoint = endpoint
	if err := cfg.Validate(); err != nil {
		return puteradapter.Config{}, fmt.Errorf("invalid puter workspace config: %v", err)
	}
	return cfg, nil
}
