package auth

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAnthropicAccessTokenFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_ACCESS_TOKEN", "env-anthropic-token")
	got := ResolveAnthropicAccessToken("", slog.Default())
	if got != "env-anthropic-token" {
		t.Fatalf("expected env token, got %q", got)
	}
}

func TestResolveAnthropicAccessTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "anthropic_auth.json")
	if err := os.WriteFile(file, []byte(`{"session":{"token":"file-anthropic-token"}}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got := ResolveAnthropicAccessToken(dir, slog.Default())
	if got != "file-anthropic-token" {
		t.Fatalf("expected file token, got %q", got)
	}
}

func TestResolveAnthropicAccessTokenFromRawTokenFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "anthropic_oauth_token.txt")
	if err := os.WriteFile(file, []byte("raw-anthropic-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got := ResolveAnthropicAccessToken(dir, slog.Default())
	if got != "raw-anthropic-token" {
		t.Fatalf("expected raw file token, got %q", got)
	}
}
