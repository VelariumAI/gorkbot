package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrainFileVersion(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "SILENCE.md")

	if got := brainFileVersion(p); got != "" {
		t.Fatalf("expected empty version for missing file, got %q", got)
	}

	if err := os.WriteFile(p, []byte("hello\n<!-- gorkbot-brain-v2 -->\n"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := brainFileVersion(p); got != "v2" {
		t.Fatalf("expected v2 marker, got %q", got)
	}

	if err := os.WriteFile(p, []byte("no marker"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := brainFileVersion(p); got != "" {
		t.Fatalf("expected empty version without marker, got %q", got)
	}
}

func TestGetDynamicBrainContext_CreatesAndLoadsFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ctx := GetDynamicBrainContext()
	if !strings.Contains(ctx, "DYNAMIC BRAIN CONTEXT") {
		t.Fatalf("expected dynamic brain header")
	}
	if !strings.Contains(ctx, "[SOUL.md]") || !strings.Contains(ctx, "[IDENTITY.md]") {
		t.Fatalf("expected loaded brain sections, got: %s", ctx)
	}

	brainDir := filepath.Join(home, ".gorkbot", "brain")
	if _, err := os.Stat(filepath.Join(brainDir, "SILENCE.md")); err != nil {
		t.Fatalf("expected SILENCE.md to be created: %v", err)
	}
	if got := brainFileVersion(filepath.Join(brainDir, "SILENCE.md")); got != "v2" {
		t.Fatalf("expected generated SILENCE.md to be v2, got %q", got)
	}
}

func TestGetDynamicBrainContext_RegeneratesOnStaleVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brainDir := filepath.Join(home, ".gorkbot", "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "SILENCE.md"), []byte("stale version"), 0o644); err != nil {
		t.Fatalf("write stale SILENCE failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "SOUL.md"), []byte("old soul"), 0o644); err != nil {
		t.Fatalf("write old SOUL failed: %v", err)
	}

	ctx := GetDynamicBrainContext()
	if !strings.Contains(ctx, "DYNAMIC BRAIN CONTEXT") {
		t.Fatalf("expected context after regeneration")
	}
	if got := brainFileVersion(filepath.Join(brainDir, "SILENCE.md")); got != "v2" {
		t.Fatalf("expected regenerated SILENCE marker v2, got %q", got)
	}
	if strings.Contains(ctx, "old soul") {
		t.Fatalf("expected regenerated content to replace stale files")
	}
}
