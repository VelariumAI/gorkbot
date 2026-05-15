package statelock

import (
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type ParadoxStatus string

type RemediationKind string

type Constraint struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Hard      bool   `json:"hard"`
	Dimension string `json:"dimension,omitempty"`
}

type Remediation struct {
	Kind        RemediationKind `json:"kind"`
	Description string          `json:"description"`
}

type ParadoxReport struct {
	ID           string            `json:"id"`
	Status       ParadoxStatus     `json:"status"`
	Summary      string            `json:"summary"`
	Conflicts    []Conflict        `json:"conflicts,omitempty"`
	Constraints  []Constraint      `json:"constraints,omitempty"`
	PolicyState  PolicyState       `json:"policy_state"`
	Risk         Risk              `json:"risk"`
	EvidenceRefs []trace.Ref       `json:"evidence_refs,omitempty"`
	Remediation  []Remediation     `json:"remediation,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   time.Time         `json:"finished_at"`
	Duration     time.Duration     `json:"duration"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

const (
	ParadoxNone         ParadoxStatus = "none"
	ParadoxPossible     ParadoxStatus = "possible"
	ParadoxConfirmed    ParadoxStatus = "confirmed"
	ParadoxInvalid      ParadoxStatus = "invalid"
	ParadoxInconclusive ParadoxStatus = "inconclusive"
)

const (
	RemediationRequestApproval RemediationKind = "request_approval"
	RemediationRelaxConstraint RemediationKind = "relax_constraint"
	RemediationChangeScope     RemediationKind = "change_scope"
	RemediationProvidePolicy   RemediationKind = "provide_policy"
	RemediationReduceRisk      RemediationKind = "reduce_risk"
	RemediationSplitTask       RemediationKind = "split_task"
	RemediationAbort           RemediationKind = "abort"
)

func (p ParadoxReport) Normalized() ParadoxReport {
	out := p
	out.ID = trace.RedactString(strings.TrimSpace(out.ID), maxIDLen)
	out.Status = normalizeParadoxStatus(out.Status)
	out.Summary = trace.RedactString(strings.TrimSpace(out.Summary), 256)
	out.PolicyState = normalizePolicyState(string(out.PolicyState))
	out.Risk = normalizeRisk(string(out.Risk))
	out.Conflicts = normalizeConflicts(out.Conflicts)
	out.Constraints = normalizeConstraints(out.Constraints)
	out.EvidenceRefs = normalizeRefs(out.EvidenceRefs)
	out.Remediation = normalizeRemediation(out.Remediation)
	out.Metadata = normalizeMetadata(out.Metadata)
	if out.StartedAt.IsZero() {
		out.StartedAt = time.Now().UTC()
	}
	if out.FinishedAt.IsZero() || out.FinishedAt.Before(out.StartedAt) {
		out.FinishedAt = out.StartedAt
	}
	out.Duration = out.FinishedAt.Sub(out.StartedAt)
	if out.Duration < 0 {
		out.Duration = 0
	}
	if out.ID == "" {
		out.ID = "paradox_" + trace.StableHash(out.Summary, string(out.Status), string(out.PolicyState), string(out.Risk))
	}
	return out
}

func (p ParadoxReport) Validate() error {
	n := p.Normalized()
	if n.ID == "" || n.Summary == "" {
		return fmt.Errorf("%w: missing id/summary", ErrInvalidParadoxReport)
	}
	if n.Status == ParadoxInvalid {
		return fmt.Errorf("%w: invalid status", ErrInvalidParadoxReport)
	}
	if n.StartedAt.IsZero() {
		return fmt.Errorf("%w: missing timestamps", ErrInvalidParadoxReport)
	}
	return nil
}

func normalizeParadoxStatus(s ParadoxStatus) ParadoxStatus {
	n := ParadoxStatus(strings.ToLower(strings.TrimSpace(string(s))))
	switch n {
	case ParadoxNone, ParadoxPossible, ParadoxConfirmed, ParadoxInconclusive:
		return n
	default:
		return ParadoxInvalid
	}
}

func normalizeConflicts(in []Conflict) []Conflict {
	if len(in) == 0 {
		return nil
	}
	out := make([]Conflict, 0, len(in))
	for i := range in {
		out = append(out, newConflict(in[i]))
	}
	return out
}

func normalizeConstraints(in []Constraint) []Constraint {
	if len(in) == 0 {
		return nil
	}
	if len(in) > 24 {
		in = in[:24]
	}
	out := make([]Constraint, 0, len(in))
	for i := range in {
		c := Constraint{
			Code:      trace.RedactString(strings.TrimSpace(in[i].Code), 128),
			Summary:   trace.RedactString(strings.TrimSpace(in[i].Summary), 256),
			Hard:      in[i].Hard,
			Dimension: trace.RedactString(strings.TrimSpace(in[i].Dimension), 64),
		}
		if c.Code == "" || c.Summary == "" {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRemediation(in []Remediation) []Remediation {
	if len(in) == 0 {
		return nil
	}
	if len(in) > 24 {
		in = in[:24]
	}
	out := make([]Remediation, 0, len(in))
	for i := range in {
		r := Remediation{
			Kind:        normalizeRemediationKind(in[i].Kind),
			Description: trace.RedactString(strings.TrimSpace(in[i].Description), 160),
		}
		if r.Description == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRemediationKind(in RemediationKind) RemediationKind {
	s := RemediationKind(strings.ToLower(strings.TrimSpace(string(in))))
	switch s {
	case RemediationRequestApproval, RemediationRelaxConstraint, RemediationChangeScope,
		RemediationProvidePolicy, RemediationReduceRisk, RemediationSplitTask, RemediationAbort:
		return s
	default:
		return RemediationAbort
	}
}
