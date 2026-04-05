package version

import "fmt"

// Internal versioning (development)
const InternalVersion = "6.2.0"

// Public versioning (user-facing releases)
const PublicMajor = 1
const PublicMinor = 6
const PublicBuild = 1
const PublicRevision = 0 // 0 = no revision suffix
const PublicStatus = "rc" // alpha|beta|rc|"" (stable)

// GetInternalVersion returns the internal development version.
func GetInternalVersion() string {
	return InternalVersion
}

// GetPublicVersion returns the formatted public release version.
// Format: major.minor[.build[.revision]]-(status)
// Examples: 1.2.0-beta, 1.2.12.102-rc, 1.2.0-alpha
func GetPublicVersion() string {
	version := fmt.Sprintf("%d.%d.%d", PublicMajor, PublicMinor, PublicBuild)

	// Add revision if non-zero
	if PublicRevision > 0 {
		version = fmt.Sprintf("%s.%d", version, PublicRevision)
	}

	// Add release status
	if PublicStatus != "" {
		version = fmt.Sprintf("%s-%s", version, PublicStatus)
	}

	return version
}

// GetVersionInfo returns both internal and public versions with description.
func GetVersionInfo() string {
	return fmt.Sprintf("Gorkbot %s (internal: %s)", GetPublicVersion(), GetInternalVersion())
}

// Subsystem Versions (maintained independently)
const SENSEVersion = "1.9.0"
const SREVersion = "1.0.0"
const XSKILLVersion = "1.0.0"
const HarnessVersion = "1.0.0" // Multi-session state tracking + verification
const ResearchVersion = "1.0.0" // Deep research engine with context-efficient web browsing
const SelfImproveVersion = "1.0.0" // Autonomous self-improvement orchestrator

// GetSubsystemVersions returns a map of all subsystem versions.
func GetSubsystemVersions() map[string]string {
	return map[string]string{
		"SENSE":        SENSEVersion,
		"SRE":          SREVersion,
		"XSKILL":       XSKILLVersion,
		"Harness":      HarnessVersion,
		"Research":     ResearchVersion,
		"SelfImprove":  SelfImproveVersion,
	}
}
