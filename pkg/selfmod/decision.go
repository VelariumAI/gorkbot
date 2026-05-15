package selfmod

import "github.com/velariumai/gorkbot/pkg/harness"

type DynamicValidationDecision struct {
	Allowed          bool
	RequiresApproval bool
	HardBlock        bool
	ReasonCode       string
	Issues           []string
	Manifest         *SelfModificationManifest
	Receipt          DynamicValidationReceipt
	AuditSummary     *harness.AuditSummary
}

func (d DynamicValidationDecision) IssuesCopy() []string {
	out := make([]string, len(d.Issues))
	copy(out, d.Issues)
	return out
}
