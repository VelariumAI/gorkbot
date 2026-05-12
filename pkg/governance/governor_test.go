package governance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/execution"
	"github.com/velariumai/gorkbot/pkg/vcseclient"
)

func newAction(id string, risk RiskClass, tool string) GovernedAction {
	return GovernedAction{
		ID:         id,
		Actor:      "gorkbot",
		Capability: "tool." + tool,
		ToolName:   tool,
		Parameters: map[string]any{"path": "x"},
		RiskClass:  risk,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestGovernorAuditAllowsIfVCSEUnavailable(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_AUDIT
	g := &Governor{
		Policy:   p,
		Budget:   execution.DefaultBudget(),
		VCSE:     vcseclient.New(vcseclient.Config{BaseURL: "http://127.0.0.1:1", Timeout: 25 * time.Millisecond, Enabled: true}),
		Breakers: execution.NewDefaultBreakerSet(),
	}
	d := g.DecideAndApprove(context.Background(), newAction("a1", RISK_EXTERNAL_SIDE_EFFECT, "git_push"))
	if !d.Allowed {
		t.Fatalf("audit should allow, got %#v", d)
	}
}

func TestGovernorFastAllowsReadOnlyIfVCSEUnavailable(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_FAST
	g := &Governor{
		Policy:   p,
		Budget:   execution.DefaultBudget(),
		VCSE:     vcseclient.New(vcseclient.Config{BaseURL: "http://127.0.0.1:1", Timeout: 25 * time.Millisecond, Enabled: true}),
		Breakers: execution.NewDefaultBreakerSet(),
	}
	d := g.DecideAndApprove(context.Background(), newAction("a2", RISK_READ_ONLY, "read_file"))
	if !d.Allowed {
		t.Fatalf("fast mode read-only should fail-open, got %#v", d)
	}
}

func TestGovernorFastBlocksMutationIfVCSEUnavailable(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_FAST
	g := &Governor{
		Policy:   p,
		Budget:   execution.DefaultBudget(),
		VCSE:     vcseclient.New(vcseclient.Config{BaseURL: "http://127.0.0.1:1", Timeout: 25 * time.Millisecond, Enabled: true}),
		Breakers: execution.NewDefaultBreakerSet(),
	}
	d := g.DecideAndApprove(context.Background(), newAction("a3", RISK_LOCAL_MUTATION, "write_file"))
	if d.Allowed {
		t.Fatalf("fast mode mutation should fail-closed on VCSE outage, got %#v", d)
	}
}

func TestGovernorEnforceBlocksMutationOnVCSE4xx(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proposal/validate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, "bad proposal", http.StatusBadRequest)
	}))
	defer s.Close()

	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy:   p,
		Budget:   execution.DefaultBudget(),
		VCSE:     vcseclient.New(vcseclient.Config{BaseURL: s.URL, Timeout: 200 * time.Millisecond, Enabled: true}),
		Breakers: execution.NewDefaultBreakerSet(),
	}

	d := g.DecideAndApprove(context.Background(), newAction("a3b", RISK_LOCAL_MUTATION, "write_file"))
	if d.Allowed {
		t.Fatalf("enforce mode must fail-closed on VCSE non-2xx: %#v", d)
	}
	if d.ReasonCode != REASON_VCSE_UNAVAILABLE {
		t.Fatalf("expected unavailable reason code for VCSE 4xx, got %s", d.ReasonCode)
	}
	if len(d.Issues) == 0 || !strings.Contains(strings.Join(d.Issues, ","), REASON_VCSE_UNAVAILABLE) {
		t.Fatalf("expected VCSE unavailable issue on non-2xx: %#v", d.Issues)
	}
	if d.ReasonCode == REASON_HUMAN_APPROVAL_GRANTED {
		t.Fatalf("unexpected approval override on VCSE failure: %#v", d)
	}
}

