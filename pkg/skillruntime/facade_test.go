package skillruntime

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/statelock"
)

func TestFacadeLifecycleMethods(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	f := Facade{Store: store}

	cfg := testConfigWithStageCapability()
	cand := Candidate{Name: "skill", Source: "local", Risk: evidence.RiskLow}

	prop, err := f.Propose(ctx, Request{Operation: OperationPropose, Candidate: cand, Config: cfg})
	if err != nil {
		t.Fatalf("propose error: %v", err)
	}
	if prop.CandidateID == "" {
		t.Fatalf("propose candidate id required")
	}

	retr, err := f.Retrieve(ctx, Request{Operation: OperationRetrieve, Candidate: Candidate{ID: prop.CandidateID}, Config: cfg})
	if err != nil {
		t.Fatalf("retrieve error: %v", err)
	}
	if retr.CandidateID != prop.CandidateID {
		t.Fatalf("retrieve candidate mismatch: got=%q want=%q", retr.CandidateID, prop.CandidateID)
	}

	val, err := f.Validate(ctx, Request{Operation: OperationValidate, Candidate: Candidate{ID: prop.CandidateID, Name: "skill", Source: "local"}, Config: cfg})
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if val.ID == "" {
		t.Fatalf("validate result id required")
	}

	stage, err := f.Stage(ctx, Request{Operation: OperationStage, Candidate: Candidate{ID: prop.CandidateID, Name: "skill", Source: "local"}, Config: cfg})
	if err != nil {
		t.Fatalf("stage error: %v", err)
	}
	if stage.CandidateID != prop.CandidateID {
		t.Fatalf("stage candidate mismatch")
	}

	dis, err := f.Disable(ctx, Request{Operation: OperationDisable, Candidate: Candidate{ID: prop.CandidateID, Name: "skill", Source: "local"}, Config: cfg})
	if err != nil {
		t.Fatalf("disable error: %v", err)
	}
	if dis.Status != StatusDisabled {
		t.Fatalf("disable status mismatch: %q", dis.Status)
	}
}

func TestPromoteNoRuntimeMutationOrAutomaticPromotionExecution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	f := Facade{Store: store}

	cfg := testConfigWithStageCapability()
	cfg.Automation.AutoPromotionMode = profile.AutomationAllowConfigured

	cand := Candidate{Name: "skill", Source: "local", Risk: evidence.RiskMedium}
	if err := store.SaveCandidate(ctx, cand); err != nil {
		t.Fatalf("save candidate: %v", err)
	}
	candID := cand.Normalized().ID

	res, err := f.Promote(ctx, Request{
		Operation:       OperationPromote,
		Candidate:       Candidate{ID: candID, Name: "skill", Source: "local", Risk: evidence.RiskMedium},
		Config:          cfg,
		HarnessReport:   &harness.Report{HarnessID: "h", ArtifactID: "a", Status: harness.StatusPass},
		ReplayResult:    &replay.Result{CaseID: "c", Verdict: replay.VerdictPass},
		StateLockResult: &statelock.CheckResult{Status: statelock.CheckStatusAllowed, PolicyState: statelock.PolicyMatched, Risk: statelock.RiskLow},
		Metadata: map[string]string{
			"rollback_path": "yes",
			"disable_path":  "yes",
		},
	})
	if err != nil {
		t.Fatalf("promote error: %v", err)
	}
	if res.Status != StatusStaged && res.Status != StatusApprovalRequired && res.Status != StatusConfigRequired {
		t.Fatalf("promote must not execute mutation, got status=%q", res.Status)
	}

	loaded, err := store.LoadCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("load candidate: %v", err)
	}
	if loaded.Disabled {
		t.Fatalf("promote should not disable candidate")
	}
}

func TestFacadeRunDispatch(t *testing.T) {
	ctx := context.Background()
	f := Facade{Store: NewMemoryStore()}
	cfg := testConfigWithStageCapability()

	cases := []Operation{OperationRetrieve, OperationPropose, OperationValidate, OperationStage, OperationPromote, OperationDisable}
	for _, op := range cases {
		_, _ = f.Run(ctx, Request{Operation: op, Candidate: Candidate{Name: "x", Source: "s"}, Config: cfg})
	}

	if _, err := f.Run(ctx, Request{Operation: OperationUnknown, Candidate: Candidate{Name: "x", Source: "s"}, Config: cfg}); err == nil {
		t.Fatalf("expected unknown op error")
	}
}
