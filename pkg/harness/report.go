package harness

import (
	"sort"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxReportEvidenceEntries    = 32
	maxReportEvidenceKindLen    = 32
	maxReportEvidenceValueLen   = 256
	maxReportRemediationEntries = 32
)

type Status string

const (
	StatusPass         Status = "pass"
	StatusWarn         Status = "warn"
	StatusFail         Status = "fail"
	StatusUnsupported  Status = "unsupported"
	StatusInvalid      Status = "invalid"
	StatusInconclusive Status = "inconclusive"
)

type Evidence struct {
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value,omitempty"`
}

type Result struct {
	AssertionID string     `json:"assertion_id"`
	Status      Status     `json:"status"`
	Severity    Severity   `json:"severity"`
	Message     string     `json:"message,omitempty"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	Remediation []string   `json:"remediation,omitempty"`
	ReasonCode  string     `json:"reason_code,omitempty"`
}

// Report records a deterministic validation run for a single artifact.
type Report struct {
	HarnessID   string            `json:"harness_id"`
	ArtifactID  string            `json:"artifact_id"`
	Status      Status            `json:"status"`
	Results     []Result          `json:"results,omitempty"`
	Evidence    []Evidence        `json:"evidence,omitempty"`
	ErrorTrace  string            `json:"error_trace,omitempty"`
	Remediation []string          `json:"remediation,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Duration    time.Duration     `json:"duration"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func NewReport(harnessID, artifactID string) Report {
	start := time.Now().UTC()
	return Report{
		HarnessID:  truncateString(strings.TrimSpace(harnessID), 128),
		ArtifactID: truncateString(strings.TrimSpace(artifactID), maxArtifactIDLen),
		Status:     StatusInconclusive,
		StartedAt:  start,
		FinishedAt: start,
	}
}

func (r Report) Normalized() Report {
	out := r
	out.HarnessID = truncateString(strings.TrimSpace(out.HarnessID), 128)
	out.ArtifactID = truncateString(strings.TrimSpace(out.ArtifactID), maxArtifactIDLen)
	out.ErrorTrace = truncateString(strings.TrimSpace(out.ErrorTrace), maxAssertionConditionLen)
	out.Metadata = trace.BoundMetadata(out.Metadata)
	out.Evidence = boundEvidence(out.Evidence)
	out.Remediation = boundStringList(out.Remediation, maxReportRemediationEntries, maxAssertionMessageLen)
	out.Results = boundResults(out.Results)
	if out.StartedAt.IsZero() {
		out.StartedAt = time.Now().UTC()
	}
	if out.FinishedAt.IsZero() || out.FinishedAt.Before(out.StartedAt) {
		out.FinishedAt = out.StartedAt
	}
	out.Duration = out.FinishedAt.Sub(out.StartedAt)
	if out.Duration < 0 {
		out.Duration = 0
	}
	return out
}

func (r Report) StableID() string {
	norm := r.Normalized()
	parts := []string{norm.HarnessID, norm.ArtifactID, string(norm.Status)}
	for i := range norm.Results {
		parts = append(parts, norm.Results[i].AssertionID, string(norm.Results[i].Status), norm.Results[i].ReasonCode)
	}
	return trace.StableHash(parts...)
}

func (r Report) ValidationRef() trace.Ref {
	norm := r.Normalized()
	return trace.NewRef(
		"harness_report",
		"harness:"+norm.StableID(),
		norm.StableID(),
		int64(len(norm.Results)),
	)
}

func boundResults(in []Result) []Result {
	if len(in) == 0 {
		return nil
	}
	out := make([]Result, 0, len(in))
	for i := range in {
		item := in[i]
		item.AssertionID = truncateString(strings.TrimSpace(item.AssertionID), maxAssertionIDLen)
		item.Message = truncateString(strings.TrimSpace(item.Message), maxAssertionMessageLen)
		item.ReasonCode = truncateString(strings.TrimSpace(item.ReasonCode), maxReasonCodeLen)
		item.Evidence = boundEvidence(item.Evidence)
		item.Remediation = boundStringList(item.Remediation, maxAssertionRemediations, maxAssertionMessageLen)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AssertionID == out[j].AssertionID {
			return out[i].ReasonCode < out[j].ReasonCode
		}
		return out[i].AssertionID < out[j].AssertionID
	})
	return out
}

func boundEvidence(in []Evidence) []Evidence {
	if len(in) == 0 {
		return nil
	}
	if len(in) > maxReportEvidenceEntries {
		in = in[:maxReportEvidenceEntries]
	}
	out := make([]Evidence, 0, len(in))
	for i := range in {
		kind := truncateString(strings.TrimSpace(in[i].Kind), maxReportEvidenceKindLen)
		value := truncateString(strings.TrimSpace(in[i].Value), maxReportEvidenceValueLen)
		if value == "" {
			continue
		}
		out = append(out, Evidence{Kind: kind, Value: value})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