func TestGovernorRequiresHumanNilHandlerBlocksFast(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_FAST
	g := &Governor{Policy: p, Budget: execution.DefaultBudget()}
	d := g.DecideAndApprove(context.Background(), newAction("a4", RISK_PRIVILEGED_BRIDGE, "bash"))
	if d.Allowed {
		t.Fatalf("expected blocked without approval handler: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_UNAVAILABLE {
		t.Fatalf("expected unavailable reason, got %s", d.ReasonCode)
	}
}

func TestGovernorRequiresHumanGrantedAllows(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy: p,
		Budget: execution.DefaultBudget(),
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
		}),
	}
	d := g.DecideAndApprove(context.Background(), newAction("a5", RISK_PRIVILEGED_BRIDGE, "bash"))
	if !d.Allowed {
		t.Fatalf("expected allowed after approval: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_GRANTED {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
}

func TestGovernorRequiresHumanDeniedBlocks(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy: p,
		Budget: execution.DefaultBudget(),
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_DENIED, Scope: APPROVAL_ONCE}, nil
		}),
	}
	d := g.DecideAndApprove(context.Background(), newAction("a6", RISK_PRIVILEGED_BRIDGE, "bash"))
	if d.Allowed {
		t.Fatalf("expected blocked after denial: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_DENIED {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
}

func TestGovernorRequiresHumanTimeoutBlocks(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy:          p,
		Budget:          execution.DefaultBudget(),
		ApprovalTimeout: 40 * time.Millisecond,
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			<-ctx.Done()
			return ApprovalResult{}, ctx.Err()
		}),
	}
	start := time.Now()
	d := g.DecideAndApprove(context.Background(), newAction("a7", RISK_PRIVILEGED_BRIDGE, "bash"))
	if d.Allowed {
		t.Fatalf("expected timeout block: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_TIMEOUT {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("approval timeout should not hang")
	}
}

func TestGovernorUsesApprovalRuntime(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	block := make(chan struct{})
	g := &Governor{
		Policy:               p,
		Budget:               execution.DefaultBudget(),
		ApprovalTimeout:      40 * time.Millisecond,
		MaxInflightApprovals: 1,
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			<-block
			return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
		}),
	}
	defer g.Shutdown()

	start := time.Now()
	d := g.DecideAndApprove(context.Background(), newAction("a7b", RISK_PRIVILEGED_BRIDGE, "bash"))
	if d.Allowed {
		t.Fatalf("expected timeout block: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout reason, got %s", d.ReasonCode)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("governor should return promptly on approval timeout")
	}
	close(block)
}

func TestGovernorApprovalCancelledByParentContext(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy:          p,
		Budget:          execution.DefaultBudget(),
		ApprovalTimeout: time.Second,
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			<-ctx.Done()
			return ApprovalResult{}, ctx.Err()
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := g.DecideAndApprove(ctx, newAction("a8", RISK_PRIVILEGED_BRIDGE, "bash"))
	if d.Allowed {
		t.Fatalf("expected cancelled block: %#v", d)
	}
	if d.ReasonCode != REASON_HUMAN_APPROVAL_CANCELLED {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
}

func TestGovernorSessionApprovalCachePreventsSecondPrompt(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	var calls int32
	g := &Governor{
		Policy:        p,
		Budget:        execution.DefaultBudget(),
		ApprovalCache: NewApprovalCache(),
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			atomic.AddInt32(&calls, 1)
			return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION}, nil
		}),
	}
	a := newAction("a9", RISK_PRIVILEGED_BRIDGE, "bash")
	d1 := g.DecideAndApprove(context.Background(), a)
	d2 := g.DecideAndApprove(context.Background(), a)
	if !d1.Allowed || !d2.Allowed {
		t.Fatalf("expected both decisions allowed: %#v %#v", d1, d2)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one prompt call, got %d", calls)
	}
}

