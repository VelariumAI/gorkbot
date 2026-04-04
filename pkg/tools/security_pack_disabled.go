//go:build !with_security

package tools

// getSecurityPack returns an empty list when compiled without -tags with-security.
// Security tools are disabled by default to prevent accidental exposure of
// penetration testing capabilities to untrusted AI agents.
func getSecurityPack() []Tool {
	return nil
}
