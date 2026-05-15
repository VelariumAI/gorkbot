package statelock

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type CheckStatus string

type CheckResult struct {
	Status          CheckStatus    `json:"status"`
	Conflicts       []Conflict     `json:"conflicts,omitempty"`
	Paradox         *ParadoxReport `json:"paradox,omitempty"`
	PolicyState     PolicyState    `json:"policy_state"`
	Risk            Risk           `json:"risk"`
	Recommendations []Remediation  `json:"recommendations,omitempty"`
	Err             error          `json:"-"`
}

type Evaluator struct {
	Store Store
}

const (
	CheckStatusAllowed  CheckStatus = "allowed"
	CheckStatusConflict CheckStatus = "conflict"
	CheckStatusInvalid  CheckStatus = "invalid"
)

func (e *Evaluator) Check(ctx context.Context, proposed ProposedState) CheckResult {
	p := proposed.Normalized()
	if err := p.Validate(); err != nil {
		return CheckResult{
			Status:      CheckStatusInvalid,
			PolicyState: p.PolicyState,
			Risk:        p.Risk,
			Err:         err,
		}
	}

	var locks []Lock
	if e != nil && e.Store != nil {
		listed, err := e.Store.ListLocks(ctx, Filter{
			Scope:     p.Scope,
			Dimension: p.Dimension,
			Subject:   p.Subject,
			Status:    StatusActive,
		})
		if err != nil {
			return CheckResult{
				Status:      CheckStatusInvalid,
				PolicyState: p.PolicyState,
				Risk:        p.Risk,
				Err:         err,
			}
		}
		locks = listed
	}

	conflicts := DetectConflicts(locks, p)
	if len(conflicts) == 0 {
		return CheckResult{
			Status:      CheckStatusAllowed,
			PolicyState: p.PolicyState,
			Risk:        p.Risk,
		}
	}

	report := buildParadoxReport(p, conflicts)
	return CheckResult{
		Status:          CheckStatusConflict,
		Conflicts:       conflicts,
		Paradox:         &report,
		PolicyState:     p.PolicyState,
		Risk:            p.Risk,
		Recommendations: append([]Remediation(nil), report.Remediation...),
		Err:             ErrLockConflict,
	}
}

func buildParadoxReport(proposed ProposedState, conflicts []Conflict) ParadoxReport {
	start := time.Now().UTC()
	status := ParadoxPossible
	if hasCriticalConflict(conflicts) {
		status = ParadoxConfirmed
	}
	constraints := make([]Constraint, 0, len(conflicts))
	recommendations := make([]Remediation, 0, len(conflicts))
	seenRemediation := map[string]struct{}{}
	for i := range conflicts {
		c := conflicts[i]
		constraints = append(constraints, Constraint{
			Code:      c.ReasonCode,
			Summary:   conflictSummary(c),
			Hard:      c.Severity == SeverityCritical || c.Severity == SeverityHigh,
			Dimension: string(c.Dimension),
		})
		for _, step := range c.Remediation {
			kind := remediationKindForReason(c.ReasonCode, step)
			key := string(kind) + ":" + strings.ToLower(step)
			if _, ok := seenRemediation[key]; ok {
				continue
			}
			seenRemediation[key] = struct{}{}
			recommendations = append(recommendations, Remediation{
				Kind:        kind,
				Description: step,
			})
		}
	}
	report := ParadoxReport{
		Status:       status,
		Summary:      "no valid path under current lock/policy constraints",
		Conflicts:    conflicts,
		Constraints:  constraints,
		PolicyState:  proposed.PolicyState,
		Risk:         proposed.Risk,
		EvidenceRefs: proposed.EvidenceRefs,
		Remediation:  recommendations,
		StartedAt:    start,
		FinishedAt:   start,
		Metadata: normalizeMetadata(map[string]string{
			"subject":        proposed.Subject,
			"scope":          string(proposed.Scope),
			"dimension":      string(proposed.Dimension),
			"conflict_count": strconv.Itoa(len(conflicts)),
		}),
	}
	norm := report.Normalized()
	norm.Metadata = normalizeMetadata(map[string]string{
		"subject":   proposed.Subject,
		"scope":     string(proposed.Scope),
		"dimension": string(proposed.Dimension),
	})
	return norm
}

func hasCriticalConflict(conflicts []Conflict) bool {
	for i := range conflicts {
		if normalizeSeverity(conflicts[i].Severity) == SeverityCritical {
			return true
		}
	}
	return false
}

func conflictSummary(c Conflict) string {
	base := c.ReasonCode
	if base == "" {
		base = "unknown_conflict"
	}
	return trace.RedactString(base+" for "+c.Subject, 256)
}

func remediationKindForReason(reasonCode, step string) RemediationKind {
	r := strings.ToLower(strings.TrimSpace(reasonCode))
	s := strings.ToLower(strings.TrimSpace(step))
	switch {
	case strings.Contains(r, "policy"):
		if strings.Contains(s, "approval") {
			return RemediationRequestApproval
		}
		if strings.Contains(s, "reduce") {
			return RemediationReduceRisk
		}
		return RemediationProvidePolicy
	case strings.Contains(r, "permission_scope"):
		if strings.Contains(s, "reduce") {
			return RemediationReduceRisk
		}
		return RemediationRequestApproval
	case strings.Contains(r, "validation"):
		return RemediationSplitTask
	case strings.Contains(r, "budget"):
		if strings.Contains(s, "approval") {
			return RemediationRequestApproval
		}
		return RemediationReduceRisk
	case strings.Contains(r, "research"):
		return RemediationProvidePolicy
	default:
		if strings.Contains(s, "scope") {
			return RemediationChangeScope
		}
		return RemediationAbort
	}
}

func boundStringList(in []string, maxEntries, maxLen int) []string {
	if len(in) == 0 || maxEntries <= 0 || maxLen <= 0 {
		return nil
	}
	if len(in) > maxEntries {
		in = in[:maxEntries]
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		clean := trace.RedactString(strings.TrimSpace(item), maxLen)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
