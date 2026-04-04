// Package env provides host environment probing for Gorkbot.
//
// EnvProbe runs concurrent detections for runtimes, Python packages, CLI
// tools, and API key presence, then caches the result as an EnvSnapshot.
// BuildSystemContext() formats the snapshot into a compact (~30 line) block
// injected into the AI system prompt so the AI understands its operating
// constraints before it attempts any tool calls.
//
// Design goals:
//   - Zero shell glob: uses exec.LookPath + single python3 -c subprocess only.
//   - No import cycle: pkg/env has no deps on pkg/tools or internal/engine.
//   - Thread-safe: Probe/Snapshot/SetMCPStatus safe for concurrent callers.
package env

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// MCPServerStatus holds the runtime status of a single MCP server.
type MCPServerStatus struct {
	Name      string
	Running   bool
	ToolCount int
	Error     string
}

// RuntimeInfo describes a detected runtime executable.
type RuntimeInfo struct {
	Path    string
	Version string
	Found   bool
}

// EnvSnapshot is an immutable point-in-time picture of the host environment.
// All map fields are non-nil after a successful Probe call.
type EnvSnapshot struct {
	// Platform
	OS        string
	Arch      string
	IsTermux  bool
	Shell     string
	Home      string
	ProbeTime time.Time

	// Privilege
	UID    int  // effective user ID (0 = root)
	IsRoot bool // true when UID == 0

	// Runtimes
	Python3 RuntimeInfo
	Node    RuntimeInfo
	GoRT    RuntimeInfo
	Java    RuntimeInfo

	// Python packages present (package name → installed).
	// Checked: mcp, fastmcp, google-genai, requests, anthropic, openai, httpx
	PythonPkgs map[string]bool

	// CLI tools present (tool name → found in PATH).
	CLITools map[string]bool

	// API key presence (short name → env var non-empty).
	// Never stores key values — boolean only.
	APIKeys map[string]bool

	// MCP server statuses — populated by SetMCPStatus after LoadAndStart.
	MCPServers []MCPServerStatus
}

// EnvProbe probes the host environment and caches the result as an
// EnvSnapshot. Create one at startup with NewEnvProbe; call Probe once
// (or RefreshAsync when the environment changes). All methods are safe for
// concurrent use.
type EnvProbe struct {
	mu       sync.RWMutex
	snapshot *EnvSnapshot
	logger   *slog.Logger
}

// NewEnvProbe creates an EnvProbe. Call Probe(ctx) to populate the snapshot.
func NewEnvProbe(logger *slog.Logger) *EnvProbe {
	return &EnvProbe{logger: logger}
}

// Probe runs all detections concurrently and stores the result.
// It returns the new snapshot immediately.  Safe to call multiple times.
func (p *EnvProbe) Probe(ctx context.Context) *EnvSnapshot {
	uid := os.Getuid()
	snap := &EnvSnapshot{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		IsTermux:   isTermux(),
		Shell:      os.Getenv("SHELL"),
		Home:       os.Getenv("HOME"),
		ProbeTime:  time.Now(),
		UID:        uid,
		IsRoot:     uid == 0,
		PythonPkgs: make(map[string]bool),
		CLITools:   make(map[string]bool),
		APIKeys:    make(map[string]bool),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// ── Runtimes ──────────────────────────────────────────────────────────────
	for _, spec := range []struct {
		dst  *RuntimeInfo
		name string
		args []string
	}{
		{&snap.Python3, "python3", []string{"--version"}},
		{&snap.Node, "node", []string{"--version"}},
		{&snap.GoRT, "go", []string{"version"}},
		{&snap.Java, "java", []string{"-version"}},
	} {
		spec := spec
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := detectRuntime(ctx, spec.name, spec.args...)
			mu.Lock()
			*spec.dst = info
			mu.Unlock()
		}()
	}

	// ── Python packages ───────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		pkgs := detectPythonPackages(ctx)
		mu.Lock()
		snap.PythonPkgs = pkgs
		mu.Unlock()
	}()

	// ── CLI tools ─────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := detectCLITools()
		mu.Lock()
		snap.CLITools = t
		mu.Unlock()
	}()

	// ── API keys ──────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		k := detectAPIKeys()
		mu.Lock()
		snap.APIKeys = k
		mu.Unlock()
	}()

	wg.Wait()

	p.mu.Lock()
	p.snapshot = snap
	p.mu.Unlock()

	if p.logger != nil {
		p.logger.Info("Environment probe complete",
			"python3", snap.Python3.Found,
			"node", snap.Node.Found,
			"cli_tools", countTrue(snap.CLITools),
			"api_keys", countTrue(snap.APIKeys),
		)
	}
	return snap
}

