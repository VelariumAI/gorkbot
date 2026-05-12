package puteradapter

import "strings"

// CapabilityPolicy holds deterministic allow/approve/block rules for Puter operations.
type CapabilityPolicy struct{}

// DefaultCapabilityPolicy returns the baseline policy for PR-005.
func DefaultCapabilityPolicy() CapabilityPolicy {
	return CapabilityPolicy{}
}

func capabilityForOperation(op PuterOperation) Capability {
	switch op {
	case OpFSRead:
		return CapabilityFSRead
	case OpFSWrite:
		return CapabilityFSWrite
	case OpFSDelete:
		return CapabilityFSDelete
	case OpFSMove:
		return CapabilityFSMove
	case OpKVGet:
		return CapabilityKVGet
	case OpKVSet:
		return CapabilityKVSet
	case OpKVDelete:
		return CapabilityKVDelete
	case OpUIWindowCreate:
		return CapabilityUIWindowCreate
	case OpAppPreview:
		return CapabilityAppPreview
	case OpHostingPublish:
		return CapabilityHostingPublish
	case OpNetworkFetch:
		return CapabilityNetworkFetch
	case OpAuthRequest:
		return CapabilityAuthRequest
	case OpBridgeHost:
		return CapabilityBridgeHost
	default:
		return ""
	}
}

// EvaluatePathOperation returns path policy decisions for filesystem operations.
func (p CapabilityPolicy) EvaluatePathOperation(op PuterOperation, path PuterWorkspacePath, manifest PuterWorkspaceManifest) Decision {
	cap := capabilityForOperation(op)
	if cap == "" {
		return Decision{Allowed: false, ReasonCode: ReasonInvalidOperation}
	}

	if isSecretsPath(path.String()) {
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonCapabilityBlocked}
	}

	isProtected := pathIsProtected(path, manifest)
	switch op {
	case OpFSRead:
		return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
	case OpFSWrite:
		if isProtected {
			return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonProtectedWriteRequiresApproval}
		}
		if pathIsWriteAllowed(path, manifest) {
			return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
		}
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonPathWriteScopeBlocked}
	case OpFSDelete:
		if isHardProtectedDelete(path, manifest) {
			return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonProtectedDeleteBlocked}
		}
		if isProtected {
			return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonDeleteRequiresApproval}
		}
		return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonDeleteRequiresApproval}
	default:
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonInvalidOperation}
	}
}

// EvaluateMoveOperation evaluates move scope rules.
func (p CapabilityPolicy) EvaluateMoveOperation(from, to PuterWorkspacePath, manifest PuterWorkspaceManifest) Decision {
	cap := CapabilityFSMove
	if isHardProtectedDelete(from, manifest) || isHardProtectedDelete(to, manifest) {
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonProtectedDeleteBlocked}
	}
	fromScope := topScope(from.String(), manifest.Root())
	toScope := topScope(to.String(), manifest.Root())
	if fromScope == "" || toScope == "" {
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonInvalidPath}
	}
	if fromScope != toScope {
		return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonMoveRequiresApproval}
	}
	if pathIsProtected(from, manifest) || pathIsProtected(to, manifest) {
		return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonMoveRequiresApproval}
	}
	return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
}

// EvaluateKVOperation returns key-value policy decisions.
func (p CapabilityPolicy) EvaluateKVOperation(op PuterOperation, key PuterKVKey) Decision {
	cap := capabilityForOperation(op)
	if cap == "" {
		return Decision{Allowed: false, ReasonCode: ReasonInvalidOperation}
	}
	raw := key.String()
	switch op {
	case OpKVGet:
		if strings.HasPrefix(raw, "gorkbot.") {
			return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
		}
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonKVNamespaceBlocked}
	case OpKVSet:
		if strings.HasPrefix(raw, "gorkbot.mission.") {
			return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
		}
		if strings.HasPrefix(raw, "gorkbot.") {
			return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonKVSetRequiresApproval}
		}
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonKVNamespaceBlocked}
	case OpKVDelete:
		if strings.HasPrefix(raw, "gorkbot.") {
			return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonKVDeleteRequiresApproval}
		}
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonKVNamespaceBlocked}
	default:
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonInvalidOperation}
	}
}

// EvaluateStandaloneCapability handles non-path/non-kv capabilities.
func (p CapabilityPolicy) EvaluateStandaloneCapability(op PuterOperation) Decision {
	cap := capabilityForOperation(op)
	if cap == "" {
		return Decision{Allowed: false, ReasonCode: ReasonInvalidOperation}
	}
	switch op {
	case OpUIWindowCreate, OpAppPreview:
		return Decision{Allowed: true, Capability: cap, ReasonCode: ReasonAllowed}
	case OpHostingPublish, OpNetworkFetch, OpAuthRequest:
		return Decision{Allowed: false, RequiresApproval: true, Capability: cap, ReasonCode: ReasonCapabilityRequiresApproval}
	case OpBridgeHost:
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonCapabilityBlocked}
	default:
		return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonInvalidOperation}
	}
}

func pathIsWriteAllowed(path PuterWorkspacePath, manifest PuterWorkspaceManifest) bool {
	for _, prefix := range manifest.writePrefixesCopy() {
		if path.inScope(prefix) {
			return true
		}
	}
	return false
}

func pathIsProtected(path PuterWorkspacePath, manifest PuterWorkspaceManifest) bool {
	for _, prefix := range manifest.protectedCopy() {
		if path.inScope(prefix) {
			return true
		}
	}
	return false
}

func isHardProtectedDelete(path PuterWorkspacePath, manifest PuterWorkspaceManifest) bool {
	for _, suffix := range []string{"/logs", "/receipts", "/ledger"} {
		prefix := strings.TrimRight(manifest.Root(), "/") + suffix
		if path.inScope(prefix) {
			return true
		}
	}
	return false
}

func topScope(pathValue, root string) string {
	trimmed := strings.TrimPrefix(pathValue, strings.TrimRight(root, "/")+"/")
	if trimmed == pathValue || trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func isSecretsPath(pathValue string) bool {
	for _, prefix := range []string{"/secrets/", "/credentials/"} {
		if strings.HasPrefix(pathValue, prefix) || pathValue == strings.TrimSuffix(prefix, "/") {
			return true
		}
	}
	return false
}
