package providers

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/auth"
)

// chooseCredentialMode selects OAuth-vs-key mode with fallback semantics.
// If providerPrefEnv is set, it overrides global GORKBOT_PREFER_OAUTH.
func chooseCredentialMode(apiKey, oauthToken, providerPrefEnv string) (resolvedKey, resolvedOAuth, mode string) {
	key := strings.TrimSpace(apiKey)
	oauth := strings.TrimSpace(oauthToken)
	preferOAuth := preferOAuthEnabled(providerPrefEnv)

	if preferOAuth {
		switch {
		case oauth != "":
			return "", oauth, "oauth"
		case key != "":
			return key, "", "api_key"
		default:
			return "", "", "none"
		}
	}

	switch {
	case key != "":
		return key, "", "api_key"
	case oauth != "":
		return "", oauth, "oauth"
	default:
		return "", "", "none"
	}
}

func preferOAuthEnabled(providerPrefEnv string) bool {
	if strings.TrimSpace(providerPrefEnv) != "" {
		if v := strings.TrimSpace(os.Getenv(providerPrefEnv)); v != "" {
			return v != "0"
		}
	}
	return strings.TrimSpace(os.Getenv("GORKBOT_PREFER_OAUTH")) != "0"
}

// ResolveGoogleCredentials returns effective Google auth material with hybrid
// semantics: OAuth sign-in when available, API key fallback otherwise.
func ResolveGoogleCredentials(ctx context.Context, configDir, apiKey string, logger *slog.Logger) (resolvedKey, resolvedOAuth, mode string) {
	var oauthToken string
	if configDir != "" {
		client, err := auth.NewGoogleClient(configDir, auth.NotebookLMScopes(), logger)
		if err == nil {
			// Try lightweight non-interactive token validation/refresh path.
			timeoutCtx := ctx
			if timeoutCtx == nil {
				var cancel context.CancelFunc
				timeoutCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
			}
			if res, err := client.EnsureToken(timeoutCtx); err == nil && res.Token != nil && strings.TrimSpace(res.Token.AccessToken) != "" {
				oauthToken = strings.TrimSpace(res.Token.AccessToken)
			} else {
				oauthToken = strings.TrimSpace(client.GetAccessToken())
			}
		}
	}

	return chooseCredentialMode(apiKey, oauthToken, "GORKBOT_PREFER_OAUTH_GOOGLE")
}

// ResolveOpenAICredentials returns effective OpenAI auth material with hybrid
// semantics: OAuth/session token when available, API key fallback otherwise.
func ResolveOpenAICredentials(configDir, apiKey string, logger *slog.Logger) (resolvedKey, resolvedOAuth, mode string) {
	oauthToken := auth.ResolveOpenAIAccessToken(configDir, logger)
	return chooseCredentialMode(apiKey, oauthToken, "GORKBOT_PREFER_OAUTH_OPENAI")
}

// ResolveAnthropicCredentials returns effective Anthropic auth material with hybrid
// semantics: OAuth/session token when available, API key fallback otherwise.
func ResolveAnthropicCredentials(configDir, apiKey string, logger *slog.Logger) (resolvedKey, resolvedOAuth, mode string) {
	oauthToken := auth.ResolveAnthropicAccessToken(configDir, logger)
	return chooseCredentialMode(apiKey, oauthToken, "GORKBOT_PREFER_OAUTH_ANTHROPIC")
}