// Snapshot returns the most recent cached snapshot, or nil if Probe hasn't run.
func (p *EnvProbe) Snapshot() *EnvSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snapshot
}

// SetMCPStatus updates the MCP server statuses in the cached snapshot.
// Call after mcpMgr.LoadAndStart() to record which servers are running
// and which failed, so BuildSystemContext() can report them accurately.
func (p *EnvProbe) SetMCPStatus(statuses []MCPServerStatus) {
	p.mu.Lock()
	if p.snapshot != nil {
		p.snapshot.MCPServers = statuses
	}
	p.mu.Unlock()
}

// RefreshAsync re-runs Probe in a background goroutine, updating the cache.
// Safe to call at any time (e.g. after installing a package).
func (p *EnvProbe) RefreshAsync(ctx context.Context) {
	go func() { p.Probe(ctx) }()
}

// HasBinary returns true when the named CLI tool was found in PATH during the
// most recent probe.  Returns true (permissive) when no snapshot exists yet so
// that capability checks do not block before the first probe completes.
func (p *EnvProbe) HasBinary(name string) bool {
	p.mu.RLock()
	snap := p.snapshot
	p.mu.RUnlock()
	if snap == nil {
		return true // permissive before first probe
	}
	return snap.CLITools[name]
}

// HasPythonPackage returns true when the given Python import-module name was
// importable during the most recent probe (e.g. "google.genai" for google-genai).
// Returns true (permissive) when no snapshot exists.
func (p *EnvProbe) HasPythonPackage(importName string) bool {
	p.mu.RLock()
	snap := p.snapshot
	p.mu.RUnlock()
	if snap == nil {
		return true
	}
	// PythonPkgs is keyed by pip/display name; importName may differ.
	// Try exact match first, then check if any key's value matches the import.
	// We also accept a direct key lookup (e.g. "mcp" → "mcp").
	if v, ok := snap.PythonPkgs[importName]; ok {
		return v
	}
	// Fallback: scan for import name match in the checks table.
	for _, pair := range pythonPackageChecks {
		if pair[1] == importName {
			return snap.PythonPkgs[pair[0]]
		}
	}
	return true // unknown package → permissive
}

// BuildSystemContext returns a compact environment-awareness block (~30 lines)
// for injection into the AI system prompt.  Returns "" if Probe hasn't run.
func (p *EnvProbe) BuildSystemContext() string {
	p.mu.RLock()
	snap := p.snapshot
	p.mu.RUnlock()
	if snap == nil {
		return ""
	}
	return buildContext(snap)
}

// ─── detection helpers ────────────────────────────────────────────────────────

func isTermux() bool {
	return os.Getenv("TERMUX_VERSION") != "" ||
		strings.Contains(os.Getenv("PREFIX"), "termux") ||
		strings.Contains(os.Getenv("HOME"), "com.termux")
}

