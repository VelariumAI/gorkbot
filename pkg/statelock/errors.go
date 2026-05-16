package statelock

import "errors"

var (
	ErrInvalidLock                     = errors.New("statelock: invalid lock")
	ErrInvalidProposedState            = errors.New("statelock: invalid proposed state")
	ErrLockConflict                    = errors.New("statelock: lock conflict")
	ErrPolicyAbsent                    = errors.New("statelock: policy absent")
	ErrSensitiveOperationWithoutPolicy = errors.New("statelock: sensitive operation without policy")
	ErrInvalidParadoxReport            = errors.New("statelock: invalid paradox report")
	ErrStoreUnavailable                = errors.New("statelock: store unavailable")
	ErrNotFound                        = errors.New("statelock: not found")
)
