package statelock

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type Severity string

type ProposedState struct {
	Scope        Scope             `json:"scope"`
	Dimension    Dimension         `json:"dimension"`
	Subject      string            `json:"subject"`
	StateHash    string            `json:"state_hash"`
	PolicyState  PolicyState       `json:"policy_state"`
	Risk         Risk              `json:"risk"`
	EvidenceRefs []trace.Ref       `json:"evidence_refs,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Conflict struct {
	ExistingLockID string      `json:"existing_lock_id,omitempty"`
	Scope          Scope       `json:"scope"`
	Dimension      Dimension   `json:"dimension"`
	Subject        string      `json:"subject"`
	ExistingHash   string      `json:"existing_hash,omitempty"`
	ProposedHash   string      `json:"proposed_hash,omitempty"`
	Severity       Severity    `json:"severity"`
	ReasonCode     string      `json:"reason_code"`
	EvidenceRefs   []trace.Ref `json:"evidence_refs,omitempty"`
	Remediation    []string    `json:"remediation,omitempty"`
}

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
	SeverityUnknown  Severity = "unknown"
)

const (
	ReasonStateHashMismatch        = "state_hash_mismatch"
	ReasonSensitiveWithoutPolicy   = "sensitive_operation_without_policy"
	ReasonPolicyAbsentNonLowRisk   = "policy_absent_non_low_risk"
	ReasonValidationDowngrade      = "validation_result_downgrade"
	ReasonCostBudgetExceeded       = "cost_budget_exceeded"
	ReasonResearchClaimHashChanged = "research_claim_hash_changed"
	ReasonPermissionScopeWidened   = "permission_scope_widened"
)

func (p ProposedState) Normalized() ProposedState {
	out := p
	out.Scope = NormalizeScope(string(out.Scope))
	out.Dimension = NormalizeDimension(string(out.Dimension))
	out.Subject = trace.RedactString(strings.TrimSpace(out.Subject), maxSubjectLen)
	out.StateHash = trace.RedactString(strings.TrimSpace(out.StateHash), maxStateHashLen)
	out.PolicyState = normalizePolicyState(string(out.PolicyState))
	out.Risk = normalizeRisk(string(out.Risk))
	out.EvidenceRefs = normalizeRefs(out.EvidenceRefs)
	out.Metadata = normalizeMetadata(out.Metadata)
	return out
}

func (p ProposedState) Validate() error {
	n := p.Normalized()
	if n.Scope == ScopeUnknown || n.Dimension == DimensionUnknown {
		return fmt.Errorf("%w: unknown scope/dimension", ErrInvalidProposedState)
	}
	if n.Subject == "" || n.StateHash == "" {
		return fmt.Errorf("%w: missing subject/state_hash", ErrInvalidProposedState)
	}
	if n.Risk == RiskUnknown {
		return fmt.Errorf("%w: unknown risk", ErrInvalidProposedState)
	}
	return nil
}

func DetectConflicts(existing []Lock, proposed ProposedState) []Conflict {
	p := proposed.Normalized()
	conflicts := make([]Conflict, 0)

	if IsPolicyAbsent(p.PolicyState) && p.Risk != RiskLow {
		severity := SeverityMedium
		reason := ReasonPolicyAbsentNonLowRisk
		if p.Risk == RiskSensitive {
			severity = SeverityCritical
			reason = ReasonSensitiveWithoutPolicy
		}
		conflicts = append(conflicts, newConflict(Conflict{
			Scope:        p.Scope,
			Dimension:    p.Dimension,
			Subject:      p.Subject,
			ProposedHash: p.StateHash,
			Severity:     severity,
			ReasonCode:   reason,
			EvidenceRefs: p.EvidenceRefs,
			Remediation: []string{
				"provide matching policy",
				"request explicit approval",
				"reduce operation risk",
				"abort operation",
			},
		}))
	}

	if p.Risk == RiskSensitive && !AllowsSensitiveOperation(p.PolicyState) {
		if !containsReason(conflicts, ReasonSensitiveWithoutPolicy) {
			conflicts = append(conflicts, newConflict(Conflict{
				Scope:        p.Scope,
				Dimension:    p.Dimension,
				Subject:      p.Subject,
				ProposedHash: p.StateHash,
				Severity:     SeverityCritical,
				ReasonCode:   ReasonSensitiveWithoutPolicy,
				EvidenceRefs: p.EvidenceRefs,
				Remediation: []string{
					"provide matching policy",
					"request explicit approval",
					"reduce operation risk",
					"abort operation",
				},
			}))
		}
	}

	for i := range existing {
		l := existing[i].Normalized()
		if l.Status != StatusActive {
			continue
		}
		if l.Scope != p.Scope || l.Dimension != p.Dimension || l.Subject != p.Subject {
			continue
		}

		if l.StateHash != p.StateHash {
			conflicts = append(conflicts, newConflict(Conflict{
				ExistingLockID: l.ID,
				Scope:          p.Scope,
				Dimension:      p.Dimension,
				Subject:        p.Subject,
				ExistingHash:   l.StateHash,
				ProposedHash:   p.StateHash,
				Severity:       SeverityHigh,
				ReasonCode:     ReasonStateHashMismatch,
				EvidenceRefs:   combineRefs(l.EvidenceRefs, p.EvidenceRefs),
				Remediation: []string{
					"release or supersede existing lock",
					"change scope to avoid collision",
					"split task into compatible steps",
				},
			}))
		}

		if p.Dimension == DimensionPermissionScope && scopeWidened(l.StateHash, p.StateHash) {
			conflicts = append(conflicts, newConflict(Conflict{
				ExistingLockID: l.ID,
				Scope:          p.Scope,
				Dimension:      p.Dimension,
				Subject:        p.Subject,
				ExistingHash:   l.StateHash,
				ProposedHash:   p.StateHash,
				Severity:       SeverityHigh,
				ReasonCode:     ReasonPermissionScopeWidened,
				EvidenceRefs:   combineRefs(l.EvidenceRefs, p.EvidenceRefs),
				Remediation: []string{
					"request approval for widened scope",
					"reduce permission scope",
					"abort operation",
				},
			}))
		}

		if p.Dimension == DimensionValidationResult && validationDowngrade(l.StateHash, p.StateHash, p.Metadata) {
			conflicts = append(conflicts, newConflict(Conflict{
				ExistingLockID: l.ID,
				Scope:          p.Scope,
				Dimension:      p.Dimension,
				Subject:        p.Subject,
				ExistingHash:   l.StateHash,
				ProposedHash:   p.StateHash,
				Severity:       SeverityCritical,
				ReasonCode:     ReasonValidationDowngrade,
				EvidenceRefs:   combineRefs(l.ValidationRefs, p.EvidenceRefs),
				Remediation: []string{
					"attach explicit override receipt",
					"run additional validation",
					"abort downgrade",
				},
			}))
		}

		if p.Dimension == DimensionCostBudget && costBudgetExceeded(l.StateHash, p.StateHash) {
			conflicts = append(conflicts, newConflict(Conflict{
				ExistingLockID: l.ID,
				Scope:          p.Scope,
				Dimension:      p.Dimension,
				Subject:        p.Subject,
				ExistingHash:   l.StateHash,
				ProposedHash:   p.StateHash,
				Severity:       SeverityMedium,
				ReasonCode:     ReasonCostBudgetExceeded,
				EvidenceRefs:   combineRefs(l.EvidenceRefs, p.EvidenceRefs),
				Remediation: []string{
					"reduce budget",
					"request approval for budget increase",
				},
			}))
		}

		if p.Dimension == DimensionResearchClaim && researchClaimChangedAfterValidation(l, p.StateHash) {
			conflicts = append(conflicts, newConflict(Conflict{
				ExistingLockID: l.ID,
				Scope:          p.Scope,
				Dimension:      p.Dimension,
				Subject:        p.Subject,
				ExistingHash:   l.StateHash,
				ProposedHash:   p.StateHash,
				Severity:       SeverityHigh,
				ReasonCode:     ReasonResearchClaimHashChanged,
				EvidenceRefs:   combineRefs(l.ValidationRefs, p.EvidenceRefs),
				Remediation: []string{
					"re-validate research claim",
					"provide new policy evidence",
					"split claim into separate lock scope",
				},
			}))
		}
	}

	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Severity == conflicts[j].Severity {
			if conflicts[i].ReasonCode == conflicts[j].ReasonCode {
				return conflicts[i].ExistingLockID < conflicts[j].ExistingLockID
			}
			return conflicts[i].ReasonCode < conflicts[j].ReasonCode
		}
		return severityRank(conflicts[i].Severity) > severityRank(conflicts[j].Severity)
	})

	return conflicts
}

func containsReason(conflicts []Conflict, reason string) bool {
	for i := range conflicts {
		if conflicts[i].ReasonCode == reason {
			return true
		}
	}
	return false
}

func newConflict(c Conflict) Conflict {
	out := c
	out.Scope = NormalizeScope(string(out.Scope))
	out.Dimension = NormalizeDimension(string(out.Dimension))
	out.Subject = trace.RedactString(strings.TrimSpace(out.Subject), maxSubjectLen)
	out.ExistingLockID = trace.RedactString(strings.TrimSpace(out.ExistingLockID), maxIDLen)
	out.ExistingHash = trace.RedactString(strings.TrimSpace(out.ExistingHash), maxStateHashLen)
	out.ProposedHash = trace.RedactString(strings.TrimSpace(out.ProposedHash), maxStateHashLen)
	out.Severity = normalizeSeverity(out.Severity)
	out.ReasonCode = trace.RedactString(strings.ToLower(strings.TrimSpace(out.ReasonCode)), 128)
	out.EvidenceRefs = normalizeRefs(out.EvidenceRefs)
	out.Remediation = boundStringList(out.Remediation, 16, 160)
	return out
}

func normalizeSeverity(s Severity) Severity {
	switch Severity(strings.ToLower(strings.TrimSpace(string(s)))) {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return Severity(strings.ToLower(strings.TrimSpace(string(s))))
	default:
		return SeverityUnknown
	}
}

func severityRank(s Severity) int {
	switch normalizeSeverity(s) {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

func combineRefs(a, b []trace.Ref) []trace.Ref {
	merged := append(cloneRefs(a), b...)
	return normalizeRefs(merged)
}

func validationDowngrade(existingHash, proposedHash string, metadata map[string]string) bool {
	if strings.EqualFold(strings.TrimSpace(metadata["override"]), "true") {
		return false
	}
	existing := strings.ToLower(strings.TrimSpace(existingHash))
	proposed := strings.ToLower(strings.TrimSpace(proposedHash))
	return strings.Contains(existing, "pass") && strings.Contains(proposed, "fail")
}

func parseBudget(raw string) (float64, bool) {
	r := strings.TrimSpace(strings.ToLower(raw))
	r = strings.TrimPrefix(r, "budget:")
	r = strings.TrimPrefix(r, "usd:")
	if r == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(r, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func costBudgetExceeded(existingHash, proposedHash string) bool {
	existingBudget, okExisting := parseBudget(existingHash)
	proposedBudget, okProposed := parseBudget(proposedHash)
	if !okExisting || !okProposed {
		return false
	}
	return proposedBudget > existingBudget
}

func researchClaimChangedAfterValidation(lock Lock, proposedHash string) bool {
	if !strings.EqualFold(strings.TrimSpace(lock.Metadata["validated"]), "true") {
		return false
	}
	return strings.TrimSpace(lock.StateHash) != strings.TrimSpace(proposedHash)
}

func scopeWidened(existingHash, proposedHash string) bool {
	return permissionRank(proposedHash) > permissionRank(existingHash)
}

func permissionRank(raw string) int {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "scope:")
	s = strings.TrimPrefix(s, "perm:")
	switch s {
	case "read":
		return 1
	case "write":
		return 2
	case "admin":
		return 3
	default:
		return 0
	}
}