func detectRuntime(ctx context.Context, name string, args ...string) RuntimeInfo {
	path, err := exec.LookPath(name)
	if err != nil {
		return RuntimeInfo{}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	out, _ := cmd.CombinedOutput() // java writes to stderr
	version := extractVersion(strings.TrimSpace(string(out)))
	return RuntimeInfo{Path: path, Version: version, Found: true}
}

// pythonPackageChecks lists (display-name, import-module) pairs.
var pythonPackageChecks = [][2]string{
	{"mcp", "mcp"},
	{"fastmcp", "fastmcp"},
	{"google-genai", "google.genai"},
	{"requests", "requests"},
	{"anthropic", "anthropic"},
	{"openai", "openai"},
	{"httpx", "httpx"},
}

func detectPythonPackages(ctx context.Context) map[string]bool {
	result := make(map[string]bool, len(pythonPackageChecks))
	for _, p := range pythonPackageChecks {
		result[p[0]] = false
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		return result
	}

	// Single subprocess: check all packages via importlib.util.find_spec.
	// Uses find_spec (not __import__) to avoid side effects.
	var scriptLines []string
	scriptLines = append(scriptLines, "import importlib.util")
	scriptLines = append(scriptLines, "checks=[")
	for _, p := range pythonPackageChecks {
		scriptLines = append(scriptLines, fmt.Sprintf("  (%q,%q),", p[0], p[1]))
	}
	scriptLines = append(scriptLines, "]")
	scriptLines = append(scriptLines, `
for name,mod in checks:
    try:
        found=importlib.util.find_spec(mod) is not None
    except (ModuleNotFoundError,ValueError,AttributeError):
        found=False
    print(f'{name}={"1" if found else "0"}')`)

	script := strings.Join(scriptLines, "\n")
	cmd := exec.CommandContext(ctx, python, "-c", script)
	out, err := cmd.Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1] == "1"
		}
	}
	return result
}

var cliToolNames = []string{
	"git", "curl", "wget", "nmap", "ffmpeg", "pkg", "npm", "npx", "pip3",
	"adb", "pandoc", "jq", "tmux", "ssh", "tar", "zip", "unzip",
	"docker", "kubectl", "terraform",
	// Privilege escalation
	"sudo", "su",
}

func detectCLITools() map[string]bool {
	result := make(map[string]bool, len(cliToolNames))
	for _, name := range cliToolNames {
		_, err := exec.LookPath(name)
		result[name] = err == nil
	}
	return result
}

// apiKeyEnvVars maps a short display name to the environment variable that
// holds the key.  We only check presence (non-empty), never the value.
var apiKeyEnvVars = map[string]string{
	"XAI":        "XAI_API_KEY",
	"GEMINI":     "GEMINI_API_KEY",
	"ANTHROPIC":  "ANTHROPIC_API_KEY",
	"OPENAI":     "OPENAI_API_KEY",
	"BRAVE":      "BRAVE_API_KEY",
	"GITHUB":     "GITHUB_PERSONAL_ACCESS_TOKEN",
	"MINIMAX":    "MINIMAX_API_KEY",
	"OPENROUTER": "OPENROUTER_API_KEY",
}

func detectAPIKeys() map[string]bool {
	result := make(map[string]bool, len(apiKeyEnvVars))
	for shortName, envVar := range apiKeyEnvVars {
		result[shortName] = os.Getenv(envVar) != ""
	}
	return result
}

// ─── system context formatter ─────────────────────────────────────────────────

