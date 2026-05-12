package puteradapter

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
