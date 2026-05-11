package contracts

// CandidateProposalContract is a lightweight payload contract used for VCSE candidate checks.
type CandidateProposalContract struct {
	ProposalVersion string         `json:"proposal_version"`
	ProposalKind    string         `json:"proposal_kind"`
	CandidateKind   string         `json:"candidate_kind"`
	Claims          []ClaimPayload `json:"claims"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// ClaimPayload describes a proposed claim under evaluation.
type ClaimPayload struct {
	ClaimStatus string `json:"claim_status"`
	ClaimType   string `json:"claim_type"`
	Claim       string `json:"claim"`
}
