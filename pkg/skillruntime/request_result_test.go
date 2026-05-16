package skillruntime

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/statelock"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestRequestNormalizedAndValidate(t *testing.T) {
	req := Request{
		Operation: Operation(" StAgE "),
		Candidate: Candidate{Name: "x", Source: "local", Risk: evidence.RiskMedium},
		Config: func() profile.Config {
			cfg := profile.DefaultConfig(profile.ProfilePowerUser)
			cfg.ConfiguredCapabilities = map[profile.Capability]bool{profile.CapabilitySkillStage: true}
			return cfg
		}(),
		DirectiveHash: "  abc  ",
		FailureHash:   "  def  ",
		ReplayResult: &replay.Result{
			CaseID:   "case-1",
			Verdict:  replay.VerdictPass,
			Duration: -1,
		},
		HarnessReport: &harness.Report{HarnessID: "h", ArtifactID: "a", Status: harness.StatusPass},
		StateLockResult: &statelock.CheckResult{
			Status:      statelock.CheckStatusAllowed,
			PolicyState: statelock.PolicyMatched,
			Risk:        statelock.RiskLow,
		},
		EvidenceRefs: []trace.Ref{trace.NewRef("x", "r", "h", 1)},
		Metadata: map[string]string{
			"ok":             "value",
			"secret":         "hidden",
			"command_output": "strip-me",
		},
	}

	n := req.Normalized()
	if n.Operation != OperationStage {
		t.Fatalf("operation normalize failed: %q", n.Operation)
	}
	if n.Candidate.ID == "" {
		t.Fatalf("candidate id should be set during normalization")
	}
	if n.Config.Profile == "" {
		t.Fatalf("config should be normalized")
	}
	if n.DirectiveHash != "abc" || n.FailureHash != "def" {
		t.Fatalf("hash bounds/trim failed: %q %q", n.DirectiveHash, n.FailureHash)
	}
	if n.Metadata["secret"] != "[REDACTED]" {
		t.Fatalf("metadata secret should redact")
	}
	if _, ok := n.Metadata["command_output"]; ok {
		t.Fatalf("command_output should be stripped")
	}

	if err := n.Validate(); err != nil {
		t.Fatalf("request validate unexpected error: %v", err)
	}
}

func TestRequestValidateRejectsUnknownOperation(t *testing.T) {
	req := Request{Operation: OperationUnknown, Candidate: Candidate{Name: "x", Source: "s"}}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error for unknown operation")
	}
}

func TestResultNormalizedStableIDAndValidate(t *testing.T) {
	res := Result{
		Operation:   OperationPromote,
		CandidateID: "cand-1",
		Status:      StatusStaged,
		Decision:    evidence.DecisionRequireApproval,
		Assessment: evidence.Assessment{
			PolicyState: evidence.PolicyNoMatch,
			Risk:        evidence.RiskSensitive,
			Operation:   "skill_promote",
			Authority:   evidence.AuthorityHumanApproval,
		},
		Receipt: evidence.Receipt{
			Status: evidence.StatusWarn,
			Assessment: evidence.Assessment{
				PolicyState: evidence.PolicyNoMatch,
				Risk:        evidence.RiskSensitive,
				Operation:   "skill_promote",
				Authority:   evidence.AuthorityHumanApproval,
			},
		},
		ValidationRefs: []trace.Ref{trace.NewRef("v", "r", "h", 1)},
		ArtifactRefs:   []trace.Ref{trace.NewRef("a", "r", "h", 1)},
		Warnings:       []string{"w1", "w2"},
		Metadata:       map[string]string{"ok": "v", "token": "x"},
	}

	n := res.Normalized()
	if n.ID == "" {
		t.Fatalf("result stable id required")
	}
	if n.Status != StatusStaged {
		t.Fatalf("status normalize failed: %q", n.Status)
	}
	if n.Metadata["token"] != "[REDACTED]" {
		t.Fatalf("sensitive metadata should be redacted")
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("result validate unexpected error: %v", err)
	}

	n2 := res.Normalized()
	if n.ID != n2.ID {
		t.Fatalf("result id should be stable")
	}
}

func TestResultValidateRejectsInvalidStatus(t *testing.T) {
	res := Result{Operation: OperationRetrieve, CandidateID: "x", Status: StatusInvalid}
	if err := res.Validate(); err == nil {
		t.Fatalf("expected invalid status error")
	}
}
