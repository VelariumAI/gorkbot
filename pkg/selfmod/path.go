package selfmod

import (
	"path/filepath"
	"strings"
)

var stagingPrefixes = []string{
	".gorkbot/staging/",
	".gorkbot/generated/",
	"pkg/tools/custom/_staged/",
}

var protectedPrefixes = []string{
	".git/",
	".github/",
	"configs/",
	"scripts/",
	"cmd/",
	"internal/",
	"pkg/governance/",
	"pkg/researchgate/",
	"pkg/puteradapter/",
	"pkg/vcseclient/",
}

var protectedFiles = map[string]bool{
	"pkg/tools/registry.go":          true,
	"pkg/tools/permissions.go":       true,
	"pkg/tools/audit_db.go":          true,
	"go.mod":                         true,
	"go.sum":                         true,
	"README.md":                      true,
	"LICENSE":                        true,
	"configs/promotion-manifest.txt": true,
}

func validateTargetPath(raw DynamicArtifactPath) (requiresApproval bool, hardBlock bool, reason string, issue string) {
	path := raw.String()
	if path == "" {
		return false, true, REASON_DYNAMIC_PATH_OUTSIDE_STAGING, "empty path"
	}
	if hasControlChars(path) {
		return false, true, REASON_DYNAMIC_CONTROL_CHAR, "control characters in path"
	}
	if strings.Contains(path, "\x00") {
		return false, true, REASON_DYNAMIC_CONTROL_CHAR, "NUL byte in path"
	}
	if filepath.IsAbs(path) {
		return false, true, REASON_DYNAMIC_PATH_OUTSIDE_STAGING, "absolute path is forbidden"
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == "" {
		return false, true, REASON_DYNAMIC_PATH_OUTSIDE_STAGING, "empty cleaned path"
	}
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return false, true, REASON_DYNAMIC_PATH_TRAVERSAL, "path traversal"
	}

	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(clean, ".github/workflows/") {
			return true, false, REASON_DYNAMIC_PROMOTION_REQUIRES_APPROVAL, "workflow promotion requires approval"
		}
		if strings.HasPrefix(clean, prefix) {
			return false, true, REASON_DYNAMIC_PROTECTED_TARGET, "protected path: " + clean
		}
	}
	if protectedFiles[clean] {
		return false, true, REASON_DYNAMIC_PROTECTED_TARGET, "protected file: " + clean
	}

	for _, prefix := range stagingPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return false, false, "", ""
		}
	}

	return false, true, REASON_DYNAMIC_PATH_OUTSIDE_STAGING, "path must be staged first"
}

func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 {
			return true
		}
	}
	return false
}
