package evidence

import (
	"fmt"
	"strings"
)

type Kind string

const (
	KindTraceEvent         Kind = "trace_event"
	KindTrajectory         Kind = "trajectory"
	KindReplayResult       Kind = "replay_result"
	KindHarnessReport      Kind = "harness_report"
	KindValidationReport   Kind = "validation_report"
	KindStateLock          Kind = "state_lock"
	KindParadoxReport      Kind = "paradox_report"
	KindPolicyAbsence      Kind = "policy_absence"
	KindGovernanceDecision Kind = "governance_decision"
	KindResearchReceipt    Kind = "research_receipt"
	KindSelfmodReceipt     Kind = "selfmod_receipt"
	KindToolAction         Kind = "tool_action"
	KindReleaseGate        Kind = "release_gate"
	KindUserApproval       Kind = "user_approval"
	KindHardInvariant      Kind = "hard_invariant"
	KindUnknown            Kind = "unknown"
)

func NormalizeKind(raw string) Kind {
	k := Kind(strings.ToLower(strings.TrimSpace(raw)))
	switch k {
	case KindTraceEvent, KindTrajectory, KindReplayResult, KindHarnessReport,
		KindValidationReport, KindStateLock, KindParadoxReport, KindPolicyAbsence,
		KindGovernanceDecision, KindResearchReceipt, KindSelfmodReceipt,
		KindToolAction, KindReleaseGate, KindUserApproval, KindHardInvariant,
		KindUnknown:
		return k
	case "":
		return KindUnknown
	default:
		return KindUnknown
	}
}

func (k Kind) Validate() error {
	if NormalizeKind(string(k)) == KindUnknown {
		return fmt.Errorf("%w: %q", ErrInvalidKind, k)
	}
	return nil
}
