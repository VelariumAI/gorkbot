package evidence

import (
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestRecordValidationAndStableID(t *testing.T) {
	r := Record{
		Kind:        KindHarnessReport,
		Status:      StatusPass,
		Subject:     "subject-a",
		Summary:     "summary-a",
		PolicyState: PolicyMatched,
		Risk:        RiskLow,
		Authority:   AuthorityPolicyMatch,
	}
	n1 := r.Normalized()
	n2 := r.Normalized()
	if n1.ID == "" || n1.ID != n2.ID {
		t.Fatalf("expected deterministic non-empty id, got %q and %q", n1.ID, n2.ID)
	}
	if err := n1.Validate(); err != nil {
		t.Fatalf("record should validate: %v", err)
	}
}

func TestRecordMetadataBoundsAndRedaction(t *testing.T) {
	meta := map[string]string{
		"token": "secret-token",
		"safe":  "ok",
	}
	r := Record{
		Kind:     KindTraceEvent,
		Status:   StatusWarn,
		Subject:  "s",
		Summary:  strings.Repeat("a", maxSummaryLen+20),
		Source:   strings.Repeat("b", maxSourceLen+20),
		Metadata: meta,
		EvidenceRefs: []trace.Ref{
			{Kind: "k", Ref: "r", Hash: "h", SizeBytes: -1},
		},
	}
	n := r.Normalized()
	if got := n.Metadata["token"]; got != "[REDACTED]" {
		t.Fatalf("expected token redacted, got %q", got)
	}
	if len(n.Summary) != maxSummaryLen {
		t.Fatalf("expected bounded summary length %d, got %d", maxSummaryLen, len(n.Summary))
	}
	if len(n.Source) != maxSourceLen {
		t.Fatalf("expected bounded source length %d, got %d", maxSourceLen, len(n.Source))
	}
	if len(n.EvidenceRefs) != 1 || n.EvidenceRefs[0].SizeBytes != 0 {
		t.Fatalf("expected normalized refs with non-negative sizes, got %+v", n.EvidenceRefs)
	}
}

func TestRecordInvalidFieldsRejected(t *testing.T) {
	if err := (Record{}).Validate(); err == nil {
		t.Fatal("empty record should fail validation")
	}
}
