package selfmod

import (
	"os"
	"testing"
)

func TestNewToolStagingPathAcceptsSafeName(t *testing.T) {
	p, err := NewToolStagingPath("safe_tool")
	if err != nil {
		t.Fatalf("expected safe tool name accepted: %v", err)
	}
	if got := p.String(); got != ".gorkbot/staging/tools/safe_tool.go" {
		t.Fatalf("unexpected path: %s", got)
	}
}

func TestNewToolStagingPathRejectsUnsafeNames(t *testing.T) {
	cases := []string{
		"../../../pkg/governance/policy",
		"foo/bar",
		"foo\\bar",
		".hidden",
		"-flag",
		"foo\x00bar",
		"foo\nbar",
	}
	for _, tc := range cases {
		if _, err := NewToolStagingPath(tc); err == nil {
			t.Fatalf("expected unsafe name rejected: %q", tc)
		}
	}
}

func TestWriteStagedFileWritesThroughSafeStagingPath(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	p, err := NewToolStagingPath("safe_tool")
	if err != nil {
		t.Fatalf("new staging path: %v", err)
	}
	if err := WriteStagedFile(p, []byte("package main\n"), 0600); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	if _, err := os.Stat(p.String()); err != nil {
		t.Fatalf("expected staged file to exist: %v", err)
	}
}
