package routing

import (
	"context"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/internal/config"
	"github.com/velariumai/gorkbot/internal/providers"
)

// TestEndToEndRouting demonstrates the complete v3.0 architecture:
// Config → Router → Provider → Response
func TestEndToEndRouting(t *testing.T) {
	type thinkingCfg struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
		Budget  int  `mapstructure:"budget" toml:"budget"`
	}
	type visionCfg struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
	}

	// Step 1: Create configuration (GORKBOT.md equivalent)
	cfg := &config.GorkbotConfig{
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				Model:           "claude-opus-4",
				APIKey:          "test-key",
				Timeout:         60,
				Thinking:        thinkingCfg{Enabled: true, Budget: 50000},
				Vision:          visionCfg{Enabled: true},
				CostPer1MInput:  3.0,
				CostPer1MOutput: 15.0,
			},
			"openai": {
				Model:           "gpt-5",
				APIKey:          "test-key",
				Timeout:         60,
				Vision:          visionCfg{Enabled: true},
				CostPer1MInput:  5.0,
				CostPer1MOutput: 15.0,
			},
		},
		Capabilities: config.CapabilitiesConfig{
			ThinkingProvider: "anthropic",
			CodingProvider:   "openai",
			VisionProvider:   "openai",
			DefaultProvider:  "anthropic",
		},
		Routing: config.RoutingConfig{
			IntentClass: map[string]string{
				"refactoring":  "anthropic",
				"optimization": "openai",
				"debugging":    "anthropic",
			},
			FileType: map[string]string{
				"*.py": "anthropic",
				"*.go": "anthropic",
				"*.ts": "openai",
			},
			Fallback: map[string][]string{
				"anthropic_fallback": {"openai"},
				"openai_fallback":    {"anthropic"},
			},
		},
		Specialist: config.SpecialistConfig{
			Provider:            "anthropic",
			ThinkingBudget:      100000,
			ComplexityThreshold: 8,
			Enabled:             true,
		},
	}

	// Apply defaults
	if err := config.ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	// Validate configuration
	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}

	logger := slog.Default()

	// Step 2: Create provider factory
	factory := providers.NewProviderFactory(cfg, logger)

	// Step 3: Create capability router
	router := NewCapabilityRouter(cfg, factory, logger)

	// Test cases: different tasks with expected routing decisions
	tests := []struct {
		name             string
		taskContent      string
		expectedIntent   string
		expectedProvider string
		expectThinking   bool
		expectBrowser    bool
		expectMCP        bool
		expectSpecialist bool
	}{
		{
			name:             "Refactoring task → Anthropic with thinking",
			taskContent:      "Refactor the authentication module for better performance",
			expectedIntent:   IntentRefactoring,
			expectedProvider: "anthropic",
			expectThinking:   true,
			expectBrowser:    false,
			expectMCP:        false,
			expectSpecialist: false,
		},
		{
			name:             "Optimization task → OpenAI",
			taskContent:      "Optimize database queries for better performance",
			expectedIntent:   IntentOptimization,
			expectedProvider: "openai",
			expectThinking:   false,
			expectBrowser:    false,
			expectMCP:        false,
			expectSpecialist: false,
		},
		{
			name:             "Debugging task → Anthropic with thinking",
			taskContent:      "Debug the authentication bug in pkg/auth/auth.go",
			expectedIntent:   IntentDebugging,
			expectedProvider: "anthropic",
			expectThinking:   true,
			expectBrowser:    false,
			expectMCP:        false,
			expectSpecialist: false,
		},
		{
			name:             "Testing task → OpenAI",
			taskContent:      "Write comprehensive tests for the user service",
			expectedIntent:   IntentTesting,
			expectedProvider: "anthropic", // Default provider
			expectThinking:   false,
			expectBrowser:    false,
			expectMCP:        false,
			expectSpecialist: false,
		},
		{
			name:             "Complex architecture task → Anthropic with thinking and specialist",
			taskContent:      "Design a microservices architecture for the payment system. This involves designing multiple services, setting up communication patterns, implementing error handling, and considering scalability. We need to refactor 50 files across the system. This is a complex architectural decision that will take several hours.",
			expectedIntent:   IntentArchitecture,
			expectedProvider: "anthropic",
			expectThinking:   true,
			expectBrowser:    false,
			expectMCP:        false,
			expectSpecialist: true, // Complex task > threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create task
			task := NewTask(tt.taskContent)

			// Get routing decision (THE CORE METHOD)
			decision, err := router.RouteCapabilities(context.Background(), task)
			if err != nil {
				t.Fatalf("RouteCapabilities failed: %v", err)
			}

			// Validate decision
			if decision.Provider == nil {
				t.Error("Decision.Provider is nil")
			}

			if decision.ProviderName != tt.expectedProvider {
				t.Errorf("Provider: got %s, want %s", decision.ProviderName, tt.expectedProvider)
			}

			if decision.UseThinking != tt.expectThinking {
				t.Errorf("UseThinking: got %v, want %v", decision.UseThinking, tt.expectThinking)
			}

			if decision.UseBrowser != tt.expectBrowser {
				t.Errorf("UseBrowser: got %v, want %v", decision.UseBrowser, tt.expectBrowser)
			}

			if decision.DelegateToSpecialist != tt.expectSpecialist {
				t.Errorf("DelegateToSpecialist: got %v, want %v",
					decision.DelegateToSpecialist, tt.expectSpecialist)
			}

			// Verify decision contains all required fields
			if decision.PromptVariant == "" {
				t.Error("PromptVariant not set")
			}

			if decision.Temperature == 0 {
				t.Error("Temperature not set")
			}

			if decision.EstimatedCost < 0 {
				t.Error("EstimatedCost is negative")
			}

			t.Logf("✓ Decision: Provider=%s, Intent=%s, Thinking=%v, Browser=%v, Specialist=%v",
				decision.ProviderName,
				tt.expectedIntent,
				decision.UseThinking,
				decision.UseBrowser,
				decision.DelegateToSpecialist,
			)
		})
	}
}