func TestGovernorApprovalCannotOverrideHardDynamicBlock(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	g := &Governor{
		Policy: p,
		Budget: execution.DefaultBudget(),
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION}, nil
		}),
	}
	a := newAction("a-hard-1", RISK_SELF_MODIFICATION, "create_tool")
	a.Parameters = map[string]any{
		"manifest": map[string]any{
			"name":             "bad",
			"artifact_kind":    "dynamic_tool",
			"risk_class":       "high",
			"capabilities":     []any{"dynamic.skill.stage"},
			"target_paths":     []any{".gorkbot/staging/tools/bad.go"},
			"expected_effects": []any{"stage"},
			"rollback_plan":    "delete",
			"verified":         true,
		},
	}
	d := g.DecideAndApprove(context.Background(), a)
	if d.Allowed {
		t.Fatalf("hard dynamic block must not be approvable: %#v", d)
	}
	if d.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
	if d.RequiresHuman {
		t.Fatalf("hard block should not request human approval")
	}
}

func TestGovernorBreakerOpensAfterFailures(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	bs := &execution.BreakerSet{VCSE: execution.NewCircuitBreaker("vcse", 2, 2*time.Second)}
	g := &Governor{
		Policy:   p,
		Budget:   execution.DefaultBudget(),
		VCSE:     vcseclient.New(vcseclient.Config{BaseURL: "http://127.0.0.1:1", Timeout: 25 * time.Millisecond, Enabled: true}),
		Breakers: bs,
	}
	_ = g.DecideAndApprove(context.Background(), newAction("a10", RISK_LOCAL_MUTATION, "write_file"))
	_ = g.DecideAndApprove(context.Background(), newAction("a11", RISK_LOCAL_MUTATION, "write_file"))
	if bs.VCSE.State() != execution.BREAKER_OPEN {
		t.Fatalf("expected breaker open, got %s", bs.VCSE.State())
	}
}

func TestBuildCandidateProposalHasNoAuthorityFields(t *testing.T) {
	a := newAction("a12", RISK_LOCAL_MUTATION, "write_file")
	payload := BuildCandidateProposal(a)

	claims, ok := payload["claims"].([]map[string]any)
	if !ok || len(claims) == 0 {
		t.Fatalf("missing claims in payload: %#v", payload)
	}
	if claims[0]["claim_status"] != "PROPOSED" {
		t.Fatalf("expected PROPOSED claim status, got %#v", claims[0]["claim_status"])
	}
	for _, forbidden := range []string{
		"verification_status", "certification_status", "trust_tier", "authoritative_support_profile_id",
		"verified", "certified", "source_supported",
	} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("forbidden field present: %s", forbidden)
		}
	}
}

func TestRequestHumanApprovalReturnsUnavailableWhenNilHandler(t *testing.T) {
	g := &Governor{}
	res, err := g.RequestHumanApproval(context.Background(), newAction("a13", RISK_PRIVILEGED_BRIDGE, "bash"), GovernanceDecision{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_UNAVAILABLE {
		t.Fatalf("expected unavailable, got %#v", res)
	}
}

func TestRequestHumanApprovalMapsTimeoutError(t *testing.T) {
	g := &Governor{
		ApprovalTimeout: 25 * time.Millisecond,
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			<-ctx.Done()
			return ApprovalResult{}, context.DeadlineExceeded
		}),
	}
	res, err := g.RequestHumanApproval(context.Background(), newAction("a14", RISK_PRIVILEGED_BRIDGE, "bash"), GovernanceDecision{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout decision, got %#v", res)
	}
}

func TestRequestHumanApprovalMapsCancelledError(t *testing.T) {
	g := &Governor{
		ApprovalTimeout: time.Second,
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			<-ctx.Done()
			return ApprovalResult{}, context.Canceled
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := g.RequestHumanApproval(ctx, newAction("a15", RISK_PRIVILEGED_BRIDGE, "bash"), GovernanceDecision{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_CANCELLED {
		t.Fatalf("expected cancelled decision, got %#v", res)
	}
}

func TestRequestHumanApprovalPropagatesUnknownAsUnavailable(t *testing.T) {
	g := &Governor{
		ApprovalHandler: ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
			return ApprovalResult{}, errors.New("boom")
		}),
	}
	res, err := g.RequestHumanApproval(context.Background(), newAction("a16", RISK_PRIVILEGED_BRIDGE, "bash"), GovernanceDecision{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_UNAVAILABLE {
		t.Fatalf("expected unavailable, got %#v", res)
	}
}
