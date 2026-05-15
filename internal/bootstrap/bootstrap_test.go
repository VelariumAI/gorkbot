package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/tools"
)

type testProvider struct {
	id   registry.ProviderID
	name string
}

func (p *testProvider) Generate(context.Context, string) (string, error) { return "", nil }
func (p *testProvider) GenerateWithHistory(context.Context, *ai.ConversationHistory) (string, error) {
	return "", nil
}
func (p *testProvider) Stream(context.Context, string, io.Writer) error { return nil }
func (p *testProvider) StreamWithHistory(context.Context, *ai.ConversationHistory, io.Writer) error {
	return nil
}
func (p *testProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: string(p.id), Name: p.name}
}
func (p *testProvider) Name() string                   { return p.name }
func (p *testProvider) ID() registry.ProviderID        { return p.id }
func (p *testProvider) Ping(context.Context) error     { return nil }
func (p *testProvider) WithModel(string) ai.AIProvider { return p }
func (p *testProvider) FetchModels(context.Context) ([]registry.ModelDefinition, error) {
	return nil, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestResolveResearchEgressModeDefaults(t *testing.T) {
	offMode, err := ResolveResearchEgressMode("", governance.GOVERNANCE_OFF)
	if err != nil {
		t.Fatalf("resolve off default: %v", err)
	}
	if offMode != "off" {
		t.Fatalf("expected off default, got %q", offMode)
	}
	enforceMode, err := ResolveResearchEgressMode("", governance.GOVERNANCE_ENFORCE)
	if err != nil {
		t.Fatalf("resolve enforce default: %v", err)
	}
	if enforceMode != "enforce" {
		t.Fatalf("expected enforce default, got %q", enforceMode)
	}
}

func TestResolveHarnessModeDefaults(t *testing.T) {
	if got := ResolveHarnessMode(""); got != harness.ModeOff {
		t.Fatalf("expected default harness mode off, got %q", got)
	}
	if got := ResolveHarnessMode("audit"); got != harness.ModeAudit {
		t.Fatalf("expected harness mode audit, got %q", got)
	}
	if got := ResolveHarnessMode("unknown"); got != harness.ModeOff {
		t.Fatalf("expected unknown harness mode to resolve off, got %q", got)
	}
}

func TestResolveRenderGuardDefaults(t *testing.T) {
	auditMode, err := ResolveRenderGuardOnUnavailable("", governance.GOVERNANCE_AUDIT)
	if err != nil {
		t.Fatalf("resolve audit default: %v", err)
	}
	if auditMode != governance.RenderGuardUnavailableAudit {
		t.Fatalf("expected audit default, got %q", auditMode)
	}
	defaultMode, err := ResolveRenderGuardOnUnavailable("", governance.GOVERNANCE_ENFORCE)
	if err != nil {
		t.Fatalf("resolve enforce default: %v", err)
	}
	if defaultMode != governance.RenderGuardUnavailableDowngrade {
		t.Fatalf("expected downgrade default, got %q", defaultMode)
	}
}

func TestBuildPuterConfigWorkspaceOffAllowsEmptyEndpoint(t *testing.T) {
	cfg, err := BuildPuterConfig("off", "/Gorkbot", "VelariumAI/puter", "f80016e4e6a6f8062b737a415a0b7c18008ade98", "local", "")
	if err != nil {
		t.Fatalf("build puter config: %v", err)
	}
	if cfg.Mode != "off" {
		t.Fatalf("expected off mode, got %q", cfg.Mode)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("expected empty endpoint, got %q", cfg.Endpoint)
	}
}

func TestSetupGovernanceOffVsEnabled(t *testing.T) {
	pm, err := tools.NewPermissionManager(t.TempDir())
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg := tools.NewRegistry(pm)

	off := SetupGovernance(GovernanceSetupOptions{
		Mode:                     governance.GOVERNANCE_OFF,
		WorkspaceRoot:            "/tmp",
		VCSEURL:                  "http://127.0.0.1:8000",
		VCSETimeout:              250 * time.Millisecond,
		VCSEEnabledExplicit:      false,
		ApprovalTimeout:          30 * time.Second,
		MaxInflightApprovals:     4,
		NoApprovalCache:          false,
		RenderGuardTimeout:       750 * time.Millisecond,
		RenderGuardOnUnavailable: governance.RenderGuardUnavailableDowngrade,
		ToolRegistry:             reg,
		Logger:                   testLogger(),
	})
	if off.GovernorWired {
		t.Fatalf("governance off should not wire governor")
	}
	if off.Governor == nil {
		t.Fatalf("expected governor struct")
	}
	if off.Governor.ApprovalRuntime != nil {
		t.Fatalf("governance off should not create approval runtime")
	}

	enabled := SetupGovernance(GovernanceSetupOptions{
		Mode:                     governance.GOVERNANCE_ENFORCE,
		WorkspaceRoot:            "/tmp",
		VCSEURL:                  "http://127.0.0.1:8000",
		VCSETimeout:              250 * time.Millisecond,
		VCSEEnabledExplicit:      true,
		ApprovalTimeout:          30 * time.Second,
		MaxInflightApprovals:     4,
		NoApprovalCache:          false,
		RenderGuardTimeout:       750 * time.Millisecond,
		RenderGuardOnUnavailable: governance.RenderGuardUnavailableDowngrade,
		ToolRegistry:             reg,
		Logger:                   testLogger(),
	})
	if !enabled.GovernorWired {
		t.Fatalf("governance enabled should wire governor")
	}
	if enabled.Governor.ApprovalRuntime == nil {
		t.Fatalf("governance enabled should create approval runtime")
	}
	enabled.Shutdown()
	enabled.Shutdown()
}

func TestSetupProvidersSelectorInjection(t *testing.T) {
	var gotPrimary string
	var gotConsultant string
	provPrimary := &testProvider{id: "xai", name: "Primary"}
	provConsultant := &testProvider{id: "google", name: "Consultant"}

	setup, err := SetupProviders(ProviderSetupOptions{
		ConfigDir:          t.TempDir(),
		Logger:             testLogger(),
		VerboseThoughts:    false,
		PrimaryOverride:    "xai",
		ConsultantOverride: "google",
		SelectProviders: func(_ *registry.ModelRegistry, primaryOverride, consultantOverride string, _ *slog.Logger) (ai.AIProvider, ai.AIProvider, error) {
			gotPrimary = primaryOverride
			gotConsultant = consultantOverride
			return provPrimary, provConsultant, nil
		},
	})
	if err != nil {
		t.Fatalf("setup providers: %v", err)
	}
	if gotPrimary != "xai" || gotConsultant != "google" {
		t.Fatalf("overrides not passed through selector: %q %q", gotPrimary, gotConsultant)
	}
	if setup.Primary != provPrimary || setup.Consultant != provConsultant {
		t.Fatalf("selector providers not propagated")
	}
}
