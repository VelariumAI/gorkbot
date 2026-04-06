package auth

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOpenAIAccessTokenFromEnv(t *testing.T) {
	t.Setenv("OPENAI_ACCESS_TOKEN", "env-openai-token")
	got := ResolveOpenAIAccessToken("", slog.Default())
	if got != "env-openai-token" {
		t.Fatalf("expected env token, got %q", got)
	}
}

func TestResolveOpenAIAccessTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "openai_auth.json")
	if err := os.WriteFile(file, []byte(`{"credentials":{"access_token":"file-openai-token"}}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got := ResolveOpenAIAccessToken(dir, slog.Default())
	if got != "file-openai-token" {
		t.Fatalf("expected file token, got %q", got)
	}
}

func TestResolveOpenAIAccessTokenFromRawTokenFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "openai_oauth_token.txt")
	if err := os.WriteFile(file, []byte("raw-openai-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got := ResolveOpenAIAccessToken(dir, slog.Default())
	if got != "raw-openai-token" {
		t.Fatalf("expected raw file token, got %q", got)
	}
}
