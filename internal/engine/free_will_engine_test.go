package engine

import (
	"context"
	"testing"
	"time"
)

func TestFreeWillEngine_DefaultsAndConfigure(t *testing.T) {
	fw := NewFreeWillEngine()
	if fw.enabled {
		t.Fatalf("expected disabled by default")
	}
	if fw.maxAutonomousRisk != "low" || fw.autoApplyConfidenceThreshold != 85 {
		t.Fatalf("unexpected defaults")
	}

	fw.Configure(map[string]interface{}{
		"engine_enabled":                  true,
		"max_autonomous_risk":             "high",
		"auto_apply_confidence_threshold": float64(70),
		"proposal_frequency":              "continuous",
		"loop_guard_sensitivity":          0.72,
		"rollback_window_size":            float64(50),
	})

	if !fw.enabled || fw.maxAutonomousRisk != "high" || fw.autoApplyConfidenceThreshold != 70 {
		t.Fatalf("configure did not update core fields")
	}
	if fw.proposalFrequency != "continuous" || fw.rollbackWindowSize != 50 {
		t.Fatalf("configure did not update frequency/window fields")
	}
}

func TestFreeWillEngine_RecordObservationAndConfidence(t *testing.T) {
	fw := NewFreeWillEngine()
	fw.RecordObservation(FreeWillObservation{Domain: "x", Confidence: 1})
	if len(fw.observationQueue) != 0 {
		t.Fatalf("expected disabled engine to ignore observation")
	}

	fw.enabled = true
	fw.RecordObservation(FreeWillObservation{Domain: "x", Confidence: 0.9})
	if len(fw.observationQueue) != 1 {
		t.Fatalf("expected observation to be queued when enabled")
	}

	if got := fw.synthesizeConfidence(nil); got != 0 {
		t.Fatalf("expected zero confidence for empty observations")
	}
	if got := fw.synthesizeConfidence([]FreeWillObservation{{Confidence: 0.5}, {Confidence: 0.9}}); got != 70 {
		t.Fatalf("unexpected synthesized confidence: %d", got)
	}
}

func TestFreeWillEngine_RiskClassificationAndAutoApply(t *testing.T) {
	fw := NewFreeWillEngine()
	fw.enabled = true
	fw.maxAutonomousRisk = "medium"
	fw.autoApplyConfidenceThreshold = 80

	if got := fw.classifyRisk("prompt_optimization"); got != "low" {
		t.Fatalf("unexpected risk classification: %s", got)
	}
	if got := fw.classifyRisk("unknown_domain"); got != "medium" {
		t.Fatalf("expected medium default risk, got %s", got)
	}

	if !fw.isRiskAcceptable("low") || !fw.isRiskAcceptable("medium") {
		t.Fatalf("expected low/medium to be acceptable")
	}
	if fw.isRiskAcceptable("high") {
		t.Fatalf("expected high risk to be unacceptable under medium max")
	}

	if fw.CanAutoApply(&FreeWillProposal{RiskLevel: "medium", ConfidenceScore: 90}) != true {
		t.Fatalf("expected proposal to be auto-applicable")
	}
	if fw.CanAutoApply(&FreeWillProposal{RiskLevel: "high", ConfidenceScore: 95}) {
		t.Fatalf("expected risk gate to reject high-risk proposal")
	}
	if fw.CanAutoApply(&FreeWillProposal{RiskLevel: "low", ConfidenceScore: 10}) {
		t.Fatalf("expected confidence gate to reject low-confidence proposal")
	}
}

