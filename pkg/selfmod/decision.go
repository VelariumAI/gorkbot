package selfmod

type DynamicValidationDecision struct {
	Allowed          bool
	RequiresApproval bool
	HardBlock        bool
	ReasonCode       string
	Issues           []string
	Manifest         *SelfModificationManifest
	Receipt          DynamicValidationReceipt
}

func (d DynamicValidationDecision) IssuesCopy() []string {
	out := make([]string, len(d.Issues))
	copy(out, d.Issues)
	return out
}
