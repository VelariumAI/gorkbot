package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func TestContextInjector_CollectHierarchyReadmeRules(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	sub := filepath.Join(proj, "service")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	mustWrite(t, filepath.Join(proj, "GORKBOT.md"), "root instructions")
	mustWrite(t, filepath.Join(sub, "GORKBOT.md"), "sub instructions")
	mustWrite(t, filepath.Join(sub, "README.md"), strings.Repeat("A", 4500))
	mustWrite(t, filepath.Join(proj, ".gorkbot", "rules", "policy.md"), "rule one")
	mustWrite(t, filepath.Join(sub, ".gorkbot", "rules", "local.md"), "rule two")

	ci := NewContextInjector()
	ctx := ci.Collect(sub)

	if ctx.SystemPromptPrefix == "" {
		t.Fatalf("expected injected context")
	}
	if len(ctx.Sources) < 4 {
		t.Fatalf("expected multiple sources, got %d", len(ctx.Sources))
	}
	if ctx.TotalBytes <= 0 {
		t.Fatalf("expected positive total bytes")
	}

	text := ctx.SystemPromptPrefix
	if !strings.Contains(text, "PROJECT CONTEXT") || !strings.Contains(text, "END OF PROJECT CONTEXT") {
		t.Fatalf("missing context boundaries")
	}
	if !strings.Contains(text, "root instructions") || !strings.Contains(text, "sub instructions") {
		t.Fatalf("missing GORKBOT hierarchy content")
	}
	if !strings.Contains(text, "Project README (summary context)") {
		t.Fatalf("missing readme section")
	}
	if !strings.Contains(text, "truncated for context brevity") {
		t.Fatalf("expected README truncation note")
	}
	if !strings.Contains(text, "Project Rule") {
		t.Fatalf("missing rules section")
	}
}

func TestContextInjector_EmptyAndLimits(t *testing.T) {
	d := t.TempDir()
	ci := NewContextInjector()
	empty := ci.Collect(d)
	if empty.SystemPromptPrefix != "" || len(empty.Sources) != 0 || empty.TotalBytes != 0 {
		t.Fatalf("expected empty context when no files are present")
	}

	mustWrite(t, filepath.Join(d, "GORKBOT.md"), "very large content here")
	ci.MaxTotalSize = 10 // too small for blob wrapper + content
	limited := ci.Collect(d)
	if limited.SystemPromptPrefix != "" {
		t.Fatalf("expected no injected content when max total size is too small")
	}
}

func TestContextInjector_Helpers(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(a, "b")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mustWrite(t, filepath.Join(root, "GORKBOT.md"), "root")
	mustWrite(t, filepath.Join(a, "GORKBOT.md"), "a")
	mustWrite(t, filepath.Join(a, ".gorkbot", "rules", "r.md"), "r")

	ci := NewContextInjector()
	files := ci.findFilesUpward(b, "GORKBOT.md")
	if len(files) != 2 {
		t.Fatalf("expected two upward files, got %d", len(files))
	}
	dirs := ci.findDirsUpward(b, filepath.Join(".gorkbot", "rules"))
	if len(dirs) != 1 {
		t.Fatalf("expected one rules dir, got %d", len(dirs))
	}

	if got := ci.truncate("abcdef", 3); got != "abc" {
		t.Fatalf("unexpected truncation result: %q", got)
	}
	if got := ci.truncate("abc", 10); got != "abc" {
		t.Fatalf("unexpected no-op truncation result: %q", got)
	}
	if got := ci.readLimited(filepath.Join(root, "missing.md")); got != "" {
		t.Fatalf("expected empty read for missing file")
	}
	if got := ci.relPath(root, filepath.Join(a, "GORKBOT.md")); !strings.Contains(got, "a") {
		t.Fatalf("unexpected relative path: %q", got)
	}
}
