package selfmod

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type DynamicValidationReceipt struct {
	OperationID      string    `json:"operation_id"`
	ArtifactKind     string    `json:"artifact_kind"`
	ArtifactName     string    `json:"artifact_name"`
	TargetPaths      []string  `json:"target_paths"`
	Capabilities     []string  `json:"capabilities"`
	RiskClass        string    `json:"risk_class"`
	Allowed          bool      `json:"allowed"`
	RequiresApproval bool      `json:"requires_approval"`
	ReasonCode       string    `json:"reason_code"`
	IssuesCount      int       `json:"issues_count"`
	ManifestHash     string    `json:"manifest_hash"`
	ArtifactHash     string    `json:"artifact_hash"`
	CreatedAt        time.Time `json:"created_at"`
}

func hashAny(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
