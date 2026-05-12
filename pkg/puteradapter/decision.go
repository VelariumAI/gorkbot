package puteradapter

// Decision is the bounded policy outcome used across validators and adapter operations.
type Decision struct {
	Allowed          bool
	RequiresApproval bool
	ReasonCode       string
	Capability       Capability
}

const (
	ReasonAllowed                        = "ALLOWED"
	ReasonAdapterDisabled                = "ADAPTER_DISABLED"
	ReasonInvalidOperation               = "INVALID_OPERATION"
	ReasonInvalidPath                    = "INVALID_PATH"
	ReasonPathTraversalBlocked           = "PATH_TRAVERSAL_BLOCKED"
	ReasonOutsideWorkspaceRoot           = "OUTSIDE_WORKSPACE_ROOT"
	ReasonControlCharacterBlocked        = "CONTROL_CHARACTER_BLOCKED"
	ReasonPathWriteScopeBlocked          = "PATH_WRITE_SCOPE_BLOCKED"
	ReasonProtectedWriteRequiresApproval = "PROTECTED_WRITE_REQUIRES_APPROVAL"
	ReasonProtectedDeleteBlocked         = "PROTECTED_DELETE_BLOCKED"
	ReasonDeleteRequiresApproval         = "DELETE_REQUIRES_APPROVAL"
	ReasonMoveRequiresApproval           = "MOVE_REQUIRES_APPROVAL"
	ReasonKVNamespaceBlocked             = "KV_NAMESPACE_BLOCKED"
	ReasonKVSetRequiresApproval          = "KV_SET_REQUIRES_APPROVAL"
	ReasonKVDeleteRequiresApproval       = "KV_DELETE_REQUIRES_APPROVAL"
	ReasonCapabilityBlocked              = "CAPABILITY_BLOCKED"
	ReasonCapabilityRequiresApproval     = "CAPABILITY_REQUIRES_APPROVAL"
)
