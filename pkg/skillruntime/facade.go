package skillruntime

import (
	"context"
	"errors"

	"github.com/velariumai/gorkbot/pkg/evidence"
)

type Facade struct {
	Store Store
}

func (f Facade) Retrieve(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationRetrieve, req)
}

func (f Facade) Propose(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationPropose, req)
}

func (f Facade) Validate(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationValidate, req)
}

func (f Facade) Stage(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationStage, req)
}

func (f Facade) Promote(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationPromote, req)
}

func (f Facade) Disable(ctx context.Context, req Request) (Result, error) {
	return f.run(ctx, OperationDisable, req)
}

func (f Facade) Run(ctx context.Context, req Request) (Result, error) {
	op := NormalizeOperation(string(req.Operation))
	switch op {
	case OperationRetrieve:
		return f.Retrieve(ctx, req)
	case OperationPropose:
		return f.Propose(ctx, req)
	case OperationValidate:
		return f.Validate(ctx, req)
	case OperationStage:
		return f.Stage(ctx, req)
	case OperationPromote:
		return f.Promote(ctx, req)
	case OperationDisable:
		return f.Disable(ctx, req)
	default:
		return Result{}, ErrInvalidOperation
	}
}

func (f Facade) run(ctx context.Context, op Operation, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	n := req.Normalized()
	n.Operation = op

	if err := n.Operation.Validate(); err != nil {
		return Result{}, err
	}

	if f.Store != nil {
		switch op {
		case OperationRetrieve:
			if n.Candidate.ID != "" {
				loaded, err := f.Store.LoadCandidate(ctx, n.Candidate.ID)
				if err == nil {
					n.Candidate = loaded
				} else if !errors.Is(err, ErrNotFound) {
					return Result{}, err
				}
			}
		case OperationPropose, OperationStage:
			if err := f.Store.SaveCandidate(ctx, n.Candidate); err != nil {
				return Result{}, err
			}
		case OperationDisable:
			if n.Candidate.ID != "" {
				if err := f.Store.DisableCandidate(ctx, n.Candidate.ID); err != nil && !errors.Is(err, ErrNotFound) {
					return Result{}, err
				}
			}
		}
	}

	result := Evaluate(n)
	if op == OperationDisable {
		result.Status = StatusDisabled
		if result.Decision == evidence.DecisionInconclusive {
			result.Decision = evidence.DecisionAuditOnly
		}
		result = result.Normalized()
	}

	if f.Store != nil {
		if err := f.Store.SaveResult(ctx, result); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}
