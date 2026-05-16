package profile

type EvidenceConfig struct {
	RequireEvidenceReceipt    bool `json:"require_evidence_receipt"`
	RequireTraceRef           bool `json:"require_trace_ref"`
	RequireHarnessReport      bool `json:"require_harness_report"`
	RequireReplayNoRegression bool `json:"require_replay_no_regression"`
	RequireStateLockCheck     bool `json:"require_statelock_check"`
	RequireParadoxCheck       bool `json:"require_paradox_check"`
	RequireRollbackPlan       bool `json:"require_rollback_plan"`
	RequireDisablePath        bool `json:"require_disable_path"`
	VectorCandidateOnly       bool `json:"vector_candidate_only"`
}

func (c EvidenceConfig) Normalized() EvidenceConfig {
	return c
}
