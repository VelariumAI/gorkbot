package profile

type ApprovalConfig struct {
	RequireHumanApprovalForSensitive    bool `json:"require_human_approval_for_sensitive"`
	RequireApprovalForPolicyAbsence     bool `json:"require_approval_for_policy_absence"`
	RequireApprovalForRelease           bool `json:"require_approval_for_release"`
	RequireApprovalForNetwork           bool `json:"require_approval_for_network"`
	RequireApprovalForPrivateNetwork    bool `json:"require_approval_for_private_network"`
	RequireApprovalForShell             bool `json:"require_approval_for_shell"`
	RequireApprovalForSelfmod           bool `json:"require_approval_for_selfmod"`
	RequireApprovalForPromotion         bool `json:"require_approval_for_promotion"`
	RequireApprovalForIrreversibleMutat bool `json:"require_approval_for_irreversible_mutation"`
}

func (c ApprovalConfig) Normalized() ApprovalConfig {
	return c
}
