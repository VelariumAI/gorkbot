package harness

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeReportBoundsAndCounts(t *testing.T) {
	start := time.Now().UTC()
	report := Report{
		HarnessID:  strings.Repeat("h", 220),
		ArtifactID: strings.Repeat("a", 220),
		Status:     StatusFail,
		StartedAt:  start,
		FinishedAt: start.Add(25 * time.Millisecond),
		Results: []Result{
			{Status: StatusWarn},
			{Status: StatusFail},
			{Status: StatusInvalid},
		},
	}

	summary := SummarizeReport(ModeAudit, report)
	if summary.Mode != ModeAudit {
		t.Fatalf("expected mode audit, got %q", summary.Mode)
	}
	if summary.WarnCount != 1 {
		t.Fatalf("expected 1 warning, got %d", summary.WarnCount)
	}
	if summary.FailedCount != 2 {
		t.Fatalf("expected 2 failed entries, got %d", summary.FailedCount)
	}
	if summary.DurationMS != 25 {
		t.Fatalf("expected 25ms duration, got %d", summary.DurationMS)
	}
	if len(summary.HarnessID) > 128 {
		t.Fatalf("harness id not bounded: %d", len(summary.HarnessID))
	}
	if len(summary.ArtifactID) > maxArtifactIDLen {
		t.Fatalf("artifact id not bounded: %d", len(summary.ArtifactID))
	}
	if summary.ReportID == "" {
		t.Fatalf("expected stable report id")
	}
}
