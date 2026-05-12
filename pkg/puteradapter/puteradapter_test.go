package puteradapter

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConfigDefaultsAndWorkspaceOffNoEndpointRequired(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != WorkspaceOff {
		t.Fatalf("default mode=%q want=%q", cfg.Mode, WorkspaceOff)
	}
	if cfg.DeploymentMode != DeploymentLocal {
		t.Fatalf("default deployment mode=%q want=%q", cfg.DeploymentMode, DeploymentLocal)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("default endpoint=%q want empty", cfg.Endpoint)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate default config: %v", err)
	}
}

func TestParseDeploymentMode(t *testing.T) {
	if mode, ok := ParseDeploymentMode(" self_hosted "); !ok || mode != DeploymentSelfHosted {
		t.Fatalf("expected self_hosted mode parse success, got mode=%q ok=%v", mode, ok)
	}
	if _, ok := ParseDeploymentMode("edge"); ok {
		t.Fatalf("expected invalid deployment mode to fail parse")
	}
}

func TestConfigValidateLocalAcceptsLoopback(t *testing.T) {
	cases := []string{
		"http://localhost:4100",
		"http://127.0.0.1:4100",
		"http://[::1]:4100",
	}
	for _, endpoint := range cases {
		cfg := DefaultConfig()
		cfg.Mode = WorkspaceAudit
		cfg.DeploymentMode = DeploymentLocal
		cfg.Endpoint = endpoint
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected local endpoint %q accepted, got %v", endpoint, err)
		}
	}
}

func TestConfigValidateLocalRejectsPublicEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = WorkspaceAudit
	cfg.DeploymentMode = DeploymentLocal
	cfg.Endpoint = "http://example.com:4100"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected non-loopback local endpoint to be rejected")
	}
}

func TestConfigValidateSaaSRequiresHTTPSUnlessExplicitlyAllowed(t *testing.T) {
	cfgHTTPS := DefaultConfig()
	cfgHTTPS.Mode = WorkspaceAudit
	cfgHTTPS.DeploymentMode = DeploymentSaaS
	cfgHTTPS.Endpoint = "https://puter.example.com"
	if err := cfgHTTPS.Validate(); err != nil {
		t.Fatalf("expected saas https endpoint accepted: %v", err)
	}

	cfgHTTP := DefaultConfig()
	cfgHTTP.Mode = WorkspaceAudit
	cfgHTTP.DeploymentMode = DeploymentSaaS
	cfgHTTP.Endpoint = "http://puter.example.com"
	if err := cfgHTTP.Validate(); err == nil {
		t.Fatalf("expected saas http endpoint rejected without explicit allow")
	}

	cfgHTTPAllow := DefaultConfig()
	cfgHTTPAllow.Mode = WorkspaceAudit
	cfgHTTPAllow.DeploymentMode = DeploymentSaaS
	cfgHTTPAllow.Endpoint = "http://puter.example.com"
	cfgHTTPAllow.AllowInsecureSaaSEndpoint = true
	if err := cfgHTTPAllow.Validate(); err != nil {
		t.Fatalf("expected saas http endpoint accepted when explicitly allowed: %v", err)
	}
}

func TestConfigValidateSelfHostedAllowsHTTPOrHTTPS(t *testing.T) {
	cases := []string{
		"http://10.0.0.12:4100",
		"https://puter.internal.example",
	}
	for _, endpoint := range cases {
		cfg := DefaultConfig()
		cfg.Mode = WorkspaceAudit
		cfg.DeploymentMode = DeploymentSelfHosted
		cfg.Endpoint = endpoint
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected self_hosted endpoint %q accepted, got %v", endpoint, err)
		}
	}
}

func TestConfigValidateRejectsInvalidEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = WorkspaceAudit
	cfg.DeploymentMode = DeploymentSelfHosted
	cfg.Endpoint = "://bad endpoint"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid endpoint to be rejected")
	}
}

type mockClient struct {
	lastFSWritePath PuterWorkspacePath
	lastKVSetKey    PuterKVKey
	readPayload     []byte
}

func (m *mockClient) FSRead(_ context.Context, path PuterWorkspacePath) ([]byte, error) {
	m.lastFSWritePath = path
	if len(m.readPayload) == 0 {
		return []byte("read-ok"), nil
	}
	return m.readPayload, nil
}

