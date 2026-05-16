package skillruntime

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/statelock"
	"github.com/velariumai/gorkbot/pkg/trace"
)

type Request struct {
	Operation       Operation              `json:"operation"`
	Candidate       Candidate              `json:"candidate"`
	Config          profile.Config         `json:"config"`
	DirectiveHash   string                 `json:"directive_hash,omitempty"`
	FailureHash     string                 `json:"failure_hash,omitempty"`
	ReplayResult    *replay.Result         `json:"replay_result,omitempty"`
	HarnessReport   *harness.Report        `json:"harness_report,omitempty"`
	StateLockResult *statelock.CheckResult `json:"statelock_result,omitempty"`
	EvidenceRefs    []trace.Ref            `json:"evidence_refs,omitempty"`
	Metadata        map[string]string      `json:"metadata,omitempty"`
}

func (r Request) Normalized() Request {
	out := r
	out.Operation = NormalizeOperation(string(out.Operation))
	out.Candidate = out.Candidate.Normalized()
	out.Config = out.Config.Normalized()
	out.DirectiveHash = boundHash(out.DirectiveHash)
	out.FailureHash = boundHash(out.FailureHash)
	out.EvidenceRefs = boundRefs(out.EvidenceRefs, maxLifecycleRefCount)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataEntries)

	if out.ReplayResult != nil {
		cp := *out.ReplayResult
		cp.CaseID = trace.RedactString(strings.TrimSpace(cp.CaseID), 128)
		cp.TrajectoryID = trace.RedactString(strings.TrimSpace(cp.TrajectoryID), 256)
		cp.CandidateID = trace.RedactString(strings.TrimSpace(cp.CandidateID), 128)
		cp.BaselineOutcome = normalizeReplayOutcome(cp.BaselineOutcome)
		cp.CandidateOutcome = normalizeReplayOutcome(cp.CandidateOutcome)
		if cp.Duration < 0 {
			cp.Duration = 0
		}
		out.ReplayResult = &cp
	}
	if out.HarnessReport != nil {
		n := out.HarnessReport.Normalized()
		out.HarnessReport = &n
	}
	if out.StateLockResult != nil {
		cp := *out.StateLockResult
		cp.PolicyState = statelock.PolicyState(strings.ToLower(strings.TrimSpace(string(cp.PolicyState))))
		cp.Risk = statelock.Risk(strings.ToLower(strings.TrimSpace(string(cp.Risk))))
		out.StateLockResult = &cp
	}
	return out
}

func (r Request) Validate() error {
	n := r.Normalized()
	if err := n.Operation.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if err := n.Candidate.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return nil
}

func normalizeReplayOutcome(in replay.Outcome) replay.Outcome {
	out := in
	if out.DurationMS < 0 {
		out.DurationMS = 0
	}
	if out.Retries < 0 {
		out.Retries = 0
	}
	if out.RepairOperations < 0 {
		out.RepairOperations = 0
	}
	out.ValidationRefs = boundRefs(out.ValidationRefs, maxLifecycleRefCount)
	out.ReceiptRefs = boundRefs(out.ReceiptRefs, maxLifecycleRefCount)
	return out
}
