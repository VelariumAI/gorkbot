package evidence

import (
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestReceiptValidationAndVisibility(t *testing.T) {
	r := Receipt{
		Records: []Record{{
			Kind:    KindPolicyAbsence,
			Status:  StatusWarn,
			Subject: "policy",
			Summary: "no matching policy",
		}},
		Assessment: Assessment{
			PolicyState: PolicyNoMatch,
			Risk:        RiskSensitive,
			Operation:   string(SensitiveShellExecution),
		},
		Status:     StatusWarn,
		Summary:    "policy gap",
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	n := r.Normalized()
	if n.Assessment.PolicyState != PolicyNoMatch {
		t.Fatalf("expected policy_no_match visible, got %s", n.Assessment.PolicyState)
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("receipt should validate: %v", err)
	}
}

func TestReceiptNoPassOnIndeterminateRecords(t *testing.T) {
	r := Receipt{
		Records: []Record{
			{Kind: KindTraceEvent, Status: StatusSkipped, Subject: "a", Summary: "s"},
			{Kind: KindTraceEvent, Status: StatusUnavailable, Subject: "b", Summary: "u"},
			{Kind: KindTraceEvent, Status: StatusInconclusive, Subject: "c", Summary: "i"},
		},
		Assessment: Assessment{PolicyState: PolicyNoMatch, Risk: RiskLow, Operation: "help", ExplicitLowRisk: true},
		Status:     StatusPass,
		Summary:    "should not remain pass",
	}
	n := r.Normalized()
	if n.Status == StatusPass {
		t.Fatalf("receipt with only indeterminate records must not remain pass")
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("normalized receipt should validate: %v", err)
	}
}

func TestReceiptRefsBoundedAndStable(t *testing.T) {
	records := make([]Record, 0, maxRecordCount+10)
	for i := 0; i < maxRecordCount+10; i++ {
		records = append(records, Record{Kind: KindTraceEvent, Status: StatusWarn, Subject: "x", Summary: "y"})
	}
	refs := make([]trace.Ref, 0, maxRefCount+5)
	for i := 0; i < maxRefCount+5; i++ {
		refs = append(refs, trace.NewRef("k", "r", "h", int64(i)))
	}
	r := Receipt{
		Records:      records,
		Assessment:   Assessment{PolicyState: PolicyMatched, Risk: RiskLow, Operation: "help", ExplicitLowRisk: true, Authority: AuthorityPolicyMatch},
		Status:       StatusWarn,
		Summary:      "bounded",
		EvidenceRefs: refs,
	}
	n1 := r.Normalized()
	n2 := r.Normalized()
	if len(n1.Records) != maxRecordCount {
		t.Fatalf("expected record cap %d, got %d", maxRecordCount, len(n1.Records))
	}
	if len(n1.EvidenceRefs) != maxRefCount {
		t.Fatalf("expected ref cap %d, got %d", maxRefCount, len(n1.EvidenceRefs))
	}
	if n1.ID == "" || n1.ID != n2.ID {
		t.Fatalf("expected stable id, got %q vs %q", n1.ID, n2.ID)
	}
}

func TestReceiptMalformedInputNoPanic(t *testing.T) {
	_ = (Receipt{}).Normalized()
	if err := (Receipt{}).Validate(); err != nil {
		// Acceptable; assertion is no panic.
	}
}

func TestReceiptAssessmentOnlyPassAllowed(t *testing.T) {
	r := Receipt{
		Assessment: Assessment{
			PolicyState:     PolicyMatched,
			Risk:            RiskLow,
			Operation:       "help",
			ExplicitLowRisk: true,
			Authority:       AuthorityPolicyMatch,
		},
		Status:     StatusPass,
		Summary:    "assessment-only pass",
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("assessment-only pass should validate: %v", err)
	}
}
