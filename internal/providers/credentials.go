package providers

import "strings"

// hasConfiguredAPIKey returns true only for non-placeholder API key values.
func hasConfiguredAPIKey(v string) bool {
	key := strings.TrimSpace(v)
	if key == "" {
		return false
	}

	switch strings.ToLower(key) {
	case "placeholder", "changeme", "your_api_key", "your-api-key", "api_key_here", "replace-me", "replace_with_api_key", "<api-key>":
		return false
	default:
		return true
	}
}
