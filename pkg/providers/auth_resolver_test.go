package providers

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/security"
)

func TestChooseCredentialMode(t *testing.T) {
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := chooseCredentialMode("api-key", "oauth-token", "GORKBOT_PREFER_OAUTH_OPENAI")
	if mode != "oauth" || oauth != "oauth-token" || key != "" {
		t.Fatalf("expected oauth-preferred selection, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}

	key, oauth, mode = chooseCredentialMode("api-key", "", "GORKBOT_PREFER_OAUTH_OPENAI")
	if mode != "api_key" || key != "api-key" || oauth != "" {
		t.Fatalf("expected api-key fallback, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}

	t.Setenv("GORKBOT_PREFER_OAUTH", "0")
	key, oauth, mode = chooseCredentialMode("api-key", "oauth-token", "GORKBOT_PREFER_OAUTH_OPENAI")
	if mode != "api_key" || key != "api-key" || oauth != "" {
		t.Fatalf("expected api-key preferred selection, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestChooseCredentialModeProviderOverride(t *testing.T) {
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	t.Setenv("GORKBOT_PREFER_OAUTH_OPENAI", "0")
	key, oauth, mode := chooseCredentialMode("api-key", "oauth-token", "GORKBOT_PREFER_OAUTH_OPENAI")
	if mode != "api_key" || key != "api-key" || oauth != "" {
		t.Fatalf("expected provider-specific api-key preference, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveOpenAICredentialsFromEnvToken(t *testing.T) {
	t.Setenv("OPENAI_ACCESS_TOKEN", "openai-session")
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveOpenAICredentials("", "api-key", nil)
	if key != "" || oauth != "openai-session" || mode != "oauth" {
		t.Fatalf("expected oauth mode from env token, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveAnthropicCredentialsFromEnvToken(t *testing.T) {
	t.Setenv("ANTHROPIC_ACCESS_TOKEN", "anthropic-session")
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveAnthropicCredentials("", "api-key", nil)
	if key != "" || oauth != "anthropic-session" || mode != "oauth" {
		t.Fatalf("expected oauth mode from env token, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveOpenAICredentialsFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openai_auth.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"file-openai-token"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveOpenAICredentials(dir, "api-key", slog.Default())
	if key != "" || oauth != "file-openai-token" || mode != "oauth" {
		t.Fatalf("expected file oauth selection, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveAnthropicCredentialsFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anthropic_auth.json")
	if err := os.WriteFile(path, []byte(`{"session":{"token":"file-anthropic-token"}}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveAnthropicCredentials(dir, "api-key", slog.Default())
	if key != "" || oauth != "file-anthropic-token" || mode != "oauth" {
		t.Fatalf("expected file oauth selection, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveGoogleCredentialsFallbackModes(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveGoogleCredentials(nil, dir, "api-key", slog.Default())
	if key != "api-key" || oauth != "" || mode != "api_key" {
		t.Fatalf("expected api-key fallback when no oauth token, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}

	key, oauth, mode = ResolveGoogleCredentials(nil, dir, "", slog.Default())
	if key != "" || oauth != "" || mode != "none" {
		t.Fatalf("expected none mode without credentials, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}

func TestResolveGoogleCredentialsFromEncryptedTokenFile(t *testing.T) {
	dir := t.TempDir()
	km, err := security.NewKeyManager(dir)
	if err != nil {
		t.Fatalf("key manager: %v", err)
	}
	tokenPayload := map[string]any{
		"access_token":  "google-oauth-token",
		"refresh_token": "refresh",
		"expires_at":    time.Now().Add(10 * time.Minute),
		"token_type":    "Bearer",
		"scope":         "scope",
	}
	b, _ := json.Marshal(tokenPayload)
	enc, err := km.Encrypt(string(b))
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "google_oauth_tokens.enc"), []byte(enc), 0o600); err != nil {
		t.Fatalf("write oauth token file: %v", err)
	}

	t.Setenv("GORKBOT_PREFER_OAUTH", "1")
	key, oauth, mode := ResolveGoogleCredentials(nil, dir, "api-key", slog.Default())
	if key != "" || oauth != "google-oauth-token" || mode != "oauth" {
		t.Fatalf("expected oauth selection from encrypted token file, got key=%q oauth=%q mode=%q", key, oauth, mode)
	}
}
