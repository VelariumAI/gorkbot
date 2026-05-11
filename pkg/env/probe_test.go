package env

import (
	"strings"
	"testing"
	"time"
)

func TestExtractVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Python 3.12.4", "3.12.4"},
		{"node v20.11.0", "v20.11.0"},
		{"go version go1.22.5 linux/amd64", "1.22.5"},
		{"openjdk version \"21.0.3\" 2024-04-16", "\"21.0.3\""},
		{"custom runtime 9.9.9", "9.9.9"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractVersion(tc.in); got != tc.want {
			t.Fatalf("extractVersion(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildContextContainsCoreSections(t *testing.T) {
	snap := &EnvSnapshot{
		OS:        "linux",
		Arch:      "amd64",
		Shell:     "/bin/bash",
		ProbeTime: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		UID:       1000,
		IsRoot:    false,
		Python3:   RuntimeInfo{Found: true, Version: "3.11.9"},
		Node:      RuntimeInfo{Found: true, Version: "v20.0.0"},
		GoRT:      RuntimeInfo{Found: true, Version: "1.22.0"},
		Java:      RuntimeInfo{Found: false},
		APIKeys: map[string]bool{
			"XAI": true, "GEMINI": false, "ANTHROPIC": true, "OPENAI": false, "BRAVE": false, "GITHUB": true,
		},
		PythonPkgs: map[string]bool{
			"mcp": true, "fastmcp": true, "google-genai": false, "requests": true, "anthropic": false, "openai": true, "httpx": true,
		},
		CLITools: map[string]bool{
			"git": true, "curl": true, "wget": false, "nmap": false, "ffmpeg": false, "pkg": false, "npm": true, "npx": true, "pip3": true, "adb": false, "jq": true, "sudo": true,
		},
		MCPServers: []MCPServerStatus{{Name: "filesystem", Running: true, ToolCount: 5}},
	}

	ctx := buildContext(snap)
	for _, mustContain := range []string{
		"### GORKBOT ENVIRONMENT",
		"Platform  : linux/amd64",
		"Privilege : UID=1000",
		"Runtimes  :",
		"API keys  :",
		"Python pkg:",
		"CLI tools :",
		"MCP status: filesystem=running(5 tools)",
		"BEFORE calling any tool",
	} {
		if !strings.Contains(ctx, mustContain) {
			t.Fatalf("context missing required fragment %q:\n%s", mustContain, ctx)
		}
	}
}

func TestEnvProbePermissiveBeforeFirstProbe(t *testing.T) {
	p := NewEnvProbe(nil)
	if got := p.BuildSystemContext(); got != "" {
		t.Fatalf("expected empty context before probe, got %q", got)
	}
	if !p.HasBinary("git") {
		t.Fatal("expected HasBinary to be permissive before first probe")
	}
	if !p.HasPythonPackage("google.genai") {
		t.Fatal("expected HasPythonPackage to be permissive before first probe")
	}
}

func TestHasPythonPackageUsesImportAliasMapping(t *testing.T) {
	p := NewEnvProbe(nil)
	p.snapshot = &EnvSnapshot{
		PythonPkgs: map[string]bool{
			"google-genai": true,
			"mcp":          false,
		},
		CLITools: map[string]bool{},
		APIKeys:  map[string]bool{},
	}
	if !p.HasPythonPackage("google.genai") {
		t.Fatal("expected import alias mapping lookup to succeed")
	}
	if p.HasPythonPackage("mcp") {
		t.Fatal("expected explicit false package entry to be honored")
	}
}

func TestSetMCPStatusUpdatesSnapshot(t *testing.T) {
	p := NewEnvProbe(nil)
	p.snapshot = &EnvSnapshot{
		PythonPkgs: map[string]bool{},
		CLITools:   map[string]bool{},
		APIKeys:    map[string]bool{},
	}

	statuses := []MCPServerStatus{
		{Name: "fs", Running: true, ToolCount: 2},
		{Name: "github", Running: false, Error: "token missing"},
	}
	p.SetMCPStatus(statuses)
	if len(p.snapshot.MCPServers) != 2 {
		t.Fatalf("expected 2 MCP statuses, got %d", len(p.snapshot.MCPServers))
	}
}

func TestDetectAPIKeys(t *testing.T) {
	t.Setenv("XAI_API_KEY", "x")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "a")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BRAVE_API_KEY", "")
	t.Setenv("GITHUB_PERSONAL_ACCESS_TOKEN", "g")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "o")

	got := detectAPIKeys()
	if !got["XAI"] || !got["ANTHROPIC"] || !got["GITHUB"] || !got["OPENROUTER"] {
		t.Fatalf("expected true keys to be detected, got %+v", got)
	}
	if got["GEMINI"] || got["OPENAI"] || got["BRAVE"] || got["MINIMAX"] {
		t.Fatalf("expected empty keys to be false, got %+v", got)
	}
}

func TestCountTrue(t *testing.T) {
	got := countTrue(map[string]bool{"a": true, "b": false, "c": true})
	if got != 2 {
		t.Fatalf("countTrue returned %d want 2", got)
	}
}
