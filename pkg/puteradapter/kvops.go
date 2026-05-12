package puteradapter

import "context"

// KVGet performs governed puter.kv.get.
func (a *Adapter) KVGet(ctx context.Context, rawKey string) ([]byte, Decision, Receipt, error) {
	safeKey, decision, err := a.readKey(rawKey, OpKVGet)
	receipt := a.newReceipt(CapabilityKVGet, decision, "", safeKey.String(), nil)
	if err != nil {
		return nil, decision, receipt, err
	}
	data, getErr := a.client.KVGet(ctx, safeKey)
	if getErr != nil {
		return nil, decision, receipt, getErr
	}
	receipt = a.newReceipt(CapabilityKVGet, decision, "", safeKey.String(), data)
	return data, decision, receipt, nil
}

// KVSet performs governed puter.kv.set.
func (a *Adapter) KVSet(ctx context.Context, rawKey string, value []byte) (Decision, Receipt, error) {
	safeKey, decision, err := a.readKey(rawKey, OpKVSet)
	receipt := a.newReceipt(CapabilityKVSet, decision, "", safeKey.String(), value)
	if err != nil {
		return decision, receipt, err
	}
	if setErr := a.client.KVSet(ctx, safeKey, value); setErr != nil {
		return decision, receipt, setErr
	}
	return decision, receipt, nil
}

// KVDelete performs governed puter.kv.delete.
func (a *Adapter) KVDelete(ctx context.Context, rawKey string) (Decision, Receipt, error) {
	safeKey, decision, err := a.readKey(rawKey, OpKVDelete)
	receipt := a.newReceipt(CapabilityKVDelete, decision, "", safeKey.String(), nil)
	if err != nil {
		return decision, receipt, err
	}
	if deleteErr := a.client.KVDelete(ctx, safeKey); deleteErr != nil {
		return decision, receipt, deleteErr
	}
	return decision, receipt, nil
}

// AllowStandalone evaluates non-filesystem/KV capabilities for approval routing.
func (a *Adapter) AllowStandalone(op PuterOperation) (Decision, Receipt, error) {
	decision, err := a.allowStandalone(op)
	receipt := a.newReceipt(capabilityForOperation(op), decision, "", "", nil)
	if err != nil {
		return decision, receipt, err
	}
	return decision, receipt, nil
}
