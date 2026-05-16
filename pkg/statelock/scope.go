package statelock

import "strings"

type Scope string

type Dimension string

type Status string

type Source string

const (
	ScopeSession    Scope = "session"
	ScopeTrajectory Scope = "trajectory"
	ScopeArtifact   Scope = "artifact"
	ScopeWorkspace  Scope = "workspace"
	ScopeRepository Scope = "repository"
	ScopeRelease    Scope = "release"
	ScopeUnknown    Scope = "unknown"
)

const (
	DimensionArtifact          Dimension = "artifact"
	DimensionDecision          Dimension = "decision"
	DimensionPermissionScope   Dimension = "permission_scope"
	DimensionProviderSelection Dimension = "provider_selection"
	DimensionResearchClaim     Dimension = "research_claim"
	DimensionFilePatch         Dimension = "file_patch"
	DimensionValidationResult  Dimension = "validation_result"
	DimensionCostBudget        Dimension = "cost_budget"
	DimensionPolicyState       Dimension = "policy_state"
	DimensionUnknown           Dimension = "unknown"
)

const (
	StatusActive     Status = "active"
	StatusExpired    Status = "expired"
	StatusSuperseded Status = "superseded"
	StatusReleased   Status = "released"
	StatusInvalid    Status = "invalid"
)

const (
	SourceTrace      Source = "trace"
	SourceReplay     Source = "replay"
	SourceHarness    Source = "harness"
	SourceGovernance Source = "governance"
	SourceManual     Source = "manual"
	SourceUnknown    Source = "unknown"
)

var validScopes = map[Scope]struct{}{
	ScopeSession: {}, ScopeTrajectory: {}, ScopeArtifact: {}, ScopeWorkspace: {},
	ScopeRepository: {}, ScopeRelease: {}, ScopeUnknown: {},
}

var validDimensions = map[Dimension]struct{}{
	DimensionArtifact: {}, DimensionDecision: {}, DimensionPermissionScope: {},
	DimensionProviderSelection: {}, DimensionResearchClaim: {}, DimensionFilePatch: {},
	DimensionValidationResult: {}, DimensionCostBudget: {}, DimensionPolicyState: {},
	DimensionUnknown: {},
}

var validStatuses = map[Status]struct{}{
	StatusActive: {}, StatusExpired: {}, StatusSuperseded: {}, StatusReleased: {}, StatusInvalid: {},
}

func NormalizeScope(raw string) Scope {
	s := Scope(strings.ToLower(strings.TrimSpace(raw)))
	if _, ok := validScopes[s]; !ok {
		return ScopeUnknown
	}
	return s
}

func NormalizeDimension(raw string) Dimension {
	d := Dimension(strings.ToLower(strings.TrimSpace(raw)))
	if _, ok := validDimensions[d]; !ok {
		return DimensionUnknown
	}
	return d
}

func normalizeStatus(raw string) Status {
	s := Status(strings.ToLower(strings.TrimSpace(raw)))
	if _, ok := validStatuses[s]; !ok {
		return StatusInvalid
	}
	return s
}

func normalizeSource(raw string) Source {
	s := Source(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case SourceTrace, SourceReplay, SourceHarness, SourceGovernance, SourceManual:
		return s
	default:
		return SourceUnknown
	}
}
