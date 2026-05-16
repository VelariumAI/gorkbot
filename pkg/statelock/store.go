package statelock

import "context"

type Filter struct {
	Scope     Scope
	Dimension Dimension
	Subject   string
	Status    Status
}

type Store interface {
	SaveLock(ctx context.Context, lock Lock) error
	LoadLock(ctx context.Context, id string) (Lock, error)
	ListLocks(ctx context.Context, filter Filter) ([]Lock, error)
	SaveParadox(ctx context.Context, report ParadoxReport) error
	LoadParadox(ctx context.Context, id string) (ParadoxReport, error)
}
