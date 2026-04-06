package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/security"
	"github.com/velariumai/gorkbot/pkg/tools"
)

func TestHasConfiguredProviderKey(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"placeholder", false},
		{"CHANGEme", false},
		{"your_api_key", false},
		{"replace-me", false},
		{"<api-key>", false},
		{"sk-live-123", true},
		{"  real-key  ", true},
	}
	for _, tt := range tests {
		if got := hasConfiguredProviderKey(tt.in); got != tt.want {
			t.Fatalf("hasConfiguredProviderKey(%q)=%v want %v", tt.in, got, tt.want)
		}
	}
}

func TestLoadEnvPrecedence(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	projectEnv := "XAI_API_KEY=project-value\nONLY_PROJECT=project-only\n"
	configEnv := "XAI_API_KEY=config-value\nONLY_CONFIG=config-only\n"
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(projectEnv), 0o600); err != nil {
		t.Fatalf("write project .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte(configEnv), 0o600); err != nil {
		t.Fatalf("write config .env: %v", err)
	}

	t.Setenv("XAI_API_KEY", "pre-existing")
	loadEnv(configDir)
	if got := os.Getenv("XAI_API_KEY"); got != "pre-existing" {
		t.Fatalf("expected existing env to win, got %q", got)
	}

	t.Setenv("XAI_API_KEY", "")
	t.Setenv("ONLY_PROJECT", "")
	t.Setenv("ONLY_CONFIG", "")
	loadEnv(configDir)

	if got := os.Getenv("XAI_API_KEY"); got != "project-value" {
		t.Fatalf("expected project .env precedence over config .env, got %q", got)
	}
	if got := os.Getenv("ONLY_PROJECT"); got != "project-only" {
		t.Fatalf("expected ONLY_PROJECT from project .env, got %q", got)
	}
	if got := os.Getenv("ONLY_CONFIG"); got != "config-only" {
		t.Fatalf("expected ONLY_CONFIG from config .env, got %q", got)
	}
}

func TestHelperFunctions(t *testing.T) {
	models := []registry.ModelDefinition{
		{ID: "m1", Name: "Model 1", Provider: "xai", Capabilities: registry.CapabilitySet{SupportsThinking: true}},
		{ID: "m2", Name: "Model 2", Provider: "google", Capabilities: registry.CapabilitySet{SupportsThinking: false}},
	}
	info := buildCommandModelInfo(models)
	if len(info) != 2 || info[0].ID != "m1" || !info[0].Thinking || info[1].Thinking {
		t.Fatalf("unexpected command model info: %#v", info)
	}

	got := removeFromSlice([]string{"a", "b", "a", "c"}, "a")
	if strings.Join(got, ",") != "b,c" {
		t.Fatalf("unexpected slice after remove: %#v", got)
	}

	if isContextOverflow(nil) {
		t.Fatalf("nil error should not be context overflow")
	}
	if !isContextOverflow(errors.New("maximum context length exceeded")) {
		t.Fatalf("expected context overflow match")
	}
	if isContextOverflow(errors.New("other failure")) {
		t.Fatalf("unexpected context overflow match")
	}
}

