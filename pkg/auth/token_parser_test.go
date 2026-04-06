package auth

import (
	"path/filepath"
	"testing"
)

func TestFindAccessTokenNestedStructures(t *testing.T) {
	v := map[string]any{
		"outer": []any{
			map[string]any{"noop": "x"},
			map[string]any{"inner": map[string]any{"access_token": "nested-token"}},
		},
	}
	if got := findAccessToken(v); got != "nested-token" {
		t.Fatalf("expected nested token, got %q", got)
	}
}

func TestFindAccessTokenNoToken(t *testing.T) {
	v := map[string]any{"a": []any{map[string]any{"b": "c"}}}
	if got := findAccessToken(v); got != "" {
		t.Fatalf("expected empty token for no-token structure, got %q", got)
	}
}

func TestOpenAITokenCandidatePaths(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-test")
	paths := openAITokenCandidatePaths("/tmp/config-dir")
	if len(paths) < 4 {
		t.Fatalf("expected multiple candidate paths, got %d", len(paths))
	}
	if paths[0] != filepath.Join("/tmp/config-dir", "openai_auth.json") {
		t.Fatalf("unexpected first config path: %q", paths[0])
	}
}

func TestAnthropicTokenCandidatePaths(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-test")
	paths := anthropicTokenCandidatePaths("/tmp/config-dir")
	if len(paths) < 4 {
		t.Fatalf("expected multiple candidate paths, got %d", len(paths))
	}
	if paths[0] != filepath.Join("/tmp/config-dir", "anthropic_auth.json") {
		t.Fatalf("unexpected first config path: %q", paths[0])
	}
}