func buildContext(snap *EnvSnapshot) string {
	var sb strings.Builder
	ts := snap.ProbeTime.UTC().Format("2006-01-02T15:04:05Z")
	sb.WriteString(fmt.Sprintf("\n### GORKBOT ENVIRONMENT (probed %s)\n", ts))

	// Platform
	platform := snap.OS + "/" + snap.Arch
	if snap.IsTermux {
		platform = "Android/Termux · " + snap.Arch
	}
	shell := snap.Shell
	if shell == "" {
		shell = "unknown"
	} else {
		shell = filepath.Base(shell)
	}
	sb.WriteString(fmt.Sprintf("Platform  : %s · shell=%s\n", platform, shell))

	// Privilege
	privStr := fmt.Sprintf("UID=%d", snap.UID)
	if snap.IsRoot {
		privStr += " (root — execute natively, no escalation needed)"
	} else {
		var esc []string
		if snap.CLITools["su"] {
			esc = append(esc, "su")
		}
		if snap.CLITools["sudo"] {
			esc = append(esc, "sudo")
		}
		if len(esc) > 0 {
			privStr += " · escalation available: " + strings.Join(esc, " | ") +
				" — use privileged_execute tool for elevated commands"
		} else {
			privStr += " · no privilege escalation available — commands run as current user only"
		}
	}
	sb.WriteString(fmt.Sprintf("Privilege : %s\n", privStr))

	// Runtimes
	var rts []string
	if snap.Python3.Found {
		rts = append(rts, "Python "+snap.Python3.Version+" ✓")
	} else {
		rts = append(rts, "Python ✗")
	}
	if snap.Node.Found {
		rts = append(rts, "Node "+snap.Node.Version+" ✓")
	} else {
		rts = append(rts, "Node ✗")
	}
	if snap.GoRT.Found {
		rts = append(rts, "Go "+snap.GoRT.Version+" ✓")
	}
	if snap.Java.Found {
		rts = append(rts, "Java "+snap.Java.Version+" ✓")
	}
	sb.WriteString(fmt.Sprintf("Runtimes  : %s\n", strings.Join(rts, " · ")))

	// API keys
	keyOrder := []string{"XAI", "GEMINI", "ANTHROPIC", "OPENAI", "BRAVE", "GITHUB"}
	keyParts := make([]string, 0, len(keyOrder))
	for _, k := range keyOrder {
		v := "✗"
		if snap.APIKeys[k] {
			v = "✓"
		}
		keyParts = append(keyParts, k+"="+v)
	}
	sb.WriteString(fmt.Sprintf("API keys  : %s\n", strings.Join(keyParts, "  ")))

	// Python packages
	pkgOrder := []string{"mcp", "fastmcp", "google-genai", "requests", "anthropic", "openai", "httpx"}
	pkgParts := make([]string, 0, len(pkgOrder))
	for _, k := range pkgOrder {
		v := "✗"
		if snap.PythonPkgs[k] {
			v = "✓"
		}
		pkgParts = append(pkgParts, k+"="+v)
	}
	sb.WriteString(fmt.Sprintf("Python pkg: %s\n", strings.Join(pkgParts, "  ")))

	// CLI tools (key subset only to keep the block compact)
	cliOrder := []string{"git", "curl", "wget", "nmap", "ffmpeg", "pkg", "npm", "npx", "pip3", "adb", "jq"}
	cliParts := make([]string, 0, len(cliOrder))
	for _, k := range cliOrder {
		v := "✗"
		if snap.CLITools[k] {
			v = "✓"
		}
		cliParts = append(cliParts, k+"="+v)
	}
	sb.WriteString(fmt.Sprintf("CLI tools : %s\n", strings.Join(cliParts, "  ")))

	// MCP servers
	if len(snap.MCPServers) > 0 {
		mcpParts := make([]string, 0, len(snap.MCPServers))
		for _, s := range snap.MCPServers {
			if s.Running {
				mcpParts = append(mcpParts, fmt.Sprintf("%s=running(%d tools)", s.Name, s.ToolCount))
			} else {
				mcpParts = append(mcpParts, s.Name+"=FAILED")
			}
		}
		sb.WriteString(fmt.Sprintf("MCP status: %s\n", strings.Join(mcpParts, " · ")))
	}

	// Constraint guidance
	sb.WriteString("⚠ BEFORE calling any tool: check the block above.\n")
	sb.WriteString("  If a required runtime, CLI binary, or API key is marked ✗,\n")
	sb.WriteString("  tell the user and suggest how to install/configure it\n")
	sb.WriteString("  INSTEAD of calling the tool and wasting tokens on a predictable failure.\n")

	return sb.String()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// extractVersion extracts a clean version string from a runtime --version output.
func extractVersion(raw string) string {
	if raw == "" {
		return ""
	}
	line := strings.SplitN(raw, "\n", 2)[0]
	for _, prefix := range []string{"Python ", "node ", "go version go", "openjdk version "} {
		if strings.HasPrefix(line, prefix) {
			s := strings.TrimPrefix(line, prefix)
			// take only the first whitespace-delimited token
			if idx := strings.IndexAny(s, " \t"); idx > 0 {
				return s[:idx]
			}
			return s
		}
	}
	// Fallback: last space-separated token on the line.
	parts := strings.Fields(line)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return line
}

func countTrue(m map[string]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}
