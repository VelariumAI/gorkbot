package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/execution"
	"github.com/velariumai/gorkbot/pkg/vcseclient"
)

const defaultApprovalTimeout = 30 * time.Second

// Governor coordinates local policy + optional VCSE checks.
type Governor struct {
	Policy          Policy
	Budget          execution.ExecutionBudget
	VCSE            *vcseclient.Client
	Breakers        *execution.BreakerSet
	Progress        *execution.ProgressTracker
	ApprovalHandler ApprovalHandler
	ApprovalCache   *ApprovalCache
	ApprovalTimeout time.Duration
	ApprovalRuntime *ApprovalRuntime
	// RenderGuardTimeout bounds final-answer renderer verification calls.
	RenderGuardTimeout time.Duration
	// RenderGuardPolicy defines renderer verification policy.
	RenderGuardPolicy RendererGuardPolicy
	// RenderGuardOnUnavailable controls correctness behavior when VCSE render
	// verification is unavailable: block|downgrade|audit.
	RenderGuardOnUnavailable string
	// MaxInflightApprovals bounds stuck approval callbacks from non-cancellable handlers.
	MaxInflightApprovals int
	Now                  func() time.Time

	approvalRuntimeMu sync.Mutex
}

// Decide evaluates a governed action.
func (g *Governor) Decide(ctx context.Context, action GovernedAction) GovernanceDecision {
	nowFn := g.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	start := nowFn()

	if action.CreatedAt.IsZero() {
		action.CreatedAt = start
	}

	if g.Policy.Mode == GOVERNANCE_OFF {
		decision := GovernanceDecision{
			ActionID:       action.ID,
			Allowed:        true,
			Mode:           g.Policy.Mode,
			FinalStatus:    GOVERNANCE_ALLOWED,
			ReasonCode:     REASON_POLICY_ALLOWED,
			RiskClass:      action.RiskClass,
			ApprovedAction: &action,
		}
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	if g.Progress != nil {
		stateHash := stateHashFromAction(action)
		g.Progress.RecordAttempt(action.ToolName, action.Parameters, stateHash)
		if g.Progress.IsLooping(action.ToolName, action.Parameters, stateHash, g.Budget.MaxRepeatedToolCalls) {
			if g.Policy.Mode != GOVERNANCE_AUDIT {
				decision := GovernanceDecision{
					ActionID:    action.ID,
					Allowed:     false,
					Mode:        g.Policy.Mode,
					FinalStatus: GOVERNANCE_BLOCKED,
					ReasonCode:  REASON_REPEATED_ACTION_NO_STATE_PROGRESS,
					RiskClass:   action.RiskClass,
					Issues:      []string{REASON_REPEATED_ACTION_NO_STATE_PROGRESS},
				}
				decision.DurationMS = time.Since(start).Milliseconds()
				return decision
			}
		}
	}

	if action.RiskClass == RISK_READ_ONLY && g.Policy.AllowReadOnlyFastPath {
		decision := GovernanceDecision{
			ActionID:       action.ID,
			Allowed:        true,
			Mode:           g.Policy.Mode,
			FinalStatus:    GOVERNANCE_ALLOWED,
			ReasonCode:     REASON_FAST_PATH_READ_ONLY,
			RiskClass:      action.RiskClass,
			ApprovedAction: &action,
		}
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	decision := g.Policy.Evaluate(action)
	if !decision.Allowed && g.Policy.Mode != GOVERNANCE_AUDIT && !decision.RequiresHuman {
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	if g.VCSE == nil || g.Policy.Mode == GOVERNANCE_OFF {
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	shouldFailClosed := g.Policy.Mode == GOVERNANCE_ENFORCE || g.Policy.Mode == GOVERNANCE_CORRECTNESS || (g.Policy.Mode == GOVERNANCE_FAST && action.RiskClass != RISK_READ_ONLY)

	breaker := vcseBreaker(g.Breakers)
	if breaker != nil && !breaker.Allow() {
		decision.Issues = append(decision.Issues, REASON_VCSE_UNAVAILABLE)
		if shouldFailClosed {
			decision.Allowed = false
			decision.RequiresHuman = false
			decision.FinalStatus = GOVERNANCE_BLOCKED
			decision.ReasonCode = REASON_VCSE_UNAVAILABLE
		} else {
			decision.FinalStatus = GOVERNANCE_DEGRADED
		}
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	fastTimeout := g.Budget.VCSEFastTimeout
	if fastTimeout <= 0 {
		fastTimeout = 250 * time.Millisecond
	}
	vcseCtx, cancel := context.WithTimeout(ctx, fastTimeout)
	defer cancel()

	_, err := g.VCSE.ValidateProposal(vcseCtx, BuildCandidateProposal(action))
	if err != nil {
		if breaker != nil {
			breaker.RecordFailure(err.Error())
		}
		if errors.Is(err, vcseclient.ErrTimeout) {
			decision.Issues = append(decision.Issues, REASON_VCSE_TIMEOUT)
			decision.ReasonCode = REASON_VCSE_TIMEOUT
		} else {
			decision.Issues = append(decision.Issues, REASON_VCSE_UNAVAILABLE)
			decision.ReasonCode = REASON_VCSE_UNAVAILABLE
		}
		if shouldFailClosed {
			decision.Allowed = false
			decision.RequiresHuman = false
			decision.FinalStatus = GOVERNANCE_BLOCKED
		} else if g.Policy.Mode == GOVERNANCE_AUDIT {
			decision.Allowed = true
			decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
			decision.ReasonCode = REASON_AUDIT_MODE
		} else {
			decision.Allowed = true
			decision.FinalStatus = GOVERNANCE_DEGRADED
		}
		decision.DurationMS = time.Since(start).Milliseconds()
		return decision
	}

	if breaker != nil {
		breaker.RecordSuccess()
	}
	decision.DurationMS = time.Since(start).Milliseconds()
	if decision.Allowed && decision.ApprovedAction == nil {
		decision.ApprovedAction = &action
	}
	return decision
}

// RequestHumanApproval requests approval with timeout + cache semantics.
func (g *Governor) RequestHumanApproval(ctx context.Context, action GovernedAction, decision GovernanceDecision) (ApprovalResult, error) {
	if g.ApprovalCache != nil {
		if cached, ok := g.ApprovalCache.Get(action); ok {
			return cached, nil
		}
	}

	if g.ApprovalHandler == nil {
		return ApprovalResult{
			ActionID: action.ID,
			Decision: APPROVAL_UNAVAILABLE,
			Scope:    APPROVAL_ONCE,
			Reason:   "approval handler unavailable",
		}, nil
	}

	now := time.Now()
	req := ApprovalRequest{
		ActionID:        action.ID,
		ToolName:        action.ToolName,
		Capability:      action.Capability,
		RiskClass:       action.RiskClass,
		ReasonCode:      decision.ReasonCode,
		Summary:         fmt.Sprintf("Governance approval required for %s (%s)", action.ToolName, action.RiskClass),
		Parameters:      action.Parameters,
		RedactedParams:  RedactParams(action.Parameters),
		ExpectedEffects: action.ExpectedEffects,
		Rollback:        action.Rollback,
		CreatedAt:       now,
		Timeout:         g.approvalTimeout(),
	}

	approvalCtx, cancel := context.WithTimeout(ctx, g.approvalTimeout())
	defer cancel()

	runtime := g.ensureApprovalRuntime()
	approvalKey := approvalKeyFromAction(action)
	res, err := runtime.Run(approvalCtx, approvalKey, func() (ApprovalResult, error) {
		return g.ApprovalHandler.RequestApproval(approvalCtx, req)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			res = ApprovalResult{ActionID: action.ID, Decision: APPROVAL_CANCELLED, Scope: APPROVAL_ONCE, Reason: err.Error()}
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(approvalCtx.Err(), context.DeadlineExceeded) {
			res = ApprovalResult{ActionID: action.ID, Decision: APPROVAL_TIMEOUT, Scope: APPROVAL_ONCE, Reason: err.Error()}
		} else {
			res = ApprovalResult{ActionID: action.ID, Decision: APPROVAL_UNAVAILABLE, Scope: APPROVAL_ONCE, Reason: err.Error()}
		}
	}
	res.DurationMS = time.Since(now).Milliseconds()
	if res.ActionID == "" {
		res.ActionID = action.ID
	}
	if res.Scope == "" {
		res.Scope = APPROVAL_ONCE
	}

	if g.ApprovalCache != nil {
		g.ApprovalCache.Put(action, res)
	}
	return res, nil
}

// DecideAndApprove performs machine decision and, when required, human approval.
func (g *Governor) DecideAndApprove(ctx context.Context, action GovernedAction) GovernanceDecision {
	decisionCtx, cancel := g.Budget.WithToolDecisionTimeout(ctx, action.ToolName)
	defer cancel()

	decision := g.Decide(decisionCtx, action)
	if decision.Mode == GOVERNANCE_OFF {
		return decision
	}
	if isHardBlockReason(decision.ReasonCode) {
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		return decision
	}
	if !decision.RequiresHuman {
		return decision
	}

	if decision.Mode == GOVERNANCE_AUDIT {
		decision.Allowed = true
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
		decision.ReasonCode = REASON_AUDIT_MODE
		if decision.ApprovedAction == nil {
			decision.ApprovedAction = &action
		}
		return decision
	}

	approval, _ := g.RequestHumanApproval(ctx, action, decision)
	decision.Issues = append(decision.Issues, "approval_decision="+string(approval.Decision))

	switch approval.Decision {
	case APPROVAL_GRANTED:
		if ApprovalGranted(approval) {
			decision.Allowed = true
			decision.RequiresHuman = false
			decision.FinalStatus = GOVERNANCE_ALLOWED
			decision.ReasonCode = REASON_HUMAN_APPROVAL_GRANTED
			decision.ApprovedAction = &action
			return decision
		}
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_DENIED
	case APPROVAL_DENIED:
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_DENIED
	case APPROVAL_TIMEOUT:
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_TIMEOUT
	case APPROVAL_CANCELLED:
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_CANCELLED
	case APPROVAL_UNAVAILABLE:
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_UNAVAILABLE
	default:
		decision.Allowed = false
		decision.RequiresHuman = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_HUMAN_APPROVAL_UNAVAILABLE
	}

	return decision
}

func isHardBlockReason(reason string) bool {
	if strings.HasPrefix(reason, "REASON_DYNAMIC_") &&
		reason != REASON_DYNAMIC_CAPABILITY_REQUIRES_APPROVAL &&
		reason != REASON_DYNAMIC_PROMOTION_REQUIRES_APPROVAL {
		return true
	}
	return reason == REASON_SELF_MODIFICATION_REQUIRES_MANIFEST
}

func (g *Governor) approvalTimeout() time.Duration {
	if g.ApprovalTimeout <= 0 {
		return defaultApprovalTimeout
	}
	return g.ApprovalTimeout
}

func (g *Governor) ensureApprovalRuntime() *ApprovalRuntime {
	g.approvalRuntimeMu.Lock()
	defer g.approvalRuntimeMu.Unlock()

	if g.ApprovalRuntime != nil {
		return g.ApprovalRuntime
	}
	g.ApprovalRuntime = NewApprovalRuntime(g.MaxInflightApprovals)
	return g.ApprovalRuntime
}

// Shutdown cancels waiters on in-flight approvals.
func (g *Governor) Shutdown() {
	g.approvalRuntimeMu.Lock()
	runtime := g.ApprovalRuntime
	g.approvalRuntimeMu.Unlock()
	if runtime != nil {
		runtime.Shutdown()
	}
}

// BuildCandidateProposal creates the VCSE candidate proposal payload.
func BuildCandidateProposal(action GovernedAction) map[string]any {
	claims := []map[string]any{
		{
			"claim_status": "PROPOSED",
			"claim_type":   "TOOL_ACTION",
			"claim":        "Candidate tool action proposed for governance review",
			"tool_name":    action.ToolName,
			"risk_class":   string(action.RiskClass),
		},
	}

	return map[string]any{
		"proposal_version": "1.0",
		"proposal_kind":    "CANDIDATE_PROPOSAL",
		"candidate_kind":   "FACTUAL_CLAIM_PACK",
		"claims":           claims,
		"metadata": map[string]any{
			"action_id":  action.ID,
			"tool_name":  action.ToolName,
			"risk_class": string(action.RiskClass),
			"actor":      action.Actor,
			"created_at": action.CreatedAt.UTC().Format(time.RFC3339Nano),
		},
	}
}

func stateHashFromAction(action GovernedAction) string {
	if action.Parameters == nil {
		return ""
	}
	if v, ok := action.Parameters["state_hash"]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func vcseBreaker(set *execution.BreakerSet) *execution.CircuitBreaker {
	if set == nil {
		return nil
	}
	return set.VCSE
}
