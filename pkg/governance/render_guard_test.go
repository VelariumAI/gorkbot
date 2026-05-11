package governance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/vcseclient"
)

func TestRenderGuardJSONRoundTrip(t *testing.T) {
	draft := AnswerDraft{
		AnswerID:     "ans-1",
		RenderedText: "Paris is in France.",
		RenderMode:   string(RENDER_MODE_CANONICAL_ONLY),
		ClaimRefs: []AnswerClaimRef{
			{ClaimID: "c1", RenderedText: "Paris is in France.", SourceSpanIDs: []string{"s1"}},
		},
		Metadata: map[string]any{"channel": "oneshot"},
	}
	view := ValidatedClaimView{
		ClaimID:           "c1",
		FinalStatus:       "SOURCE_SUPPORTED",
		CanonicalText:     "Paris is in France.",
		AllowedRenderings: []string{"Paris is in France."},
		SourceSpanIDs:     []string{"s1"},
	}

	blob, err := json.Marshal(struct {
		Answer AnswerDraft         `json:"answer"`
		Claim  ValidatedClaimView  `json:"claim"`
		Policy RendererGuardPolicy `json:"policy"`
	}{
		Answer: draft,
		Claim:  view,
		Policy: RendererGuardPolicy{RenderMode: string(RENDER_MODE_CANONICAL_ONLY)},
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out struct {
		Answer AnswerDraft         `json:"answer"`
		Claim  ValidatedClaimView  `json:"claim"`
		Policy RendererGuardPolicy `json:"policy"`
	}
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out.Answer.AnswerID != draft.AnswerID || out.Claim.ClaimID != view.ClaimID {
		t.Fatalf("unexpected round trip: %#v", out)
	}
}

func TestRenderGuardConstantsStable(t *testing.T) {
	if string(RENDER_MODE_CANONICAL_ONLY) != "CANONICAL_ONLY" {
		t.Fatalf("unexpected render mode constant: %s", RENDER_MODE_CANONICAL_ONLY)
	}
	if RENDER_GUARD_PASSED != "RENDER_GUARD_PASSED" {
		t.Fatalf("unexpected reason constant: %s", RENDER_GUARD_PASSED)
	}
}

func TestBuildAnswerDraftFromClaimViewsNoClaims(t *testing.T) {
	draft := BuildAnswerDraftFromClaimViews("hello", nil, nil)
	if draft.AnswerID == "" {
		t.Fatal("expected answer id")
	}
	if draft.RenderMode != string(RENDER_MODE_CANONICAL_ONLY) {
		t.Fatalf("unexpected render mode: %s", draft.RenderMode)
	}
	if len(draft.ClaimRefs) != 0 {
		t.Fatalf("expected no claim refs: %#v", draft.ClaimRefs)
	}
}

func TestHasUnsupportedSegments(t *testing.T) {
	if HasUnsupportedSegments([]string{"", "   "}) {
		t.Fatal("expected no unsupported segments")
	}
	if !HasUnsupportedSegments([]string{"unsupported claim"}) {
		t.Fatal("expected unsupported segments")
	}
}

func TestVerifyFinalAnswerCorrectnessMissingClaimMap(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{Policy: p}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
	})
	if d.Valid {
		t.Fatalf("expected invalid decision: %#v", d)
	}
	if d.FinalStatus != RENDER_NEEDS_CLAIM_MAP || d.ReasonCode != MISSING_CLAIM_REFS {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func TestVerifyFinalAnswerUnsupportedSegmentsBlocks(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{Policy: p}
	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText:          "hello",
		ClaimRefs:           []AnswerClaimRef{{ClaimID: "c1", RenderedText: "hello"}},
		ClaimViews:          []ValidatedClaimView{{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"}},
		UnsupportedSegments: []string{"not in validated set"},
	})
	if d.Valid {
		t.Fatalf("expected invalid decision: %#v", d)
	}
	if d.FinalStatus != RENDER_EXCEEDS_VALIDATED_MATERIAL {
		t.Fatalf("unexpected final status: %#v", d)
	}
}

func TestVerifyFinalAnswerCorrectnessVCSEValidAllows(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"answer_id":"ans-1",
			"final_status":"RENDER_VALID",
			"valid":true,
			"reason_code":"RENDER_GUARD_PASSED",
			"issues":[],
			"claim_count":1,
			"accepted_claim_ids":["c1"],
			"rejected_claim_ids":[],
			"render_mode":"CANONICAL_ONLY"
		}`))
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{
		Policy:             p,
		VCSE:               vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true}),
		RenderGuardTimeout: 500 * time.Millisecond,
	}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs: []AnswerClaimRef{
			{ClaimID: "c1", RenderedText: "hello"},
		},
		ClaimViews: []ValidatedClaimView{
			{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"},
		},
	})
	if !d.Valid {
		t.Fatalf("expected valid decision: %#v", d)
	}
	if d.ReasonCode != RENDER_GUARD_PASSED {
		t.Fatalf("unexpected reason code: %#v", d)
	}
}

func TestVerifyFinalAnswerCorrectnessVCSEInvalidBlocks(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"answer_id":"ans-1",
			"final_status":"RENDER_INVALID",
			"valid":false,
			"reason_code":"CLAIM_STATUS_NOT_ALLOWED",
			"issues":["claim rejected"],
			"claim_count":1,
			"accepted_claim_ids":[],
			"rejected_claim_ids":["c1"],
			"render_mode":"CANONICAL_ONLY"
		}`))
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{
		Policy:             p,
		VCSE:               vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true}),
		RenderGuardTimeout: 500 * time.Millisecond,
	}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs: []AnswerClaimRef{
			{ClaimID: "c1", RenderedText: "hello"},
		},
		ClaimViews: []ValidatedClaimView{
			{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"},
		},
	})
	if d.Valid {
		t.Fatalf("expected invalid decision: %#v", d)
	}
	if d.FinalStatus != RENDER_INVALID {
		t.Fatalf("unexpected final status: %#v", d)
	}
}

