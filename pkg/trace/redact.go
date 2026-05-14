package trace

import "strings"

var sensitiveNeedles = []string{
	"token",
	"secret",
	"password",
	"passwd",
	"authorization",
	"cookie",
	"api_key",
	"bearer",
	"credential",
	"private_key",
	"access_key",
	"refresh_token",
	"session",
}

func IsSensitiveKey(key string) bool {
	norm := strings.ToLower(strings.TrimSpace(key))
	norm = strings.ReplaceAll(norm, "-", "_")
	for _, needle := range sensitiveNeedles {
		if norm == needle || strings.Contains(norm, needle) {
			return true
		}
	}
	return false
}

func RedactString(raw string, maxLen int) string {
	trimmed := strings.TrimSpace(raw)
	if maxLen <= 0 {
		return ""
	}
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen]
}
