package governance

import "time"

// Receipt captures a persisted governance decision artifact.
type Receipt struct {
	ID        string             `json:"id"`
	Action    GovernedAction     `json:"action"`
	Decision  GovernanceDecision `json:"decision"`
	CreatedAt time.Time          `json:"created_at"`
}
