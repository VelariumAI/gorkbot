package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/velariumai/gorkbot/pkg/config"
)

// ─── Test Group 1: ParseUseKey ───────────────────────────────────────────────

func TestParseUseKey_Valid(t *testing.T) {
	tests := []struct {
		input     string
		wantNS    string
		wantProv  string
	}{
		{
			input:    "pkg.ai:XAIChatModel",
			wantNS:   "pkg.ai",
			wantProv: "XAIChatModel",
		},
		{
			input:    "pkg.sandbox.local:LocalSandboxProvider",
			wantNS:   "pkg.sandbox.local",
			wantProv: "LocalSandboxProvider",
		},
		{
			input:    "pkg.guardrails:AllowlistProvider",
			wantNS:   "pkg.guardrails",
			wantProv: "AllowlistProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ns, prov, err := ParseUseKey(tt.input)
			if err != nil {
				t.Fatalf("ParseUseKey(%q) failed: %v", tt.input, err)
			}
			if ns != tt.wantNS || prov != tt.wantProv {
				t.Errorf("ParseUseKey(%q) = (%q, %q), want (%q, %q)", tt.input, ns, prov, tt.wantNS, tt.wantProv)
			}
		})
	}
}

func TestParseUseKey_Invalid(t *testing.T) {
	tests := []string{
		"nodollar",
		":",
		":Provider",
		"pkg.ai:",
		"",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, _, err := ParseUseKey(input)
			if err == nil {
				t.Errorf("ParseUseKey(%q) expected error, got nil", input)
			}
		})
	}
}

// ─── Test Group 2: Registry Isolation ────────────────────────────────────────

func TestRegistry_Isolation(t *testing.T) {
	// Each test gets its own registry (not using DefaultRegistry)
	r := NewFactoryRegistry()

	// Import and use a real AI provider for testing
	// For the test, we just need to verify that registration and resolution work
	// We'll create a minimal dummy that satisfies the AIProvider interface
	// Actually, let's use one of the real factories to keep the test simple
	r.RegisterAI("test:XAI", newXAIFactory)

	// Try to resolve it - this should return an error because we don't have a key
	_, err := r.ResolveAI("test:XAI", AIFactoryParams{})
	if err == nil {
		t.Error("expected error due to missing APIKey")
	}

	// Try with valid API key
	result, err := r.ResolveAI("test:XAI", AIFactoryParams{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("ResolveAI failed: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestRegistry_DuplicatePanic(t *testing.T) {
	r := NewFactoryRegistry()
	r.RegisterAI("test:Dup", newXAIFactory)

	// Second registration with same key should panic
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()

	r.RegisterAI("test:Dup", newXAIFactory)
}

func TestRegistry_UnknownKey(t *testing.T) {
	r := NewFactoryRegistry()
	_, err := r.ResolveAI("unknown:Provider", AIFactoryParams{})
	if !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("expected ErrUnknownProvider, got %v", err)
	}
}

// ─── Test Group 3: AI Factory Validation ─────────────────────────────────────

func TestAIFactory_EmptyAPIKey(t *testing.T) {
	factories := map[string]AIProviderFactory{
		"XAI":        newXAIFactory,
		"Anthropic":  newAnthropicFactory,
		"Google":     newGoogleFactory,
		"OpenAI":     newOpenAIFactory,
		"MiniMax":    newMiniMaxFactory,
		"OpenRouter": newOpenRouterFactory,
		"Moonshot":   newMoonshotFactory,
	}

	params := AIFactoryParams{
		APIKey: "", // Empty API key
		Model:  "some-model",
	}

	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			_, err := factory(params)
			if err == nil {
				t.Errorf("%s factory should error on empty APIKey", name)
			}
		})
	}
}

// ─── Test Group 4: Resolver Functions ────────────────────────────────────────