func TestOutputHelpersAndToolFilters(t *testing.T) {
	// Capture stdout for outputErrorJSON and printHelp.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	outputErrorJSON("boom")
	printHelp()
	_ = w.Close()
	os.Stdout = oldStdout
	b, _ := io.ReadAll(r)
	out := string(b)
	if !strings.Contains(out, "\"error\": \"boom\"") {
		t.Fatalf("expected json error output, got: %s", out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected help output")
	}

	// Verify allow/deny filters write permissions.
	tmp := t.TempDir()
	pm, err := tools.NewPermissionManager(tmp)
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg := tools.NewRegistry(pm)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	applyToolFilters(reg, "allow_tool", "deny_tool", logger)
	if got := pm.GetPermission("allow_tool"); got != tools.PermissionAlways {
		t.Fatalf("expected allow_tool always, got %q", got)
	}
	if got := pm.GetPermission("deny_tool"); got != tools.PermissionNever {
		t.Fatalf("expected deny_tool never, got %q", got)
	}

	// Nil permission manager branch should no-op without panic.
	regNil := tools.NewRegistry(nil)
	applyToolFilters(regNil, "x", "y", logger)
}

func TestApplyToolFiltersPermissionWriteErrors(t *testing.T) {
	tmp := t.TempDir()
	pm, err := tools.NewPermissionManager(tmp)
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg := tools.NewRegistry(pm)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := os.Chmod(tmp, 0o500); err != nil {
		t.Fatalf("chmod readonly: %v", err)
	}
	defer func() { _ = os.Chmod(tmp, 0o700) }()

	applyToolFilters(reg, "allow_fail", "deny_fail", logger)
}

func TestLoadEnvFileEncryptedAndMalformed(t *testing.T) {
	tmp := t.TempDir()
	km, err := security.NewKeyManager(tmp)
	if err != nil {
		t.Fatalf("key manager: %v", err)
	}
	enc, err := km.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	envPath := filepath.Join(tmp, "test.env")
	data := strings.Join([]string{
		"# comment",
		"MALFORMED_LINE",
		"PLAIN_KEY=plain-value",
		"ENC_KEY=ENC_" + enc,
		"BAD_ENC=ENC_not-valid",
		"",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("PLAIN_KEY", "")
	t.Setenv("ENC_KEY", "")
	t.Setenv("BAD_ENC", "")
	loadEnvFile(envPath, km)

	if got := os.Getenv("PLAIN_KEY"); got != "plain-value" {
		t.Fatalf("expected plain value, got %q", got)
	}
	if got := os.Getenv("ENC_KEY"); got != "secret-value" {
		t.Fatalf("expected decrypted value, got %q", got)
	}
	// Failed decrypt keeps original ENC_ payload.
	if got := os.Getenv("BAD_ENC"); got != "ENC_not-valid" {
		t.Fatalf("expected raw encrypted payload fallback, got %q", got)
	}
}

func TestHandleStatusSmoke(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-verylong-key")
	t.Setenv("GEMINI_API_KEY", "gem-verylong-key")
	t.Setenv("OPENAI_API_KEY", "opn-verylong-key")
	t.Setenv("OPENAI_ACCESS_TOKEN", "tok-verylong-value")
	t.Setenv("ANTHROPIC_API_KEY", "ant-verylong-key")

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	handleStatus()
	_ = w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "Gorkbot Status") || !strings.Contains(s, "Primary API Key:") {
		t.Fatalf("unexpected status output: %s", s)
	}
}

func TestHandlePendingRebuildBranches(t *testing.T) {
	pm, err := tools.NewPermissionManager(t.TempDir())
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg := tools.NewRegistry(pm)

	// No pending branch should emit nothing.
	oldStderr := os.Stderr
	r0, w0, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w0
	handlePendingRebuild(reg, t.TempDir())
	_ = w0.Close()
	os.Stderr = oldStderr
	b0, _ := io.ReadAll(r0)
	if len(strings.TrimSpace(string(b0))) != 0 {
		t.Fatalf("expected no output when no pending rebuild, got %q", string(b0))
	}

	// Pending + auto rebuild where go is not on PATH.
	reg.MarkPendingRebuild("my_tool")
	t.Setenv("GORKBOT_AUTO_REBUILD", "1")
	t.Setenv("PATH", "")

	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w1
	handlePendingRebuild(reg, t.TempDir())
	_ = w1.Close()
	os.Stderr = oldStderr
	b1, _ := io.ReadAll(r1)
	out := string(b1)
	if !strings.Contains(out, "PENDING REBUILD") || !strings.Contains(out, "my_tool") {
		t.Fatalf("expected pending rebuild output, got %q", out)
	}
	if !strings.Contains(out, "Auto-rebuild failed") {
		t.Fatalf("expected auto-rebuild failure message, got %q", out)
	}
}

func TestHandlePendingRebuildAutoRebuildSuccessAndFailure(t *testing.T) {
	makeFakeGo := func(dir string, exitCode int) string {
		p := filepath.Join(dir, "go")
		content := "#!/bin/sh\nexit " + strconv.Itoa(exitCode) + "\n"
		if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
			t.Fatalf("write fake go: %v", err)
		}
		return p
	}

	// Success branch clears pending rebuild list.
	pm, err := tools.NewPermissionManager(t.TempDir())
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg := tools.NewRegistry(pm)
	reg.SetConfigDir(t.TempDir())
	reg.MarkPendingRebuild("tool_success")

	successDir := t.TempDir()
	_ = makeFakeGo(successDir, 0)
	t.Setenv("PATH", successDir)
	t.Setenv("GORKBOT_AUTO_REBUILD", "1")

	oldStderr := os.Stderr
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w1
	handlePendingRebuild(reg, t.TempDir())
	_ = w1.Close()
	os.Stderr = oldStderr
	out1, _ := io.ReadAll(r1)
	if !strings.Contains(string(out1), "Rebuild successful") {
		t.Fatalf("expected rebuild success output, got %q", string(out1))
	}
	if pending := reg.GetPendingRebuild(); len(pending) != 0 {
		t.Fatalf("expected pending rebuild list cleared, got %#v", pending)
	}

	// Failure branch keeps pending list intact.
	pm2, err := tools.NewPermissionManager(t.TempDir())
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}
	reg2 := tools.NewRegistry(pm2)
	reg2.SetConfigDir(t.TempDir())
	reg2.MarkPendingRebuild("tool_failure")

	failDir := t.TempDir()
	_ = makeFakeGo(failDir, 1)
	t.Setenv("PATH", failDir)

	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w2
	handlePendingRebuild(reg2, t.TempDir())
	_ = w2.Close()
	os.Stderr = oldStderr
	out2, _ := io.ReadAll(r2)
	if !strings.Contains(string(out2), "Auto-rebuild failed") {
		t.Fatalf("expected rebuild failure output, got %q", string(out2))
	}
	if pending := reg2.GetPendingRebuild(); len(pending) == 0 {
		t.Fatalf("expected pending rebuild list to remain after failure")
	}
}

