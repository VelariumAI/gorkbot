package puteradapter

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PuterWorkspacePath is a validated workspace path artifact.
type PuterWorkspacePath struct {
	value string
}

// String returns the normalized absolute workspace path.
func (p PuterWorkspacePath) String() string {
	return p.value
}

func (p PuterWorkspacePath) inScope(prefix string) bool {
	if p.value == prefix {
		return true
	}
	return strings.HasPrefix(p.value, prefix+"/")
}

// ValidatePuterWorkspacePath validates, normalizes, and constrains a path to the workspace root.
// Symlink resolution is intentionally unsupported in PR-005: callers must not rely on symlink
// traversal or link-following behavior for governed operations.
func ValidatePuterWorkspacePath(raw string, manifest PuterWorkspaceManifest) (PuterWorkspacePath, Decision) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return PuterWorkspacePath{}, Decision{Allowed: false, ReasonCode: ReasonInvalidPath}
	}
	if !utf8.ValidString(trimmed) || containsControlRune(trimmed) {
		return PuterWorkspacePath{}, Decision{Allowed: false, ReasonCode: ReasonControlCharacterBlocked}
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if strings.HasPrefix(normalized, "//") {
		normalized = "/" + strings.TrimLeft(normalized, "/")
	}
	root := manifest.Root()
	candidate := normalized
	if !strings.HasPrefix(candidate, "/") {
		candidate = strings.TrimRight(root, "/") + "/" + candidate
	}
	cleaned := path.Clean(candidate)
	if cleaned == "." || cleaned == "/" {
		return PuterWorkspacePath{}, Decision{Allowed: false, ReasonCode: ReasonInvalidPath}
	}
	if cleaned != root && !strings.HasPrefix(cleaned, root+"/") {
		return PuterWorkspacePath{}, Decision{Allowed: false, ReasonCode: ReasonOutsideWorkspaceRoot}
	}
	if strings.Contains(cleaned, "../") || strings.HasSuffix(cleaned, "/..") {
		return PuterWorkspacePath{}, Decision{Allowed: false, ReasonCode: ReasonPathTraversalBlocked}
	}
	return PuterWorkspacePath{value: cleaned}, Decision{Allowed: true, ReasonCode: ReasonAllowed}
}

func containsControlRune(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
