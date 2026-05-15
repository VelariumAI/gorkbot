package harness

// AuditSummary is a bounded runtime-facing digest of a harness report.
type AuditSummary struct {
	Mode        Mode   `json:"mode"`
	Status      Status `json:"status"`
	ReportID    string `json:"report_id,omitempty"`
	HarnessID   string `json:"harness_id,omitempty"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	FailedCount int    `json:"failed_count,omitempty"`
	WarnCount   int    `json:"warn_count,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
}

// SummarizeReport converts a report into a bounded summary for receipts/events.
func SummarizeReport(mode Mode, report Report) AuditSummary {
	norm := report.Normalized()
	out := AuditSummary{
		Mode:       ParseMode(string(mode)),
		Status:     norm.Status,
		ReportID:   truncateString(norm.StableID(), 128),
		HarnessID:  truncateString(norm.HarnessID, 128),
		ArtifactID: truncateString(norm.ArtifactID, maxArtifactIDLen),
		DurationMS: norm.Duration.Milliseconds(),
	}
	for i := range norm.Results {
		switch norm.Results[i].Status {
		case StatusWarn:
			out.WarnCount++
		case StatusFail, StatusInvalid:
			out.FailedCount++
		}
	}
	if out.DurationMS < 0 {
		out.DurationMS = 0
	}
	return out
}
