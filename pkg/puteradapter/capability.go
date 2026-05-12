package puteradapter

import "strings"

// Capability is the safe identifier for Puter capability grants.
type Capability string

const (
	CapabilityFSRead         Capability = "puter.fs.read"
	CapabilityFSWrite        Capability = "puter.fs.write"
	CapabilityFSDelete       Capability = "puter.fs.delete"
	CapabilityFSMove         Capability = "puter.fs.move"
	CapabilityKVGet          Capability = "puter.kv.get"
	CapabilityKVSet          Capability = "puter.kv.set"
	CapabilityKVDelete       Capability = "puter.kv.delete"
	CapabilityUIWindowCreate Capability = "puter.ui.window.create"
	CapabilityAppPreview     Capability = "puter.app.preview"
	CapabilityHostingPublish Capability = "puter.hosting.publish"
	CapabilityNetworkFetch   Capability = "puter.network.fetch"
	CapabilityAuthRequest    Capability = "puter.auth.request"
	CapabilityBridgeHost     Capability = "puter.bridge.host"
)

// PuterCapabilityGrant is a validated capability artifact.
type PuterCapabilityGrant struct {
	capability       Capability
	requiresApproval bool
}

func (g PuterCapabilityGrant) Capability() Capability {
	return g.capability
}

func (g PuterCapabilityGrant) RequiresApproval() bool {
	return g.requiresApproval
}

// ValidateCapability ensures the capability belongs to the PR-005 allowlist.
func ValidateCapability(raw string) (Capability, bool) {
	capability := Capability(strings.ToLower(strings.TrimSpace(raw)))
	switch capability {
	case CapabilityFSRead,
		CapabilityFSWrite,
		CapabilityFSDelete,
		CapabilityFSMove,
		CapabilityKVGet,
		CapabilityKVSet,
		CapabilityKVDelete,
		CapabilityUIWindowCreate,
		CapabilityAppPreview,
		CapabilityHostingPublish,
		CapabilityNetworkFetch,
		CapabilityAuthRequest,
		CapabilityBridgeHost:
		return capability, true
	default:
		return "", false
	}
}

// ValidatePuterCapabilityGrant creates a safe capability grant artifact.
func ValidatePuterCapabilityGrant(raw string, requiresApproval bool) (PuterCapabilityGrant, Decision) {
	capability, ok := ValidateCapability(raw)
	if !ok {
		return PuterCapabilityGrant{}, Decision{Allowed: false, ReasonCode: ReasonCapabilityBlocked}
	}
	return PuterCapabilityGrant{
		capability:       capability,
		requiresApproval: requiresApproval,
	}, Decision{Allowed: true, Capability: capability, ReasonCode: ReasonAllowed}
}
