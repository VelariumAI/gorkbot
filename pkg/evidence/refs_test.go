package evidence

import "testing"

func TestTraceRefHelpersStable(t *testing.T) {
	record := Record{Kind: KindTraceEvent, Status: StatusPass, Subject: "sub", Summary: "sum"}
	receipt := Receipt{
		Records:    []Record{record},
		Assessment: Assessment{PolicyState: PolicyMatched, Risk: RiskLow, Operation: "help", ExplicitLowRisk: true, Authority: AuthorityPolicyMatch},
		Status:     StatusWarn,
		Summary:    "receipt",
	}
	assessment := Assessment{PolicyState: PolicyNoMatch, Risk: RiskSensitive, Operation: string(SensitiveShellExecution)}

	r1 := RecordRef(record)
	r2 := RecordRef(record)
	if r1.Ref == "" || r1.Ref != r2.Ref || r1.Hash != r2.Hash {
		t.Fatalf("record refs should be stable: %+v %+v", r1, r2)
	}

	rc1 := ReceiptRef(receipt)
	rc2 := ReceiptRef(receipt)
	if rc1.Ref == "" || rc1.Ref != rc2.Ref || rc1.Hash != rc2.Hash {
		t.Fatalf("receipt refs should be stable: %+v %+v", rc1, rc2)
	}

	a1 := AssessmentRef(assessment)
	a2 := AssessmentRef(assessment)
	if a1.Ref == "" || a1.Ref != a2.Ref || a1.Hash != a2.Hash {
		t.Fatalf("assessment refs should be stable: %+v %+v", a1, a2)
	}
}
