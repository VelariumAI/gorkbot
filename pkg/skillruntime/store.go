package skillruntime

import "context"

type Store interface {
	SaveCandidate(ctx context.Context, candidate Candidate) error
	LoadCandidate(ctx context.Context, id string) (Candidate, error)
	ListCandidates(ctx context.Context) ([]Candidate, error)
	SaveResult(ctx context.Context, result Result) error
	LoadResult(ctx context.Context, id string) (Result, error)
	DisableCandidate(ctx context.Context, id string) error
}
