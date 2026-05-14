package replay

import "context"

type CaseSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	TrajectoryID string `json:"trajectory_id,omitempty"`
	CandidateID  string `json:"candidate_id,omitempty"`
}

type Store interface {
	SaveCase(ctx context.Context, c Case) error
	LoadCase(ctx context.Context, id string) (Case, error)
	ListCases(ctx context.Context) ([]CaseSummary, error)
	SaveResult(ctx context.Context, r Result) error
	LoadResult(ctx context.Context, id string) (Result, error)
}
