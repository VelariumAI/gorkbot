package governance

// PostconditionResult represents a future post-execution verification outcome.
type PostconditionResult struct {
	Satisfied bool     `json:"satisfied"`
	Issues    []string `json:"issues,omitempty"`
}
