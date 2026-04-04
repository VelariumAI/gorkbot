package provider

import (
	"context"
	"fmt"
)

// AllowlistProvider implements GuardrailsProvider using allowlist/blocklist logic.
// If a tool is in the blocklist, it's denied regardless of the allowlist.
// If the allowlist is empty, all tools are permitted (unless blocked).
type AllowlistProvider struct {
	allowedTools map[string]bool
	blockedTools map[string]bool
}

// newAllowlistProvider creates a new allowlist-based guardrails provider.
// It reads params["allowed_tools"] and params["blocked_tools"] as []interface{} (strings).
func newAllowlistProvider(params map[string]interface{}) (GuardrailsProvider, error) {
	ap := &AllowlistProvider{
		allowedTools: make(map[string]bool),
		blockedTools: make(map[string]bool),
	}

	// Parse allowed_tools
	if allowedIface, ok := params["allowed_tools"]; ok {
		if allowedSlice, ok := allowedIface.([]interface{}); ok {
			for _, tool := range allowedSlice {
				if toolStr, ok := tool.(string); ok {
					ap.allowedTools[toolStr] = true
				}
			}
		}
	}

	// Parse blocked_tools
	if blockedIface, ok := params["blocked_tools"]; ok {
		if blockedSlice, ok := blockedIface.([]interface{}); ok {
			for _, tool := range blockedSlice {
				if toolStr, ok := tool.(string); ok {
					ap.blockedTools[toolStr] = true
				}
			}
		}
	}

	return ap, nil
}

// Authorize checks if a tool is allowed.
// Blocklist takes priority: if the tool is blocked, deny immediately.
// If the allowlist is non-empty and the tool is not in it, deny.
// Otherwise, allow.
func (a *AllowlistProvider) Authorize(ctx context.Context, toolName string, params map[string]interface{}) error {
	// Blocklist takes priority
	if a.blockedTools[toolName] {
		return fmt.Errorf("tool %q is blocked", toolName)
	}

	// If allowlist is non-empty, require membership
	if len(a.allowedTools) > 0 && !a.allowedTools[toolName] {
		return fmt.Errorf("tool %q is not in the allowlist", toolName)
	}

	return nil
}

// Name returns the name of this provider.
func (a *AllowlistProvider) Name() string {
	return "AllowlistProvider"
}
