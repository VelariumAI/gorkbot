package auth

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ResolveAnthropicAccessToken resolves an Anthropic OAuth/session token.
// Resolution order:
//  1. ANTHROPIC_ACCESS_TOKEN env var
//  2. Known auth files in config and user home directories
func ResolveAnthropicAccessToken(configDir string, logger *slog.Logger) string {
	if tok := strings.TrimSpace(os.Getenv("ANTHROPIC_ACCESS_TOKEN")); tok != "" {
		return tok
	}

	paths := anthropicTokenCandidatePaths(configDir)
	for _, p := range paths {
		tok := readAnthropicTokenFromFile(p)
		if tok == "" {
			continue
		}
		if logger != nil {
			logger.Debug("anthropic oauth token resolved from file", "path", p)
		}
		return tok
	}
	return ""
}

func anthropicTokenCandidatePaths(configDir string) []string {
	var paths []string
	if configDir != "" {
		paths = append(paths,
			filepath.Join(configDir, "anthropic_auth.json"),
			filepath.Join(configDir, "anthropic_oauth_token.json"),
			filepath.Join(configDir, "anthropic_oauth_token.txt"),
		)
	}

	home := os.Getenv("HOME")
	if strings.TrimSpace(home) == "" {
		return paths
	}

	return append(paths,
		filepath.Join(home, ".config", "anthropic", "auth.json"),
		filepath.Join(home, ".anthropic", "auth.json"),
		filepath.Join(home, ".config", "claude", "auth.json"),
		filepath.Join(home, ".claude", "auth.json"),
	)
}

func readAnthropicTokenFromFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		return raw
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	return findAccessToken(v)
}
