package statelock

import (
	"testing"
	"time"
)

func baseLock(d Dimension, hash string) Lock {
	return Lock{
		ID:          "base-lock",
		Scope:       ScopeWorkspace,
		Dimension:   d,
		Subject:     "subject-a",
		StateHash:   hash,
		Status:      StatusActive,
		PolicyState: PolicyMatched,
		CreatedAt:   time.Now().UTC(),
	}
}

func TestConflictSameSubjectDifferentHash(t *testing.T) {
	lock := baseLock(DimensionArtifact, "h1")
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "subject-a",
		StateHash:   "h2",
		PolicyState: PolicyMatched,
		Risk:        RiskMedium,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if len(conflicts) == 0 {
		t.Fatal("expected conflict")
	}
	if conflicts[0].ReasonCode != ReasonStateHashMismatch {
		t.Fatalf("unexpected reason: %s", conflicts[0].ReasonCode)
	}
}

func TestNoConflictSameHash(t *testing.T) {
	lock := baseLock(DimensionArtifact, "h1")
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "subject-a",
		StateHash:   "h1",
		PolicyState: PolicyMatched,
		Risk:        RiskLow,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestPermissionScopeWideningConflict(t *testing.T) {
	lock := baseLock(DimensionPermissionScope, "read")
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionPermissionScope,
		Subject:     "subject-a",
		StateHash:   "admin",
		PolicyState: PolicyMatched,
		Risk:        RiskSensitive,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if !containsReason(conflicts, ReasonPermissionScopeWidened) {
		t.Fatalf("expected permission widening conflict: %#v", conflicts)
	}
}

func TestValidationDowngradeConflict(t *testing.T) {
	lock := baseLock(DimensionValidationResult, "pass")
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionValidationResult,
		Subject:     "subject-a",
		StateHash:   "fail",
		PolicyState: PolicyMatched,
		Risk:        RiskSensitive,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if !containsReason(conflicts, ReasonValidationDowngrade) {
		t.Fatalf("expected downgrade conflict: %#v", conflicts)
	}
}

func TestCostBudgetConflict(t *testing.T) {
	lock := baseLock(DimensionCostBudget, "budget:10")
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionCostBudget,
		Subject:     "subject-a",
		StateHash:   "budget:20",
		PolicyState: PolicyMatched,
		Risk:        RiskMedium,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if !containsReason(conflicts, ReasonCostBudgetExceeded) {
		t.Fatalf("expected cost budget conflict: %#v", conflicts)
	}
}

func TestResearchClaimHashConflict(t *testing.T) {
	lock := baseLock(DimensionResearchClaim, "hash-old")
	lock.Metadata = map[string]string{"validated": "true"}
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionResearchClaim,
		Subject:     "subject-a",
		StateHash:   "hash-new",
		PolicyState: PolicyMatched,
		Risk:        RiskMedium,
	}
	conflicts := DetectConflicts([]Lock{lock}, proposed)
	if !containsReason(conflicts, ReasonResearchClaimHashChanged) {
		t.Fatalf("expected research claim conflict: %#v", conflicts)
	}
}

func TestSensitiveOperationWithoutPolicyConflict(t *testing.T) {
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionDecision,
		Subject:     "subject-a",
		StateHash:   "x",
		PolicyState: PolicyNoMatch,
		Risk:        RiskSensitive,
	}
	conflicts := DetectConflicts(nil, proposed)
	if !containsReason(conflicts, ReasonSensitiveWithoutPolicy) {
		t.Fatalf("expected sensitive-without-policy conflict: %#v", conflicts)
	}
}

func TestPolicyAbsentLowRiskOnlyAllowed(t *testing.T) {
	low := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionDecision,
		Subject:     "subject-a",
		StateHash:   "x",
		PolicyState: PolicyNoMatch,
		Risk:        RiskLow,
	}
	if conflicts := DetectConflicts(nil, low); len(conflicts) != 0 {
		t.Fatalf("expected no conflict for explicit low risk, got %#v", conflicts)
	}

	medium := low
	medium.Risk = RiskMedium
	conflicts := DetectConflicts(nil, medium)
	if !containsReason(conflicts, ReasonPolicyAbsentNonLowRisk) {
		t.Fatalf("expected absent-policy conflict for medium risk: %#v", conflicts)
	}
}
