package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/router"
)

type ProviderSelector func(reg *registry.ModelRegistry, primaryOverride, consultantOverride string, logger *slog.Logger) (ai.AIProvider, ai.AIProvider, error)

type ProviderSetupOptions struct {
	ConfigDir               string
	Logger                  *slog.Logger
	VerboseThoughts         bool
	PrimaryOverride         string
	ConsultantOverride      string
	PrimaryModelOverride    string
	ConsultantModelOverride string
	AppState                *config.AppStateManager
	SelectProviders         ProviderSelector
}

type ProviderSetup struct {
	KeyStore            *providers.KeyStore
	ProviderManager     *providers.Manager
	Registry            *registry.ModelRegistry
	Router              *router.Router
	Primary             ai.AIProvider
	Consultant          ai.AIProvider
	SystemConfig        *router.SystemConfiguration
	PrimaryModelName    string
	ConsultantModelName string
	RawGeminiKey        string
}

func hasConfiguredProviderKey(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return false
	}
	switch v {
	case "placeholder", "changeme", "your_api_key", "your-api-key", "api_key_here", "replace-me", "replace_with_api_key", "<api-key>":
		return false
	}
	return true
}

func SetupProviders(opts ProviderSetupOptions) (*ProviderSetup, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	keyStore := providers.NewKeyStore(opts.ConfigDir)
	provMgr := providers.NewManager(keyStore, logger)
	provMgr.SetVerboseThoughts(opts.VerboseThoughts)
	providers.SetGlobalProviderManager(provMgr)
	engine.SetProviderManager(provMgr)

	grokKey, _ := keyStore.Get(providers.ProviderXAI)
	geminiKey, _ := keyStore.Get(providers.ProviderGoogle)
	anthropicKey, _ := keyStore.Get(providers.ProviderAnthropic)
	openaiKey, _ := keyStore.Get(providers.ProviderOpenAI)
	minimaxKey, _ := keyStore.Get(providers.ProviderMiniMax)
	openrouterKey, _ := keyStore.Get(providers.ProviderOpenRouter)
	moonshotKey, _ := keyStore.Get(providers.ProviderMoonshot)

	credCtx, credCancel := context.WithTimeout(context.Background(), 3*time.Second)
	resolvedGeminiKey, geminiOAuthToken, geminiAuthMode := providers.ResolveGoogleCredentials(credCtx, opts.ConfigDir, geminiKey, logger)
	credCancel()
	resolvedOpenAIKey, openaiOAuthToken, openaiAuthMode := providers.ResolveOpenAICredentials(opts.ConfigDir, openaiKey, logger)
	resolvedAnthropicKey, anthropicOAuthToken, anthropicAuthMode := providers.ResolveAnthropicCredentials(opts.ConfigDir, anthropicKey, logger)

	keyState := map[string]string{
		"XAI_API_KEY":        grokKey,
		"GEMINI_API_KEY":     geminiKey,
		"ANTHROPIC_API_KEY":  anthropicKey,
		"OPENAI_API_KEY":     openaiKey,
		"MINIMAX_API_KEY":    minimaxKey,
		"OPENROUTER_API_KEY": openrouterKey,
		"MOONSHOT_API_KEY":   moonshotKey,
	}

	var configured []string
	var placeholder []string
	for envName, keyVal := range keyState {
		trimmed := strings.TrimSpace(keyVal)
		if trimmed == "" {
			continue
		}
		if hasConfiguredProviderKey(trimmed) {
			configured = append(configured, envName)
			continue
		}
		placeholder = append(placeholder, envName)
	}
	if geminiAuthMode == "oauth" {
		configured = append(configured, "GOOGLE_OAUTH")
	}
	if openaiAuthMode == "oauth" {
		configured = append(configured, "OPENAI_OAUTH")
	}
	if anthropicAuthMode == "oauth" {
		configured = append(configured, "ANTHROPIC_OAUTH")
	}

	if len(configured) == 0 {
		logger.Warn("No valid AI provider API keys configured. Set at least one provider API key before model calls.")
		logger.Info("Provider key env vars", "supported", strings.Join([]string{
			"XAI_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "MINIMAX_API_KEY", "OPENROUTER_API_KEY", "MOONSHOT_API_KEY",
		}, ", "))
	}
	if len(placeholder) > 0 {
		logger.Warn("Placeholder provider API keys detected; these providers will be treated as unavailable",
			"env_vars", strings.Join(placeholder, ", "))
	}
	if geminiAuthMode == "oauth" {
		logger.Info("Google provider using OAuth sign-in credentials (API key fallback available)")
	}
	if openaiAuthMode == "oauth" {
		logger.Info("OpenAI provider using OAuth/session credentials (API key fallback available)")
	}
	if anthropicAuthMode == "oauth" {
		logger.Info("Anthropic provider using OAuth/session credentials (API key fallback available)")
	}

	reg := registry.NewModelRegistry(logger)
	startupCtx := context.Background()

	if hasConfiguredProviderKey(grokKey) {
		baseGrok := ai.NewGrokProvider(strings.TrimSpace(grokKey), opts.PrimaryModelOverride)
		if err := reg.RegisterProvider(startupCtx, baseGrok); err != nil {
			logger.Error("Failed to register Grok provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(resolvedGeminiKey) || strings.TrimSpace(geminiOAuthToken) != "" {
		baseGemini := ai.NewGeminiProviderWithAuth(strings.TrimSpace(resolvedGeminiKey), strings.TrimSpace(geminiOAuthToken), opts.ConsultantModelOverride, opts.VerboseThoughts)
		if err := reg.RegisterProvider(startupCtx, baseGemini); err != nil {
			logger.Error("Failed to register Gemini provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(resolvedAnthropicKey) || strings.TrimSpace(anthropicOAuthToken) != "" {
		baseAnthropic := ai.NewAnthropicProviderWithAuth(strings.TrimSpace(resolvedAnthropicKey), strings.TrimSpace(anthropicOAuthToken), "")
		if err := reg.RegisterProvider(startupCtx, baseAnthropic); err != nil {
			logger.Error("Failed to register Anthropic provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(resolvedOpenAIKey) || strings.TrimSpace(openaiOAuthToken) != "" {
		baseOpenAI := ai.NewOpenAIProviderWithAuth(strings.TrimSpace(resolvedOpenAIKey), strings.TrimSpace(openaiOAuthToken), "")
		if err := reg.RegisterProvider(startupCtx, baseOpenAI); err != nil {
			logger.Error("Failed to register OpenAI provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(minimaxKey) {
		baseMiniMax := ai.NewMiniMaxProvider(strings.TrimSpace(minimaxKey), "")
		if err := reg.RegisterProvider(startupCtx, baseMiniMax); err != nil {
			logger.Error("Failed to register MiniMax provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(openrouterKey) {
		baseOpenRouter := ai.NewOpenRouterProvider(strings.TrimSpace(openrouterKey), "")
		if err := reg.RegisterProvider(startupCtx, baseOpenRouter); err != nil {
			logger.Error("Failed to register OpenRouter provider", "error", err)
		}
	}
	if hasConfiguredProviderKey(moonshotKey) {
		baseMoonshot := ai.NewMoonshotProvider(strings.TrimSpace(moonshotKey), "")
		if err := reg.RegisterProvider(startupCtx, baseMoonshot); err != nil {
			logger.Error("Failed to register Moonshot provider", "error", err)
		}
	}

	r := router.NewRouter(reg, logger)
	if opts.AppState != nil {
		if init := opts.AppState.Get(); init.PrimaryProvider != "" {
			r.PrimaryBiasProvider = init.PrimaryProvider
		}
		if init := opts.AppState.Get(); init.SecondaryProvider != "" {
			r.ConsultantBiasProvider = init.SecondaryProvider
		}
	}

	var primary ai.AIProvider
	var consultant ai.AIProvider
	if opts.SelectProviders != nil {
		var selErr error
		primary, consultant, selErr = opts.SelectProviders(reg, opts.PrimaryOverride, opts.ConsultantOverride, logger)
		if selErr != nil {
			return nil, selErr
		}
	}

	primaryModelName := "Primary AI"
	consultantModelName := ""
	if primary != nil {
		primaryModelName = primary.GetMetadata().Name
	}
	if consultant != nil {
		consultantModelName = consultant.GetMetadata().Name
	}

	sysConfig, err := r.SelectSystemModels()
	if err == nil {
		logger.Info("Dynamic Model Selection Successful",
			"primary", sysConfig.PrimaryModel.ID,
			"specialist", sysConfig.SpecialistModel.ID,
			"reason", sysConfig.Reasoning)
	} else {
		logger.Debug("Dynamic Model Selection unavailable, using default models", "error", err)
	}

	return &ProviderSetup{
		KeyStore:            keyStore,
		ProviderManager:     provMgr,
		Registry:            reg,
		Router:              r,
		Primary:             primary,
		Consultant:          consultant,
		SystemConfig:        sysConfig,
		PrimaryModelName:    primaryModelName,
		ConsultantModelName: consultantModelName,
		RawGeminiKey:        geminiKey,
	}, nil
}

func ReadProviderOverridesFromEnv() (primaryOverride, consultantOverride, primaryModelOverride, consultantModelOverride string) {
	primaryOverride = os.Getenv("GORKBOT_PRIMARY")
	consultantOverride = os.Getenv("GORKBOT_CONSULTANT")
	primaryModelOverride = os.Getenv("GORKBOT_PRIMARY_MODEL")
	consultantModelOverride = os.Getenv("GORKBOT_CONSULTANT_MODEL")
	return
}

func ValidateProviderSelectionResult(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("provider selection failed: %w", err)
}
