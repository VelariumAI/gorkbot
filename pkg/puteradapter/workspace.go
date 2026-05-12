package puteradapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrAdapterDisabled indicates Puter workspace mode is explicitly off.
	ErrAdapterDisabled = errors.New("puter workspace adapter is disabled")
	// ErrRequiresApproval indicates a guarded operation needs governance approval.
	ErrRequiresApproval = errors.New("puter operation requires governance approval")
	// ErrOperationBlocked indicates a policy-denied operation.
	ErrOperationBlocked = errors.New("puter operation blocked by policy")
)

// Adapter provides governed workspace operations on top of a Puter client sink.
type Adapter struct {
	cfg      Config
	manifest PuterWorkspaceManifest
	policy   CapabilityPolicy
	client   Client
	now      func() time.Time
	newID    func() string
}

// NewAdapter creates an adapter boundary without requiring a live Puter runtime.
func NewAdapter(cfg Config, manifest PuterWorkspaceManifest, policy CapabilityPolicy, client Client) (*Adapter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("puter client cannot be nil")
	}
	if manifest.Root() != cfg.Root {
		return nil, fmt.Errorf("manifest root %q does not match config root %q", manifest.Root(), cfg.Root)
	}
	return &Adapter{
		cfg:      cfg,
		manifest: manifest,
		policy:   policy,
		client:   client,
		now:      time.Now,
		newID:    func() string { return uuid.NewString() },
	}, nil
}

func (a *Adapter) evaluateExecution(decision Decision) error {
	if a.cfg.Mode == WorkspaceOff {
		return ErrAdapterDisabled
	}
	if !decision.Allowed && !decision.RequiresApproval {
		return ErrOperationBlocked
	}
	if decision.RequiresApproval && a.cfg.Mode == WorkspaceEnforce {
		return ErrRequiresApproval
	}
	return nil
}

func (a *Adapter) buildDecisionForDisabled(cap Capability) Decision {
	return Decision{Allowed: false, Capability: cap, ReasonCode: ReasonAdapterDisabled}
}

func (a *Adapter) newReceipt(cap Capability, d Decision, pathValue, keyValue string, payload []byte) Receipt {
	return buildReceipt(a.newID(), cap, d, pathValue, keyValue, payload, a.now())
}

func (a *Adapter) readPath(raw string, op PuterOperation) (PuterWorkspacePath, Decision, error) {
	safePath, pathDecision := ValidatePuterWorkspacePath(raw, a.manifest)
	if !pathDecision.Allowed {
		pathDecision.Capability = capabilityForOperation(op)
		return PuterWorkspacePath{}, pathDecision, ErrOperationBlocked
	}
	decision := a.policy.EvaluatePathOperation(op, safePath, a.manifest)
	if err := a.evaluateExecution(decision); err != nil {
		return safePath, decision, err
	}
	return safePath, decision, nil
}

func (a *Adapter) readKey(raw string, op PuterOperation) (PuterKVKey, Decision, error) {
	safeKey, keyDecision := ValidatePuterKVKey(raw)
	if !keyDecision.Allowed {
		keyDecision.Capability = capabilityForOperation(op)
		return PuterKVKey{}, keyDecision, ErrOperationBlocked
	}
	decision := a.policy.EvaluateKVOperation(op, safeKey)
	if err := a.evaluateExecution(decision); err != nil {
		return safeKey, decision, err
	}
	return safeKey, decision, nil
}

func (a *Adapter) allowStandalone(op PuterOperation) (Decision, error) {
	decision := a.policy.EvaluateStandaloneCapability(op)
	if err := a.evaluateExecution(decision); err != nil {
		return decision, err
	}
	return decision, nil
}

// ReadFile performs governed puter.fs.read.
func (a *Adapter) ReadFile(ctx context.Context, rawPath string) ([]byte, Decision, Receipt, error) {
	safePath, decision, err := a.readPath(rawPath, OpFSRead)
	receipt := a.newReceipt(CapabilityFSRead, decision, safePath.String(), "", nil)
	if err != nil {
		return nil, decision, receipt, err
	}
	data, readErr := a.client.FSRead(ctx, safePath)
	if readErr != nil {
		return nil, decision, receipt, readErr
	}
	receipt = a.newReceipt(CapabilityFSRead, decision, safePath.String(), "", data)
	return data, decision, receipt, nil
}