func TestFreeWillEngine_GenerateProposal(t *testing.T) {
	fw := NewFreeWillEngine()
	if p := fw.GenerateProposal(context.Background(), "prompt_optimization", []FreeWillObservation{{Confidence: 1}}); p != nil {
		t.Fatalf("expected nil proposal when engine disabled")
	}

	fw.enabled = true
	fw.lastProposalTime = time.Now()
	if p := fw.GenerateProposal(context.Background(), "prompt_optimization", []FreeWillObservation{{Confidence: 1}}); p != nil {
		t.Fatalf("expected nil proposal under rate limit")
	}

	fw.lastProposalTime = time.Now().Add(-time.Hour)
	p := fw.GenerateProposal(context.Background(), "prompt_optimization", []FreeWillObservation{{Confidence: 0.8}, {Confidence: 1.0}})
	if p == nil {
		t.Fatalf("expected proposal when enabled and outside rate limit")
	}
	if p.Domain != "prompt_optimization" || p.RiskLevel != "low" {
		t.Fatalf("unexpected proposal fields: %+v", p)
	}
	if p.ConfidenceScore != 90 {
		t.Fatalf("unexpected proposal confidence score: %d", p.ConfidenceScore)
	}
}

func TestFreeWillEngine_AutonomousHeartbeat(t *testing.T) {
	fw := NewFreeWillEngine()
	fw.enabled = true
	fw.proposalFrequency = "continuous"
	fw.lastProposalTime = time.Now().Add(-time.Hour)
	fw.SetAutonomousHeartbeatInterval(10 * time.Millisecond)

	fw.RecordObservation(FreeWillObservation{Domain: "tool_efficiency", Confidence: 0.9})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fw.StartAutonomousHeartbeat(ctx)

	deadline := time.After(500 * time.Millisecond)
	for {
		if len(fw.proposalQueue) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected autonomous heartbeat to enqueue at least one proposal")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	fw.StopAutonomousHeartbeat()
}

func TestLoopGuard_CoreBehavior(t *testing.T) {
	lg := NewLoopGuard(0.8, 2)
	lg.SetSensitivity(0.9)

	if !lg.CheckRecursionDepth() {
		t.Fatalf("expected recursion depth to be initially valid")
	}
	lg.IncrementRecursionDepth()
	lg.IncrementRecursionDepth()
	lg.IncrementRecursionDepth()
	if lg.CheckRecursionDepth() {
		t.Fatalf("expected recursion depth limit to be reached")
	}
	lg.ResetRecursionDepth()
	if !lg.CheckRecursionDepth() {
		t.Fatalf("expected recursion depth reset")
	}

	if got := lg.normalizeText("  Hello   WORLD  "); got != "hello world" {
		t.Fatalf("unexpected normalizeText output: %q", got)
	}
	if got := lg.computeHash("x"); got == "" {
		t.Fatalf("expected non-empty hash")
	}

	t1 := lg.tokenize("a b c")
	t2 := lg.tokenize("b c d")
	if sim := lg.jaccardSimilarity(t1, t2); sim <= 0 || sim >= 1 {
		t.Fatalf("unexpected jaccard similarity: %f", sim)
	}
	if sim := lg.computeSimilarity(lg.computeHash("abc"), "abc"); sim < 0 || sim > 1 {
		t.Fatalf("unexpected similarity range: %f", sim)
	}

	lg.RecordApprovedEvolution(EvolutionRecord{ID: "1", Domain: "d", ProposedChange: "change 1", OutcomeMetric: 1})
	lg.RecordApprovedEvolution(EvolutionRecord{ID: "2", Domain: "d", ProposedChange: "change 2", OutcomeMetric: 1})
	lg.RecordApprovedEvolution(EvolutionRecord{ID: "3", Domain: "d", ProposedChange: "change 3", OutcomeMetric: 1})
	if len(lg.recentEvolutions) != 2 {
		t.Fatalf("expected rollback window trim to 2, got %d", len(lg.recentEvolutions))
	}

	// Force a loop detection by seeding a degraded identical hash.
	blocked := NewLoopGuard(0.5, 10)
	blocked.RecordApprovedEvolution(EvolutionRecord{ID: "1", Domain: "prompt_optimization", ProposedChange: "", OutcomeMetric: 0.1})
	blocked.RecordApprovedEvolution(EvolutionRecord{ID: "2", Domain: "prompt_optimization", ProposedChange: "x", OutcomeMetric: 1.0})
	if !blocked.DetectDivergence("prompt_optimization", &FreeWillProposal{ProposedChange: ""}) {
		t.Fatalf("expected loop guard divergence detection for degraded identical proposal")
	}
}
