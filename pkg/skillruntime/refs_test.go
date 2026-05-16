package skillruntime

import (
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestCandidateRefStableAndBounded(t *testing.T) {
	cand := Candidate{
		Name:   strings.Repeat("n", 220),
		Source: "s",
		Risk:   evidence.RiskLow,
		ArtifactRefs: []trace.Ref{
			trace.NewRef("artifact", strings.Repeat("r", 800), strings.Repeat("h", 600), 3),
		},
	}
	ref1 := CandidateRef(cand)
	ref2 := CandidateRef(cand)
	if ref1 != ref2 {
		t.Fatalf("candidate ref should be stable")
	}
	if ref1.Ref == "" || ref1.Hash == "" {
		t.Fatalf("candidate ref fields required: %+v", ref1)
	}
	if len(ref1.Ref) > 256 {
		t.Fatalf("candidate ref not bounded")
	}
}

func TestResultRefStableAndBounded(t *testing.T) {
	res := Result{
		Operation:   OperationStage,
		CandidateID: "cand-1",
		Status:      StatusStaged,
		Decision:    evidence.DecisionAuditOnly,
		Assessment: evidence.Assessment{
			PolicyState: evidence.PolicyMatched,
			Risk:        evidence.RiskMedium,
			Operation:   "skill_stage",
			Authority:   evidence.AuthorityPolicyMatch,
		},
		Receipt: evidence.Receipt{
			Status: evidence.StatusWarn,
			Assessment: evidence.Assessment{
				PolicyState: evidence.PolicyMatched,
				Risk:        evidence.RiskMedium,
				Operation:   "skill_stage",
				Authority:   evidence.AuthorityPolicyMatch,
			},
		},
		Warnings: []string{strings.Repeat("w", 400)},
	}
	ref1 := ResultRef(res)
	ref2 := ResultRef(res)
	if ref1 != ref2 {
		t.Fatalf("result ref should be stable")
	}
	if ref1.Ref == "" || ref1.Hash == "" {
		t.Fatalf("result ref fields required: %+v", ref1)
	}
	if len(ref1.Ref) > 256 {
		t.Fatalf("result ref not bounded")
	}
}