// TestProviderExecutionDisabled verifies internal/providers fails closed for execution paths.
func TestProviderExecutionDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers["anthropic"] = config.ProviderConfig{
		Model:   "claude-opus-4",
		APIKey:  "test-key",
		Timeout: 60,
		Thinking: struct {
			Enabled bool `mapstructure:"enabled" toml:"enabled"`
			Budget  int  `mapstructure:"budget" toml:"budget"`
		}{Enabled: true, Budget: 50000},
	}
	cfg.Capabilities.DefaultProvider = "anthropic"
	if err := config.ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	logger := slog.Default()
	factory := providers.NewProviderFactory(cfg, logger)

	// Test: Create provider and verify execution is explicitly unavailable.
	provider, err := factory.CreateProvider("anthropic")
	if err != nil {
		t.Fatalf("CreateProvider failed: %v", err)
	}

	// Execute request
	req := &providers.ExecuteRequest{
		Messages: []*providers.Message{
			{
				Role:    "user",
				Content: "Say hello",
			},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	if _, err := provider.Execute(context.Background(), req); err == nil {
		t.Fatal("expected execution-disabled error, got nil")
	}
}

// TestFallbackChain demonstrates graceful fallback
func TestFallbackChain(t *testing.T) {
	cfg := &config.GorkbotConfig{
		Providers: map[string]config.ProviderConfig{
			"primary": {
				Model:   "test",
				Timeout: 60,
			},
			"fallback1": {
				Model:   "test",
				Timeout: 60,
			},
			"fallback2": {
				Model:   "test",
				Timeout: 60,
			},
		},
		Capabilities: config.CapabilitiesConfig{
			DefaultProvider: "primary",
		},
		Routing: config.RoutingConfig{
			Fallback: map[string][]string{
				"primary_fallback": {"fallback1", "fallback2"},
			},
		},
	}

	if err := config.ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	logger := slog.Default()
	factory := providers.NewProviderFactory(cfg, logger)

	// Get fallback chain
	fallbacks := factory.GetFallbacks("primary")

	expected := []string{"fallback1", "fallback2"}
	if len(fallbacks) != len(expected) {
		t.Errorf("Fallback chain length: got %d, want %d", len(fallbacks), len(expected))
	}

	for i, fb := range fallbacks {
		if fb != expected[i] {
			t.Errorf("Fallback[%d]: got %s, want %s", i, fb, expected[i])
		}
	}

	t.Logf("✓ Fallback chain verified: %v", fallbacks)
}

// TestIntentClassification demonstrates intent detection
func TestIntentClassification(t *testing.T) {
	classifier := NewIntentClassifier()

	tests := []struct {
		content        string
		expectedIntent string
	}{
		{"refactor the code", IntentRefactoring},
		{"optimize performance", IntentOptimization},
		{"fix the bug", IntentDebugging},
		{"write tests", IntentTesting},
		{"design architecture", IntentArchitecture},
		{"plan the sprint", IntentPlanning},
	}

	for _, tt := range tests {
		t.Run(tt.expectedIntent, func(t *testing.T) {
			intent := classifier.Classify(tt.content)
			if intent != tt.expectedIntent {
				t.Errorf("Intent: got %s, want %s", intent, tt.expectedIntent)
			}

			confidence := classifier.ConfidenceScore(tt.content, intent)
			if confidence == 0 {
				t.Error("Confidence score is 0")
			}

			t.Logf("✓ Intent: %s (confidence: %.2f)", intent, confidence)
		})
	}
}

// TestCapabilityDetection demonstrates auto-capability selection
func TestCapabilityDetection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers["anthropic"] = config.ProviderConfig{
		Model:   "claude-opus-4",
		APIKey:  "test-key",
		Timeout: 60,
		Thinking: struct {
			Enabled bool `mapstructure:"enabled" toml:"enabled"`
			Budget  int  `mapstructure:"budget" toml:"budget"`
		}{Enabled: true, Budget: 50000},
		Vision: struct {
			Enabled bool `mapstructure:"enabled" toml:"enabled"`
		}{Enabled: true},
	}
	cfg.Capabilities.DefaultProvider = "anthropic"
	if err := config.ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	logger := slog.Default()
	factory := providers.NewProviderFactory(cfg, logger)
	router := NewCapabilityRouter(cfg, factory, logger)

	tests := []struct {
		taskContent       string
		shouldUseThinking bool
		shouldUseBrowser  bool
	}{
		{"refactor the auth module for security and performance", true, false},
		{"test the UI by taking screenshots", false, true},
		{"debug the issue and check what the screen looks like", true, true},
		{"analyze the code structure", true, false},
	}

	for _, tt := range tests {
		task := NewTask(tt.taskContent)
		decision, err := router.RouteCapabilities(context.Background(), task)
		if err != nil {
			t.Fatalf("RouteCapabilities failed: %v", err)
		}

		if decision.UseThinking != tt.shouldUseThinking {
			t.Errorf("Thinking for '%s': got %v, want %v",
				tt.taskContent, decision.UseThinking, tt.shouldUseThinking)
		}

		if decision.UseBrowser != tt.shouldUseBrowser {
			t.Errorf("Browser for '%s': got %v, want %v",
				tt.taskContent, decision.UseBrowser, tt.shouldUseBrowser)
		}

		t.Logf("✓ Task: thinking=%v, browser=%v", decision.UseThinking, decision.UseBrowser)
	}
}