func TestHandleSetupWritesEncryptedEnv(t *testing.T) {
	tmp := t.TempDir()

	// Script stdin answers for setup prompts.
	oldStdin := os.Stdin
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	_, _ = inW.Write([]byte("xai-test-key\nAIza-test-gemini\n"))
	_ = inW.Close()
	os.Stdin = inR
	defer func() { os.Stdin = oldStdin }()

	// Capture stdout to keep test output clean and assert success path.
	oldStdout := os.Stdout
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	os.Stdout = outW
	handleSetup(tmp)
	_ = outW.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(outR)
	if !strings.Contains(string(out), "Configuration saved") {
		t.Fatalf("expected setup success output, got %q", string(out))
	}

	envPath := filepath.Join(tmp, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("expected .env to be written: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "XAI_API_KEY=ENC_") || !strings.Contains(s, "GEMINI_API_KEY=ENC_") {
		t.Fatalf("expected encrypted keys in env file, got %q", s)
	}
}

func TestHandleSetupWithEmptyKeysStillWritesEncryptedEntries(t *testing.T) {
	tmp := t.TempDir()

	oldStdin := os.Stdin
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	_, _ = inW.Write([]byte("\n\n"))
	_ = inW.Close()
	os.Stdin = inR
	defer func() { os.Stdin = oldStdin }()

	oldStdout := os.Stdout
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	os.Stdout = outW
	handleSetup(tmp)
	_ = outW.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(outR)
	if !strings.Contains(string(out), "Configuration saved") {
		t.Fatalf("expected setup success output, got %q", string(out))
	}

	data, err := os.ReadFile(filepath.Join(tmp, ".env"))
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "XAI_API_KEY=ENC_") || !strings.Contains(s, "GEMINI_API_KEY=ENC_") {
		t.Fatalf("expected encrypted entries for empty keys, got %q", s)
	}
}

func TestHandleSetupMkdirFailureBranch(t *testing.T) {
	base := t.TempDir()
	badPath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed marker file: %v", err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	handleSetup(badPath)
	_ = w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "Error creating config directory") {
		t.Fatalf("expected mkdir failure output, got %q", string(out))
	}
}

func TestHandleStatusNotConfiguredOutput(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_ACCESS_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	handleStatus()
	_ = w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "Primary API Key: ❌ Not set") || !strings.Contains(s, "OpenAI API Key:  ❌ Not set") {
		t.Fatalf("expected not-configured status output, got %q", s)
	}
}

