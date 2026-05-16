package profile

import "strings"

type Capability string

const (
	CapabilityToolExecute             Capability = "tool_execute"
	CapabilityFileRead                Capability = "file_read"
	CapabilityFileMutate              Capability = "file_mutate"
	CapabilityNetworkEgress           Capability = "network_egress"
	CapabilityPrivateNetworkEgress    Capability = "private_network_egress"
	CapabilityShellExecute            Capability = "shell_execute"
	CapabilitySelfmodValidate         Capability = "selfmod_validate"
	CapabilitySelfmodPromote          Capability = "selfmod_promote"
	CapabilitySkillStage              Capability = "skill_stage"
	CapabilitySkillPromote            Capability = "skill_promote"
	CapabilityPlannerMutateSession    Capability = "planner_mutate_session"
	CapabilityPlannerMutatePersistent Capability = "planner_mutate_persistent"
	CapabilityReleasePublish          Capability = "release_publish"
	CapabilityVectorRetrieve          Capability = "vector_retrieve"
	CapabilityVectorAssertTruth       Capability = "vector_assert_truth"
	CapabilityUnknown                 Capability = "unknown"
)

func NormalizeCapability(raw string) Capability {
	c := Capability(strings.ToLower(strings.TrimSpace(raw)))
	switch c {
	case CapabilityToolExecute, CapabilityFileRead, CapabilityFileMutate,
		CapabilityNetworkEgress, CapabilityPrivateNetworkEgress,
		CapabilityShellExecute, CapabilitySelfmodValidate, CapabilitySelfmodPromote,
		CapabilitySkillStage, CapabilitySkillPromote,
		CapabilityPlannerMutateSession, CapabilityPlannerMutatePersistent,
		CapabilityReleasePublish, CapabilityVectorRetrieve, CapabilityVectorAssertTruth:
		return c
	default:
		return CapabilityUnknown
	}
}

func normalizeCapabilityValue(c Capability) Capability {
	return NormalizeCapability(string(c))
}
