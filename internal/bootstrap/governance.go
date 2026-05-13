package bootstrap

import (
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/pkg/execution"
	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/vcseclient"
)

type GovernanceSetupOptions struct {
	Mode                     governance.Mode
	WorkspaceRoot            string
	VCSEURL                  string
	VCSETimeout              time.Duration
	VCSEEnabledExplicit      bool
	ApprovalTimeout          time.Duration
	MaxInflightApprovals     int
	NoApprovalCache          bool
	RenderGuardTimeout       time.Duration
	RenderGuardOnUnavailable string
	ToolRegistry             *tools.Registry
	Logger                   *slog.Logger
}

type GovernanceSetup struct {
	Governor      *governance.Governor
	VCSEEnabled   bool
	Shutdown      func()
	GovernorWired bool
}

func SetupGovernance(opts GovernanceSetupOptions) *GovernanceSetup {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	vcseEnabled := opts.Mode != governance.GOVERNANCE_OFF || opts.VCSEEnabledExplicit
	govPolicy := governance.DefaultPolicy()
	govPolicy.Mode = opts.Mode
	govPolicy.WorkspaceRoot = opts.WorkspaceRoot

	gov := &governance.Governor{
		Policy:               govPolicy,
		Budget:               execution.DefaultBudget(),
		Breakers:             execution.NewDefaultBreakerSet(),
		Progress:             execution.NewProgressTracker(),
		ApprovalHandler:      opts.ToolRegistry,
		ApprovalTimeout:      opts.ApprovalTimeout,
		MaxInflightApprovals: opts.MaxInflightApprovals,
		RenderGuardTimeout:   opts.RenderGuardTimeout,
		RenderGuardPolicy: governance.RendererGuardPolicy{
			RenderMode: string(governance.RENDER_MODE_CANONICAL_ONLY),
		},
		RenderGuardOnUnavailable: opts.RenderGuardOnUnavailable,
	}
	if !opts.NoApprovalCache {
		gov.ApprovalCache = governance.NewApprovalCache()
	}
	if vcseEnabled {
		gov.VCSE = vcseclient.New(vcseclient.Config{
			BaseURL: opts.VCSEURL,
			Timeout: opts.VCSETimeout,
			Enabled: true,
		})
	}

	setup := &GovernanceSetup{Governor: gov, VCSEEnabled: vcseEnabled, Shutdown: func() {}}
	if opts.Mode != governance.GOVERNANCE_OFF {
		gov.ApprovalRuntime = governance.NewApprovalRuntime(opts.MaxInflightApprovals)
		if opts.ToolRegistry != nil {
			opts.ToolRegistry.SetGovernor(gov)
		}
		setup.GovernorWired = true
		setup.Shutdown = gov.Shutdown
		logger.Info("Governance enabled",
			"mode", opts.Mode,
			"vcse_enabled", vcseEnabled,
			"vcse_url", opts.VCSEURL,
			"vcse_timeout", opts.VCSETimeout.String(),
			"approval_timeout", opts.ApprovalTimeout.String(),
			"approval_max_inflight", gov.ApprovalRuntime.MaxInflight(),
			"approval_cache_enabled", !opts.NoApprovalCache,
			"render_guard_timeout", opts.RenderGuardTimeout.String(),
			"render_guard_on_unavailable", opts.RenderGuardOnUnavailable,
		)
	} else {
		logger.Info("Governance disabled", "mode", opts.Mode)
	}

	return setup
}