func TestRunOneShotTaskInvalidInputExitsText(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestRunOneShotTaskInvalidInputExitsText_Helper")
	cmd.Env = append(os.Environ(), "GO_WANT_ONESHOT_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit from helper")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got err=%v", err)
	}
	if !strings.Contains(string(out), "Input validation error") {
		t.Fatalf("expected validation error output, got %q", string(out))
	}
}

func TestRunOneShotTaskInvalidInputExitsText_Helper(t *testing.T) {
	if os.Getenv("GO_WANT_ONESHOT_HELPER") != "1" {
		return
	}
	runOneShotTask(context.Background(), nil, "bad\x01prompt", "", "text", nil)
}

func TestRunOneShotTaskInvalidInputExitsJSON(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestRunOneShotTaskInvalidInputExitsJSON_Helper")
	cmd.Env = append(os.Environ(), "GO_WANT_ONESHOT_JSON_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit from helper")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got err=%v", err)
	}
	if !strings.Contains(string(out), "\"error\"") {
		t.Fatalf("expected json error output, got %q", string(out))
	}
}

func TestRunOneShotTaskInvalidInputExitsJSON_Helper(t *testing.T) {
	if os.Getenv("GO_WANT_ONESHOT_JSON_HELPER") != "1" {
		return
	}
	runOneShotTask(context.Background(), nil, "bad\x01prompt", "", "json", nil)
}

func minimalOrchestratorForOneShot() *engine.Orchestrator {
	return &engine.Orchestrator{
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		ConversationHistory: ai.NewConversationHistory(),
	}
}

func TestRunOneShotTaskErrorTextMode(t *testing.T) {
	orch := minimalOrchestratorForOneShot()

	oldStderr := os.Stderr
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	os.Stderr = errW
	runOneShotTask(context.Background(), orch, "hello world", "", "text", nil)
	_ = errW.Close()
	os.Stderr = oldStderr

	stderrOut, _ := io.ReadAll(errR)
	if !strings.Contains(string(stderrOut), "no primary provider available") {
		t.Fatalf("expected no-primary error output, got %q", string(stderrOut))
	}
}

func TestRunOneShotTaskErrorJSONFileOutput(t *testing.T) {
	orch := minimalOrchestratorForOneShot()
	outFile := filepath.Join(t.TempDir(), "oneshot.json")

	runOneShotTask(context.Background(), orch, "hello world", outFile, "json", nil)

	b, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal output json: %v", err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("expected error key in output payload, got %s", string(b))
	}
}

func TestRunOneShotTaskErrorJSONFileWriteFailure(t *testing.T) {
	orch := minimalOrchestratorForOneShot()
	outFile := filepath.Join(t.TempDir(), "missing", "oneshot.json")

	oldStderr := os.Stderr
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	os.Stderr = errW
	runOneShotTask(context.Background(), orch, "hello world", outFile, "json", nil)
	_ = errW.Close()
	os.Stderr = oldStderr

	stderrOut, _ := io.ReadAll(errR)
	if !strings.Contains(string(stderrOut), "Error writing output file") {
		t.Fatalf("expected output file write error, got %q", string(stderrOut))
	}
}

func TestHandleSetupWriteFailureExits(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleSetupWriteFailureExits_Helper")
	cmd.Env = append(os.Environ(), "GO_WANT_SETUP_WRITEFAIL_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit from helper")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got err=%v", err)
	}
	if !strings.Contains(string(out), "Error saving .env file") {
		t.Fatalf("expected env write failure output, got %q", string(out))
	}
}

func TestHandleSetupWriteFailureExits_Helper(t *testing.T) {
	if os.Getenv("GO_WANT_SETUP_WRITEFAIL_HELPER") != "1" {
		return
	}
	cfgDir := filepath.Join(os.TempDir(), "gorkweb-setup-writefail-test")
	_ = os.RemoveAll(cfgDir)
	_ = os.MkdirAll(cfgDir, 0o700)
	_ = os.Chmod(cfgDir, 0o500)
	defer func() {
		_ = os.Chmod(cfgDir, 0o700)
		_ = os.RemoveAll(cfgDir)
	}()

	oldStdin := os.Stdin
	inR, inW, _ := os.Pipe()
	_, _ = inW.Write([]byte("\n\n"))
	_ = inW.Close()
	os.Stdin = inR
	defer func() { os.Stdin = oldStdin }()

	handleSetup(cfgDir)
}
