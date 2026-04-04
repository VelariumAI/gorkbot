package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_RejectsInvalidSemver(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "badsemver")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
name: bad-semver
version: v1
description: invalid semver
tools:
  - name: read_file
`
	if err := os.WriteFile(filepath.Join(skillDir, ".gorkskill.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	reg := NewInMemoryRegistry(slog.Default())
	loader := NewLoader(reg, slog.Default())
	_, err := loader.LoadManifest(filepath.Join(skillDir, ".gorkskill.yaml"))
	if err == nil || !strings.Contains(err.Error(), "semantic version") {
		t.Fatalf("expected semantic version validation error, got %v", err)
	}
}

func TestLoader_LintDirectory(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "lintme")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
name: lint-me
version: 1.2.3
description: lint target
permissions:
  - tool: bash
    level: superuser
`
	if err := os.WriteFile(filepath.Join(skillDir, ".gorkskill.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	reg := NewInMemoryRegistry(slog.Default())
	loader := NewLoader(reg, slog.Default())
	issues := loader.LintDirectory(dir)
	if len(issues) == 0 {
		t.Fatalf("expected lint issues, got none")
	}
}
