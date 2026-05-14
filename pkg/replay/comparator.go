package replay

import (
	"strings"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type Comparison struct {
	BaselineOutcome  Outcome
	CandidateOutcome Outcome
	Regressions      []Regression
	Improvements     []Improvement
	Verdict          Verdict
}

type Comparator interface {
	Compare(c Case, candidate CandidateOutcome) Comparison
}

type DeterministicComparator struct{}

func (DeterministicComparator) Compare(c Case, candidate CandidateOutcome) Comparison {
	baseline := outcomeFromTrajectory(c.Baseline, c.BaselineEvents)
	cand := outcomeFromTrajectory(candidate.Trajectory, candidate.Events)

	cmp := Comparison{
		BaselineOutcome:  baseline,
		CandidateOutcome: cand,
		Verdict:          VerdictPass,
	}

	if baseline.Invalid {
		// Invalid/missing baseline takes precedence because replay cannot establish
		// a trustworthy comparison target in that state.
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "baseline_invalid", Message: baseline.InvalidReason})
		cmp.Verdict = VerdictInvalid
		return cmp
	}
	if cand.Invalid {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "candidate_invalid", Message: cand.InvalidReason})
		if baseline.HasSuccessSignal && baseline.Success {
			cmp.Regressions = append(cmp.Regressions, Regression{Code: "baseline_pass_candidate_invalid", Message: "baseline succeeded while candidate is invalid"})
		}
		cmp.Verdict = VerdictInvalid
		return cmp
	}

	expect := c.Expectations.Normalized()

	if baseline.HasSuccessSignal && baseline.Success && cand.HasSuccessSignal && !cand.Success {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "outcome_worsened", Message: "baseline succeeded while candidate failed"})
	}
	if baseline.HasSuccessSignal && baseline.Success && !cand.HasSuccessSignal {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "missing_required_outcome", Message: "candidate outcome signal is missing"})
	}

	operatorSet := operatorSet(cand.OperatorPath)
	for i := range expect.RequiredOperators {
		op := expect.RequiredOperators[i]
		if _, ok := operatorSet[op]; !ok {
			cmp.Regressions = append(cmp.Regressions, Regression{Code: "required_operator_missing", Message: "required operator missing: " + string(op)})
		}
	}
	for i := range expect.ForbiddenOperators {
		op := expect.ForbiddenOperators[i]
		if _, ok := operatorSet[op]; ok {
			cmp.Regressions = append(cmp.Regressions, Regression{Code: "forbidden_operator_introduced", Message: "forbidden operator introduced: " + string(op)})
		}
	}

	eventSet := stringSet(cand.EventKinds)
	for i := range expect.RequiredEventKinds {
		k := expect.RequiredEventKinds[i]
		if _, ok := eventSet[k]; !ok {
			cmp.Regressions = append(cmp.Regressions, Regression{Code: "required_event_missing", Message: "required event missing: " + k})
		}
	}
	for i := range expect.ForbiddenEventKinds {
		k := expect.ForbiddenEventKinds[i]
		if _, ok := eventSet[k]; ok {
			cmp.Regressions = append(cmp.Regressions, Regression{Code: "forbidden_event_introduced", Message: "forbidden event introduced: " + k})
		}
	}

	if len(baseline.ValidationRefs) > len(cand.ValidationRefs) {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "validation_refs_worsened", Message: "candidate has fewer validation refs than baseline"})
	}
	if len(baseline.ReceiptRefs) > len(cand.ReceiptRefs) {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "receipt_refs_worsened", Message: "candidate has fewer receipt refs than baseline"})
	}

	baseCost := totalMicros(baseline.CostSummary)
	candCost := totalMicros(cand.CostSummary)
	if expect.MaxCostIncreaseMicros > 0 && candCost-baseCost > expect.MaxCostIncreaseMicros {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "cost_regression", Message: "candidate cost exceeds allowed threshold"})
	}
	if expect.MaxDurationIncreaseMS > 0 && cand.DurationMS-baseline.DurationMS > expect.MaxDurationIncreaseMS {
		cmp.Regressions = append(cmp.Regressions, Regression{Code: "duration_regression", Message: "candidate duration exceeds allowed threshold"})
	}

	if baseline.HasSuccessSignal && baseline.Success && cand.HasSuccessSignal && cand.Success {
		if candCost < baseCost {
			cmp.Improvements = append(cmp.Improvements, Improvement{Code: "cost_improved", Message: "success preserved with lower cost"})
		}
		if cand.DurationMS < baseline.DurationMS {
			cmp.Improvements = append(cmp.Improvements, Improvement{Code: "duration_improved", Message: "success preserved with lower duration"})
		}
		if cand.RepairOperations < baseline.RepairOperations {
			cmp.Improvements = append(cmp.Improvements, Improvement{Code: "repair_operations_reduced", Message: "fewer repair operations"})
		}
		if cand.Retries < baseline.Retries {
			cmp.Improvements = append(cmp.Improvements, Improvement{Code: "retries_reduced", Message: "fewer retries"})
		}
		if len(cand.ValidationRefs) > len(baseline.ValidationRefs) {
			cmp.Improvements = append(cmp.Improvements, Improvement{Code: "validation_strengthened", Message: "stronger validation reference coverage"})
		}
	}

	hasEnoughSignal := baseline.HasSuccessSignal || cand.HasSuccessSignal || len(cmp.Regressions) > 0 || len(cmp.Improvements) > 0
	if !hasEnoughSignal {
		cmp.Verdict = VerdictInconclusive
		return cmp
	}
	if len(cmp.Regressions) > 0 {
		cmp.Verdict = VerdictRegression
		return cmp
	}
	if len(cmp.Improvements) > 0 {
		cmp.Verdict = VerdictImprovement
		return cmp
	}
	cmp.Verdict = VerdictPass
	return cmp
}

