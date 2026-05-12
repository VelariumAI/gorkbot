package puteradapter

import "strings"

// PuterOperation identifies a validated operation request.
type PuterOperation string

const (
	OpFSRead         PuterOperation = "puter.fs.read"
	OpFSWrite        PuterOperation = "puter.fs.write"
	OpFSDelete       PuterOperation = "puter.fs.delete"
	OpFSMove         PuterOperation = "puter.fs.move"
	OpKVGet          PuterOperation = "puter.kv.get"
	OpKVSet          PuterOperation = "puter.kv.set"
	OpKVDelete       PuterOperation = "puter.kv.delete"
	OpUIWindowCreate PuterOperation = "puter.ui.window.create"
	OpAppPreview     PuterOperation = "puter.app.preview"
	OpHostingPublish PuterOperation = "puter.hosting.publish"
	OpNetworkFetch   PuterOperation = "puter.network.fetch"
	OpAuthRequest    PuterOperation = "puter.auth.request"
	OpBridgeHost     PuterOperation = "puter.bridge.host"
)

// ValidatePuterOperation converts raw operation strings into safe typed operations.
func ValidatePuterOperation(raw string) (PuterOperation, Decision) {
	op := PuterOperation(strings.ToLower(strings.TrimSpace(raw)))
	switch op {
	case OpFSRead, OpFSWrite, OpFSDelete, OpFSMove,
		OpKVGet, OpKVSet, OpKVDelete,
		OpUIWindowCreate, OpAppPreview,
		OpHostingPublish, OpNetworkFetch,
		OpAuthRequest, OpBridgeHost:
		return op, Decision{Allowed: true, ReasonCode: ReasonAllowed}
	default:
		return "", Decision{Allowed: false, ReasonCode: ReasonInvalidOperation}
	}
}
