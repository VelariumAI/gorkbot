package selfmod

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
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

var (
	traceMu   sync.RWMutex
	traceSink trace.Sink = trace.NoopSink{}
	traceMode trace.Mode = trace.ModeOff
)

func SetTraceSink(sink trace.Sink, mode trace.Mode) {
	traceMu.Lock()
	defer traceMu.Unlock()
	if sink == nil {
		sink = trace.NoopSink{}
	}
	traceSink = sink
	traceMode = mode
}

func emitValidationTrace(decision DynamicValidationDecision, receipt DynamicValidationReceipt) {
	traceMu.RLock()
	sink := traceSink
	mode := traceMode
	traceMu.RUnlock()
	if sink == nil || mode == trace.ModeOff {
		return
	}
	ev := trace.NewEvent("selfmod", "selfmod_validation")
	ev.Operator = trace.OperatorVerify
	ev.Decision = trace.RedactString(decision.ReasonCode, 64)
	ev.ReasonCode = trace.RedactString(decision.ReasonCode, 128)
	ev.Status = "ok"
	if !decision.Allowed || decision.HardBlock {
		ev.Status = "blocked"
	}
	ev.ErrorClass = trace.RedactString(receipt.RiskClass, 64)
	ev.RedactionState = trace.RedactionRedacted
	ev.ReceiptRefs = []trace.Ref{
		trace.NewRef("operation_id", receipt.OperationID, "", 0),
		trace.NewRef("manifest_hash", receipt.ManifestHash, receipt.ManifestHash, 0),
		trace.NewRef("artifact_hash", receipt.ArtifactHash, receipt.ArtifactHash, 0),
	}
	ev.ArtifactRefs = []trace.Ref{
		trace.NewRef(receipt.ArtifactKind, receipt.ArtifactName, receipt.ArtifactHash, 0),
	}
	ev.Metadata = trace.NewMetadata(map[string]string{
		"allowed":           boolString(decision.Allowed),
		"requires_approval": boolString(decision.RequiresApproval),
		"hard_block":        boolString(decision.HardBlock),
		"issue_count":       intString(receipt.IssuesCount),
		"target_count":      intString(len(receipt.TargetPaths)),
	})
	if !receipt.CreatedAt.IsZero() {
		ev.Timestamp = receipt.CreatedAt.UTC()
	}
	_ = trace.Emit(context.Background(), sink, mode, ev)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intString(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func hashAny(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
