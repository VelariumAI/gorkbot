package auth

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ResolveOpenAIAccessToken resolves an OpenAI OAuth/session access token.
// Resolution order:
//  1. OPENAI_ACCESS_TOKEN env var
//  2. Known auth files in config and user home directories
func ResolveOpenAIAccessToken(configDir string, logger *slog.Logger) string {
	if tok := strings.TrimSpace(os.Getenv("OPENAI_ACCESS_TOKEN")); tok != "" {
		return tok
	}

	paths := openAITokenCandidatePaths(configDir)
	for _, p := range paths {
		tok := readOpenAITokenFromFile(p)
		if tok == "" {
			continue
		}
		if logger != nil {
			logger.Debug("openai oauth token resolved from file", "path", p)
		}
		return tok
	}
	return ""
}

func openAITokenCandidatePaths(configDir string) []string {
	var paths []string
	if configDir != "" {
		paths = append(paths,
			filepath.Join(configDir, "openai_auth.json"),
			filepath.Join(configDir, "openai_oauth_token.json"),
			filepath.Join(configDir, "openai_oauth_token.txt"),
		)
	}

	home := os.Getenv("HOME")
	if strings.TrimSpace(home) == "" {
		return paths
	}

	return append(paths,
		filepath.Join(home, ".config", "openai", "auth.json"),
		filepath.Join(home, ".openai", "auth.json"),
		filepath.Join(home, ".config", "codex", "auth.json"),
		filepath.Join(home, ".codex", "auth.json"),
	)
}

func readOpenAITokenFromFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return ""
	}

	// Raw-token file support.
	if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		return raw
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	return findAccessToken(v)
}

func findAccessToken(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"access_token", "token", "id_token"} {
			if s, ok := t[k].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		for _, sub := range t {
			if tok := findAccessToken(sub); tok != "" {
				return tok
			}
		}
	case []any:
		for _, sub := range t {
			if tok := findAccessToken(sub); tok != "" {
				return tok
			}
		}
	}
	return ""
}