func (m *mockClient) FSWrite(_ context.Context, path PuterWorkspacePath, _ []byte) error {
	m.lastFSWritePath = path
	return nil
}

func (m *mockClient) FSDelete(_ context.Context, _ PuterWorkspacePath) error {
	return nil
}

func (m *mockClient) FSMove(_ context.Context, _, _ PuterWorkspacePath) error {
	return nil
}

func (m *mockClient) KVGet(_ context.Context, key PuterKVKey) ([]byte, error) {
	m.lastKVSetKey = key
	return []byte("kv-ok"), nil
}

func (m *mockClient) KVSet(_ context.Context, key PuterKVKey, _ []byte) error {
	m.lastKVSetKey = key
	return nil
}

func (m *mockClient) KVDelete(_ context.Context, _ PuterKVKey) error {
	return nil
}

func TestWorkspaceManifestDefaults(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	if got, want := manifest.Root(), "/Gorkbot"; got != want {
		t.Fatalf("root=%q want=%q", got, want)
	}
	if manifest.PuterRepo() != "VelariumAI/puter" {
		t.Fatalf("unexpected repo: %s", manifest.PuterRepo())
	}
	if manifest.PuterRef() == "" {
		t.Fatalf("expected pinned puter ref")
	}
	if len(manifest.InspectedDocs()) == 0 {
		t.Fatalf("expected inspected docs")
	}
}

func TestReceiptDoesNotIncludeFileContent(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	client := &mockClient{}
	cfg := DefaultConfig()
	cfg.Mode = WorkspaceEnforce
	adapter, err := NewAdapter(cfg, manifest, DefaultCapabilityPolicy(), client)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	adapter.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	adapter.newID = func() string { return "op-fixed" }

	decision, receipt, err := adapter.WriteFile(context.Background(), "/Gorkbot/scratch/out.txt", []byte("top-secret-body"))
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected write allowed")
	}
	if receipt.Bytes == 0 || receipt.SHA256 == "" {
		t.Fatalf("expected bounded hash metadata in receipt")
	}
	if receipt.Path != "/Gorkbot/scratch/out.txt" {
		t.Fatalf("unexpected receipt path: %s", receipt.Path)
	}
}

func TestMockClientReceivesOnlySafeTypedArtifacts(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	client := &mockClient{}
	cfg := DefaultConfig()
	cfg.Mode = WorkspaceEnforce
	adapter, err := NewAdapter(cfg, manifest, DefaultCapabilityPolicy(), client)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	if _, _, err := adapter.WriteFile(context.Background(), "/Gorkbot/scratch/typed.txt", []byte("x")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := client.lastFSWritePath.String(); got != "/Gorkbot/scratch/typed.txt" {
		t.Fatalf("unexpected safe path passed to client: %q", got)
	}

	if _, _, err := adapter.KVSet(context.Background(), "gorkbot.mission.alpha", []byte("ok")); err != nil {
		t.Fatalf("kv set: %v", err)
	}
	if got := client.lastKVSetKey.String(); got != "gorkbot.mission.alpha" {
		t.Fatalf("unexpected safe key passed to client: %q", got)
	}
}

func TestDeleteAndKVDeleteRequireApprovalInEnforceMode(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	client := &mockClient{}
	cfg := DefaultConfig()
	cfg.Mode = WorkspaceEnforce
	adapter, err := NewAdapter(cfg, manifest, DefaultCapabilityPolicy(), client)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	decision, _, err := adapter.DeleteFile(context.Background(), "/Gorkbot/scratch/a.txt")
	if !errors.Is(err, ErrRequiresApproval) {
		t.Fatalf("expected requires-approval error, got %v", err)
	}
	if !decision.RequiresApproval {
		t.Fatalf("expected decision to require approval")
	}

	kvDecision, _, err := adapter.KVDelete(context.Background(), "gorkbot.mission.alpha")
	if !errors.Is(err, ErrRequiresApproval) {
		t.Fatalf("expected kv delete requires approval, got %v", err)
	}
	if !kvDecision.RequiresApproval {
		t.Fatalf("expected kv delete decision to require approval")
	}
}
