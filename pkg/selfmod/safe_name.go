package selfmod

import (
	"fmt"
	"strings"
)

// ValidateSafeArtifactName rejects names that could traverse out of a staging
// directory when interpolated into a path. The constructed stage path is
// otherwise opaque to the manifest validator, so callers MUST sanitise the
// name before joining it with any directory prefix.
func ValidateSafeArtifactName(raw string) error {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fmt.Errorf("%s: name", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	if strings.ContainsAny(v, "/\\") {
		return fmt.Errorf("%s: name contains path separator", REASON_DYNAMIC_PATH_TRAVERSAL)
	}
	if strings.Contains(v, "..") {
		return fmt.Errorf("%s: name contains parent reference", REASON_DYNAMIC_PATH_TRAVERSAL)
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s: name contains control character", REASON_DYNAMIC_CONTROL_CHAR)
		}
	}
	if strings.HasPrefix(v, ".") || strings.HasPrefix(v, "-") {
		return fmt.Errorf("%s: name has reserved prefix", REASON_DYNAMIC_PATH_TRAVERSAL)
	}
	return nil
}

// ValidateStagedTargetPath exposes the internal target-path validator for
// callers that construct a stage path themselves and need to ensure it lands
// inside an allowed staging prefix (no traversal, no protected target).
func ValidateStagedTargetPath(path string) (requiresApproval bool, hardBlock bool, reason string, issue string) {
	return validateTargetPath(DynamicArtifactPath{value: path})
}