func outcomeFromTrajectory(t trace.Trajectory, events []trace.Event) Outcome {
	tn := sanitizeTrajectory(t)
	out := Outcome{
		TrajectoryID: tn.TrajectoryID,
		Status:       strings.ToLower(strings.TrimSpace(tn.Status)),
		Outcome:      strings.ToLower(strings.TrimSpace(tn.Outcome)),
		CostSummary:  tn.CostSummary,
		DurationMS:   tn.Duration,
		OperatorPath: append([]trace.Operator(nil), tn.OperatorPath...),
	}
	if err := tn.Validate(); err != nil {
		out.Invalid = true
		out.InvalidReason = err.Error()
		return normalizeOutcome(out)
	}

	if out.DurationMS == 0 && !tn.FinishedAt.IsZero() && !tn.StartedAt.IsZero() && !tn.FinishedAt.Before(tn.StartedAt) {
		out.DurationMS = tn.FinishedAt.Sub(tn.StartedAt).Milliseconds()
	}

	validation := append([]trace.Ref(nil), tn.ValidationRefs...)
	receipts := append([]trace.Ref(nil), tn.ReceiptRefs...)
	eventKinds := make([]string, 0, len(events))
	for i := range events {
		ev := events[i].Normalized()
		if ev.Decision != "" {
			out.Decision = strings.ToLower(strings.TrimSpace(ev.Decision))
		}
		if ev.ErrorClass != "" {
			out.ErrorClass = strings.ToLower(strings.TrimSpace(ev.ErrorClass))
		}
		if ev.EventKind != "" {
			eventKinds = append(eventKinds, ev.EventKind)
		}
		validation = append(validation, ev.ValidationRefs...)
		receipts = append(receipts, ev.ReceiptRefs...)
		if ev.Operator == trace.OperatorRepair {
			out.RepairOperations++
		}
		if strings.Contains(strings.ToLower(ev.EventKind), "retry") {
			out.Retries++
		}
	}
	if out.RepairOperations == 0 {
		for i := range tn.OperatorPath {
			if tn.OperatorPath[i] == trace.OperatorRepair {
				out.RepairOperations++
			}
		}
	}
	out.EventKinds = eventKinds
	out.ValidationRefs = validation
	out.ReceiptRefs = receipts

	sig, ok := successSignal(out.Status, out.Outcome)
	out.Success = sig
	out.HasSuccessSignal = ok

	return normalizeOutcome(out)
}

func successSignal(status, outcome string) (bool, bool) {
	if v, ok := classifySignal(status); ok {
		return v, true
	}
	if v, ok := classifySignal(outcome); ok {
		return v, true
	}
	return false, false
}

func classifySignal(v string) (bool, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return false, false
	}
	successes := []string{"success", "succeeded", "pass", "passed", "ok", "completed"}
	for i := range successes {
		if s == successes[i] {
			return true, true
		}
	}
	failures := []string{"failure", "failed", "fail", "error", "errored", "invalid"}
	for i := range failures {
		if s == failures[i] {
			return false, true
		}
	}
	inconclusive := []string{"inconclusive", "unknown", "skipped", "missing"}
	for i := range inconclusive {
		if s == inconclusive[i] {
			return false, true
		}
	}
	return false, false
}

func totalMicros(c trace.CostSummary) int64 {
	total := c.CostEstimate.TotalMicros
	if total > 0 {
		return total
	}
	in := c.CostEstimate.InputMicros
	out := c.CostEstimate.OutputMicros
	if in < 0 {
		in = 0
	}
	if out < 0 {
		out = 0
	}
	return in + out
}

func operatorSet(in []trace.Operator) map[trace.Operator]struct{} {
	set := make(map[trace.Operator]struct{}, len(in))
	for i := range in {
		set[in[i]] = struct{}{}
	}
	return set
}

func stringSet(in []string) map[string]struct{} {
	set := make(map[string]struct{}, len(in))
	for i := range in {
		set[strings.ToLower(strings.TrimSpace(in[i]))] = struct{}{}
	}
	return set
}
