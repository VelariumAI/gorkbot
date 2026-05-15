package harness

import (
	"strings"
	"testing"
)

func TestReportEvidenceAndRemediationBounding(t *testing.T) {
	evidence := make([]Evidence, 0, maxReportEvidenceEntries+5)
	for i := 0; i < maxReportEvidenceEntries+5; i++ {
		evidence = append(evidence, Evidence{
			Kind:  "kind",
			Value: strings.Repeat("x", maxReportEvidenceValueLen+25),
		})
	}

	remediation := make([]string, 0, maxReportRemediationEntries+5)
	for i := 0; i < maxReportRemediationEntries+5; i++ {
		remediation = append(remediation, strings.Repeat("r", maxAssertionMessageLen+20))
	}

	r := Report{
		HarnessID:   "h-1",
		ArtifactID:  "art-1",
		Status:      StatusWarn,
		Evidence:    evidence,
		Remediation: remediation,
	}
	norm := r.Normalized()

	if len(norm.Evidence) > maxReportEvidenceEntries {
		t.Fatalf("expected bounded evidence, got %d", len(norm.Evidence))
	}
	if len(norm.Evidence[0].Value) > maxReportEvidenceValueLen {
		t.Fatalf("expected bounded evidence value length, got %d", len(norm.Evidence[0].Value))
	}
	if len(norm.Remediation) > maxReportRemediationEntries {
		t.Fatalf("expected bounded remediation, got %d", len(norm.Remediation))
	}
	if len(norm.Remediation[0]) > maxAssertionMessageLen {
		t.Fatalf("expected bounded remediation entry")
	}
}

func TestReportValidationRef(t *testing.T) {
	r := Report{
		HarnessID:  "h-1",
		ArtifactID: "a-1",
		Status:     StatusPass,
		Results: []Result{
			{AssertionID: "a", Status: StatusPass},
			{AssertionID: "b", Status: StatusWarn, ReasonCode: "warn"},
		},
	}
	ref := r.ValidationRef()
	if ref.Kind != "harness_report" {
		t.Fatalf("expected harness_report kind, got %q", ref.Kind)
	}
	if ref.Ref == "" {
		t.Fatalf("expected non-empty validation ref")
	}
	if ref.Hash == "" {
		t.Fatalf("expected non-empty validation hash")
	}
}