func TestVerifyFinalAnswerCorrectnessVCSETimeoutDowngrades(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`{"valid":true}`))
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{
		Policy:                   p,
		VCSE:                     vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: 25 * time.Millisecond, Enabled: true}),
		RenderGuardTimeout:       25 * time.Millisecond,
		RenderGuardOnUnavailable: RenderGuardUnavailableDowngrade,
	}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs: []AnswerClaimRef{
			{ClaimID: "c1", RenderedText: "hello"},
		},
		ClaimViews: []ValidatedClaimView{
			{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"},
		},
	})
	if d.Valid || d.ReasonCode != RENDER_GUARD_TIMEOUT {
		t.Fatalf("expected timeout downgrade: %#v", d)
	}
	if d.FinalStatus != RENDER_INVALID {
		t.Fatalf("expected downgrade invalid status: %#v", d)
	}
}

func TestVerifyFinalAnswerCorrectnessVCSETimeoutBlocksWithPolicy(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{
		Policy:                   p,
		VCSE:                     vcseclient.New(vcseclient.Config{BaseURL: "http://127.0.0.1:1", Timeout: 20 * time.Millisecond, Enabled: true}),
		RenderGuardTimeout:       20 * time.Millisecond,
		RenderGuardOnUnavailable: RenderGuardUnavailableBlock,
	}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs: []AnswerClaimRef{
			{ClaimID: "c1", RenderedText: "hello"},
		},
		ClaimViews: []ValidatedClaimView{
			{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"},
		},
	})
	if d.Valid {
		t.Fatalf("expected blocked decision: %#v", d)
	}
	if d.FinalStatus != RENDER_POLICY_BLOCKED {
		t.Fatalf("expected policy blocked status: %#v", d)
	}
}

func TestVerifyFinalAnswerAuditModeDoesNotBlock(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"valid":false,"final_status":"RENDER_INVALID","reason_code":"CLAIM_STATUS_NOT_ALLOWED","issues":["bad"]}`))
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_AUDIT
	g := &Governor{
		Policy:             p,
		VCSE:               vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true}),
		RenderGuardTimeout: 300 * time.Millisecond,
	}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs:  []AnswerClaimRef{{ClaimID: "c1", RenderedText: "hello"}},
		ClaimViews: []ValidatedClaimView{
			{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"},
		},
	})
	if d.Valid {
		t.Fatalf("expected vcse invalid audit decision payload for logging: %#v", d)
	}
}

func TestVerifyFinalAnswerOffModeSkips(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_OFF
	g := &Governor{Policy: p}

	d := g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{AnswerText: "hello"})
	if !d.Valid || d.ReasonCode != RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE {
		t.Fatalf("unexpected skip decision: %#v", d)
	}
}

func TestVerifyFinalAnswerCallsRenderVerifyOnly(t *testing.T) {
	var callCount int32
	var lastPath atomic.Pointer[string]
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		p := r.URL.Path
		lastPath.Store(&p)
		_, _ = w.Write([]byte(`{"valid":true,"final_status":"RENDER_VALID","reason_code":"RENDER_GUARD_PASSED"}`))
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	g := &Governor{
		Policy:             p,
		VCSE:               vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true}),
		RenderGuardTimeout: 300 * time.Millisecond,
	}
	_ = g.VerifyFinalAnswer(context.Background(), FinalAnswerVerificationInput{
		AnswerText: "hello",
		ClaimRefs:  []AnswerClaimRef{{ClaimID: "c1", RenderedText: "hello"}},
		ClaimViews: []ValidatedClaimView{{ClaimID: "c1", CanonicalText: "hello", FinalStatus: "SOURCE_SUPPORTED"}},
	})
	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected exactly one vcse call, got %d", callCount)
	}
	ptr := lastPath.Load()
	if ptr == nil || *ptr != "/render/verify" {
		t.Fatalf("expected /render/verify call, got %v", ptr)
	}
}
