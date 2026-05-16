package evidence

import (
	"fmt"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

// Record is a bounded evidence atom for trace/replay/harness/governance linkage.
type Record struct {
	ID           string            `json:"id"`
	Kind         Kind              `json:"kind"`
	Status       Status            `json:"status"`
	Subject      string            `json:"subject"`
	Summary      string            `json:"summary"`
	Source       string            `json:"source,omitempty"`
	PolicyState  PolicyState       `json:"policy_state,omitempty"`
	Risk         Risk              `json:"risk,omitempty"`
	Authority    Authority         `json:"authority,omitempty"`
	EvidenceRefs []trace.Ref       `json:"evidence_refs,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (r Record) Normalized() Record {
	out := r
	out.Kind = NormalizeKind(string(out.Kind))
	out.Status = NormalizeStatus(string(out.Status))
	out.Subject = boundString(out.Subject, maxSubjectLen)
	out.Summary = boundString(out.Summary, maxSummaryLen)
	out.Source = boundString(out.Source, maxSourceLen)
	out.PolicyState = NormalizePolicyState(string(out.PolicyState))
	out.Risk = NormalizeRisk(string(out.Risk))
	out.Authority = NormalizeAuthority(string(out.Authority))
	out.EvidenceRefs = boundRefs(out.EvidenceRefs, maxRefCount)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataCount)
	if out.CreatedAt.IsZero() {
		out.CreatedAt = zeroTime
	} else {
		out.CreatedAt = out.CreatedAt.UTC()
	}
	if out.ID == "" {
		out.ID = "record_" + trace.StableHash(
			string(out.Kind),
			string(out.Status),
			out.Subject,
			out.Summary,
			out.Source,
			string(out.PolicyState),
			string(out.Risk),
			string(out.Authority),
			stableRefsHash(out.EvidenceRefs),
			stableMetadataHash(out.Metadata),
		)
	}
	return out
}

func (r Record) Validate() error {
	n := r.Normalized()
	if n.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	if n.Kind == KindUnknown {
		return fmt.Errorf("%w: kind unknown", ErrInvalidRecord)
	}
	if n.Subject == "" {
		return fmt.Errorf("%w: subject required", ErrInvalidRecord)
	}
	if n.Summary == "" {
		return fmt.Errorf("%w: summary required", ErrInvalidRecord)
	}
	if n.Status == StatusInvalid || n.Status == StatusUnknown {
		return fmt.Errorf("%w: status invalid", ErrInvalidRecord)
	}
	return nil
}
