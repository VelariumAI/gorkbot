package puteradapter

import "context"

// WriteFile performs governed puter.fs.write.
func (a *Adapter) WriteFile(ctx context.Context, rawPath string, data []byte) (Decision, Receipt, error) {
	safePath, decision, err := a.readPath(rawPath, OpFSWrite)
	receipt := a.newReceipt(CapabilityFSWrite, decision, safePath.String(), "", data)
	if err != nil {
		return decision, receipt, err
	}
	if writeErr := a.client.FSWrite(ctx, safePath, data); writeErr != nil {
		return decision, receipt, writeErr
	}
	return decision, receipt, nil
}

// DeleteFile performs governed puter.fs.delete.
func (a *Adapter) DeleteFile(ctx context.Context, rawPath string) (Decision, Receipt, error) {
	safePath, decision, err := a.readPath(rawPath, OpFSDelete)
	receipt := a.newReceipt(CapabilityFSDelete, decision, safePath.String(), "", nil)
	if err != nil {
		return decision, receipt, err
	}
	if delErr := a.client.FSDelete(ctx, safePath); delErr != nil {
		return decision, receipt, delErr
	}
	return decision, receipt, nil
}

// MoveFile performs governed puter.fs.move.
func (a *Adapter) MoveFile(ctx context.Context, rawFrom, rawTo string) (Decision, Receipt, error) {
	from, fromDecision := ValidatePuterWorkspacePath(rawFrom, a.manifest)
	if !fromDecision.Allowed {
		fromDecision.Capability = CapabilityFSMove
		receipt := a.newReceipt(CapabilityFSMove, fromDecision, "", "", nil)
		return fromDecision, receipt, ErrOperationBlocked
	}
	to, toDecision := ValidatePuterWorkspacePath(rawTo, a.manifest)
	if !toDecision.Allowed {
		toDecision.Capability = CapabilityFSMove
		receipt := a.newReceipt(CapabilityFSMove, toDecision, "", "", nil)
		return toDecision, receipt, ErrOperationBlocked
	}
	decision := a.policy.EvaluateMoveOperation(from, to, a.manifest)
	receipt := a.newReceipt(CapabilityFSMove, decision, from.String()+" -> "+to.String(), "", nil)
	if err := a.evaluateExecution(decision); err != nil {
		return decision, receipt, err
	}
	if moveErr := a.client.FSMove(ctx, from, to); moveErr != nil {
		return decision, receipt, moveErr
	}
	return decision, receipt, nil
}
