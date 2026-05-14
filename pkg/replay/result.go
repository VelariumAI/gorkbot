package replay

import (
	"sort"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type Verdict string

const (
	VerdictPass         Verdict = "pass"
	VerdictFail         Verdict = "fail"
	VerdictRegression   Verdict = "regression"
	VerdictImprovement  Verdict = "improvement"
	VerdictInconclusive Verdict = "inconclusive"
	VerdictInvalid      Verdict = "invalid"
)

type Regression struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Improvement struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Outcome struct {
	TrajectoryID     string            `json:"trajectory_id,omitempty"`
	Status           string            `json:"status,omitempty"`
	Decision         string            `json:"decision,omitempty"`
	ErrorClass       string            `json:"error_class,omitempty"`
	Outcome          string            `json:"outcome,omitempty"`
	CostSummary      trace.CostSummary `json:"cost_summary,omitempty"`
	DurationMS       int64             `json:"duration_ms,omitempty"`
	OperatorPath     []trace.Operator  `json:"operator_path,omitempty"`
	EventKinds       []string          `json:"event_kinds,omitempty"`
	ValidationRefs   []trace.Ref       `json:"validation_refs,omitempty"`
	ReceiptRefs      []trace.Ref       `json:"receipt_refs,omitempty"`
	Retries          int               `json:"retries,omitempty"`
	RepairOperations int               `json:"repair_operations,omitempty"`
	Success          bool              `json:"success"`
	HasSuccessSignal bool              `json:"has_success_signal"`
	Invalid          bool              `json:"invalid"`
	InvalidReason    string            `json:"invalid_reason,omitempty"`
}

type CandidateOutcome struct {
	Trajectory trace.Trajectory `json:"trajectory"`
	Events     []trace.Event    `json:"events,omitempty"`
}

type Result struct {
	CaseID           string        `json:"case_id"`
	TrajectoryID     string        `json:"trajectory_id,omitempty"`
	CandidateID      string        `json:"candidate_id,omitempty"`
	BaselineOutcome  Outcome       `json:"baseline_outcome"`
	CandidateOutcome Outcome       `json:"candidate_outcome"`
	Regressions      []Regression  `json:"regressions,omitempty"`
	Improvements     []Improvement `json:"improvements,omitempty"`
	Verdict          Verdict       `json:"verdict"`
	StartedAt        time.Time     `json:"started_at"`
	FinishedAt       time.Time     `json:"finished_at"`
	Duration         time.Duration `json:"duration"`
}

func normalizeOutcome(o Outcome) Outcome {
	out := o
	out.TrajectoryID = truncate(strings.TrimSpace(out.TrajectoryID), 256)
	out.Status = truncate(strings.ToLower(strings.TrimSpace(out.Status)), 64)
	out.Decision = truncate(strings.ToLower(strings.TrimSpace(out.Decision)), 64)
	out.ErrorClass = truncate(strings.ToLower(strings.TrimSpace(out.ErrorClass)), 64)
	out.Outcome = truncate(strings.ToLower(strings.TrimSpace(out.Outcome)), 256)
	out.EventKinds = normalizeKinds(out.EventKinds)
	out.OperatorPath = normalizeOperators(out.OperatorPath)
	if out.DurationMS < 0 {
		out.DurationMS = 0
	}
	if out.CostSummary.CostEstimate.TotalMicros < 0 {
		out.CostSummary.CostEstimate.TotalMicros = 0
	}
	if out.CostSummary.CostEstimate.InputMicros < 0 {
		out.CostSummary.CostEstimate.InputMicros = 0
	}
	if out.CostSummary.CostEstimate.OutputMicros < 0 {
		out.CostSummary.CostEstimate.OutputMicros = 0
	}
	if out.CostSummary.TokenUsage.InputTokens < 0 {
		out.CostSummary.TokenUsage.InputTokens = 0
	}
	if out.CostSummary.TokenUsage.OutputTokens < 0 {
		out.CostSummary.TokenUsage.OutputTokens = 0
	}
	if out.CostSummary.TokenUsage.CachedInputToken < 0 {
		out.CostSummary.TokenUsage.CachedInputToken = 0
	}
	if out.Retries < 0 {
		out.Retries = 0
	}
	if out.RepairOperations < 0 {
		out.RepairOperations = 0
	}
	out.ValidationRefs = normalizeRefs(out.ValidationRefs)
	out.ReceiptRefs = normalizeRefs(out.ReceiptRefs)
	return out
}

func normalizeRefs(in []trace.Ref) []trace.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]trace.Ref, 0, len(in))
	for i := range in {
		ref := trace.NewRef(in[i].Kind, in[i].Ref, in[i].Hash, in[i].SizeBytes)
		if ref.Ref == "" {
			continue
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if out[i].Ref == out[j].Ref {
				return out[i].Hash < out[j].Hash
			}
			return out[i].Ref < out[j].Ref
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
