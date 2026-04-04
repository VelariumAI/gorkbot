package provider

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/pkg/config"
)

// TestIntegration_FullRegistry verifies the complete provider factory system works end-to-end.
func TestIntegration_FullRegistry(t *testing.T) {
	// Initialize the DefaultRegistry (simulating main.go calling RegisterAll)
	// Note: This will panic if providers are already registered (which is OK for tests)
	// In practice, RegisterAll is called once at startup in main.go
	testReg := NewFactoryRegistry()
	RegisterAIProviders(testReg)
	RegisterSandboxProviders(testReg)
	RegisterGuardrailsProviders(testReg)

	// Verify AI providers are registered
	aiKeys := testReg.ListAIKeys()
	if len(aiKeys) != 7 {
		t.Errorf("expected 7 AI providers registered, got %d", len(aiKeys))
	}

	// Test 1: Resolve a valid AI provider
	cfg := &config.ModelConfig{
		Use:    "pkg.ai:XAIChatModel",
		APIKey: "test-key-123",
		Model:  "grok-3",
	}

	provider, err := ResolveAIFromConfigWithRegistry(testReg, cfg, false)
	if err != nil {
		t.Fatalf("ResolveAIFromConfig failed: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	if provider.Name() != "Grok" {
		t.Errorf("expected provider name 'Grok', got %q", provider.Name())
	}

	// Test 2: Sandbox provider integration
	sandboxCfg := &config.SandboxConfig{
		Use:     "pkg.sandbox.local:LocalSandboxProvider",
		Enabled: true,
		CustomFields: map[string]interface{}{
			"work_dir": "/tmp/test",
		},
	}

	sandboxProv, err := ResolveSandboxFromConfigWithRegistry(testReg, sandboxCfg)
	if err != nil {
		t.Fatalf("ResolveSandboxFromConfig failed: %v", err)
	}
	if sandboxProv == nil {
		t.Error("expected non-nil sandbox provider")
	}
	if sandboxProv.Name() != "LocalSandboxProvider" {
		t.Errorf("expected 'LocalSandboxProvider', got %q", sandboxProv.Name())
	}

	// Test 3: Guardrails provider integration
	guardrailsCfg := &config.GuardrailsConfig{
		Use:     "pkg.guardrails:AllowlistProvider",
		Enabled: true,
		CustomFields: map[string]interface{}{
			"allowed_tools": []interface{}{"safe_tool", "another_safe"},
		},
	}

	guardrailsProv, err := ResolveGuardrailsFromConfigWithRegistry(testReg, guardrailsCfg)
	if err != nil {
		t.Fatalf("ResolveGuardrailsFromConfig failed: %v", err)
	}
	if guardrailsProv == nil {
		t.Error("expected non-nil guardrails provider")
	}
	if guardrailsProv.Name() != "AllowlistProvider" {
		t.Errorf("expected 'AllowlistProvider', got %q", guardrailsProv.Name())
	}

	// Test 4: Verify guardrails authorization works
	ctx := context.Background()
	err = guardrailsProv.Authorize(ctx, "safe_tool", nil)
	if err != nil {
		t.Errorf("expected authorization for safe_tool, got %v", err)
	}

	err = guardrailsProv.Authorize(ctx, "forbidden_tool", nil)
	if err == nil {
		t.Error("expected authorization to fail for forbidden_tool")
	}

	// Test 5: Disabled sandbox/guardrails return nil
	disabledSandbox := &config.SandboxConfig{Enabled: false}
	result, err := ResolveSandboxFromConfig(disabledSandbox)
	if err != nil || result != nil {
		t.Errorf("expected (nil, nil) for disabled sandbox, got (%v, %v)", result, err)
	}

	disabledGuardrails := &config.GuardrailsConfig{Enabled: false}
	result2, err := ResolveGuardrailsFromConfig(disabledGuardrails)
	if err != nil || result2 != nil {
		t.Errorf("expected (nil, nil) for disabled guardrails, got (%v, %v)", result2, err)
	}
}

// TestIntegration_AllAIFactories verifies all 7 AI factories work correctly.
func TestIntegration_AllAIFactories(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"XAI", "pkg.ai:XAIChatModel", "Grok"},
		{"Anthropic", "pkg.ai:AnthropicChatModel", "Claude"},
		{"Google", "pkg.ai:GoogleChatModel", "Gemini"},
		{"OpenAI", "pkg.ai:OpenAIChatModel", "OpenAI"},
		{"MiniMax", "pkg.ai:MiniMaxChatModel", "MiniMax"},
		{"OpenRouter", "pkg.ai:OpenRouterChatModel", "OpenRouter"},
		{"Moonshot", "pkg.ai:MoonshotChatModel", "Moonshot"},
	}

	testReg := NewFactoryRegistry()
	RegisterAIProviders(testReg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ModelConfig{
				Use:    tt.key,
				APIKey: "test-key",
				Model:  "test-model",
			}

			prov, err := ResolveAIFromConfigWithRegistry(testReg, cfg, false)
			if err != nil {
				t.Fatalf("ResolveAIFromConfig failed: %v", err)
			}
			if prov == nil {
				t.Fatal("expected non-nil provider")
			}
			if prov.Name() != tt.want {
				t.Errorf("expected name %q, got %q", tt.want, prov.Name())
			}
		})
	}
}