func TestResolveAIFromConfig_Nil(t *testing.T) {
	_, err := ResolveAIFromConfig(nil, false)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestResolveAIFromConfig_EmptyUse(t *testing.T) {
	cfg := &config.ModelConfig{
		Use:    "",
		APIKey: "test-key",
	}
	_, err := ResolveAIFromConfig(cfg, false)
	if err == nil {
		t.Error("expected error for empty Use field")
	}
}

func TestResolveSandboxFromConfig_Disabled(t *testing.T) {
	cfg := &config.SandboxConfig{
		Enabled: false,
	}
	result, err := ResolveSandboxFromConfig(cfg)
	if err != nil || result != nil {
		t.Errorf("expected (nil, nil) for disabled config, got (%v, %v)", result, err)
	}
}

func TestResolveSandboxFromConfig_Nil(t *testing.T) {
	result, err := ResolveSandboxFromConfig(nil)
	if err != nil || result != nil {
		t.Errorf("expected (nil, nil) for nil config, got (%v, %v)", result, err)
	}
}

func TestResolveGuardrailsFromConfig_Disabled(t *testing.T) {
	cfg := &config.GuardrailsConfig{
		Enabled: false,
	}
	result, err := ResolveGuardrailsFromConfig(cfg)
	if err != nil || result != nil {
		t.Errorf("expected (nil, nil) for disabled config, got (%v, %v)", result, err)
	}
}

func TestResolveGuardrailsFromConfig_Nil(t *testing.T) {
	result, err := ResolveGuardrailsFromConfig(nil)
	if err != nil || result != nil {
		t.Errorf("expected (nil, nil) for nil config, got (%v, %v)", result, err)
	}
}

// ─── Test Group 5: Sandbox & Guardrails Providers ──────────────────────────

func TestLocalSandboxProvider(t *testing.T) {
	// Create a temporary directory for the sandbox
	tmpDir, err := os.MkdirTemp("", "sandbox-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create provider with temp directory
	provider, err := newLocalSandboxProvider(map[string]interface{}{
		"work_dir": tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Test WriteFile
	content := []byte("test content")
	testFile := "test.txt"
	err = provider.WriteFile(ctx, testFile, content)
	if err != nil {
		t.Errorf("WriteFile failed: %v", err)
	}

	// Test ReadFile
	read, err := provider.ReadFile(ctx, testFile)
	if err != nil {
		t.Errorf("ReadFile failed: %v", err)
	}
	if string(read) != string(content) {
		t.Errorf("ReadFile returned %q, want %q", string(read), string(content))
	}
}

func TestLocalSandboxProvider_PathEscape(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sandbox-escape-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider, err := newLocalSandboxProvider(map[string]interface{}{
		"work_dir": tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Try to access parent directory (path escape attempt)
	escapePath := filepath.Join(tmpDir, "..", "outside.txt")
	_, err = provider.ReadFile(ctx, escapePath)
	if err == nil {
		t.Error("expected error for path escape attempt")
	}

	// Try to write outside the work directory
	err = provider.WriteFile(ctx, escapePath, []byte("data"))
	if err == nil {
		t.Error("expected error for write outside work directory")
	}
}

func TestAllowlistProvider(t *testing.T) {
	ctx := context.Background()

	// Test 1: Empty allowlist = all permitted
	provider, err := newAllowlistProvider(map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	err = provider.Authorize(ctx, "any_tool", nil)
	if err != nil {
		t.Errorf("expected to authorize any_tool with empty allowlist, got %v", err)
	}

	// Test 2: Tool in blocklist = denied
	provider, err = newAllowlistProvider(map[string]interface{}{
		"blocked_tools": []interface{}{"dangerous_tool"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	err = provider.Authorize(ctx, "dangerous_tool", nil)
	if err == nil {
		t.Error("expected to deny blocked tool")
	}

	// Test 3: Non-empty allowlist = only allowed tools
	provider, err = newAllowlistProvider(map[string]interface{}{
		"allowed_tools": []interface{}{"safe_tool", "another_safe"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	err = provider.Authorize(ctx, "safe_tool", nil)
	if err != nil {
		t.Errorf("expected to authorize safe_tool, got %v", err)
	}

	err = provider.Authorize(ctx, "forbidden_tool", nil)
	if err == nil {
		t.Error("expected to deny tool not in allowlist")
	}

	// Test 4: Blocklist takes priority
	provider, err = newAllowlistProvider(map[string]interface{}{
		"allowed_tools": []interface{}{"blocked_but_allowed"},
		"blocked_tools": []interface{}{"blocked_but_allowed"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	err = provider.Authorize(ctx, "blocked_but_allowed", nil)
	if err == nil {
		t.Error("expected blocklist to take priority")
	}
}
