package selfmod

import "strings"

type DynamicCapability string

const (
	CapabilityReadFile            DynamicCapability = "dynamic.read_file"
	CapabilityWriteFile           DynamicCapability = "dynamic.write_file"
	CapabilityDeleteFile          DynamicCapability = "dynamic.delete_file"
	CapabilityExec                DynamicCapability = "dynamic.exec"
	CapabilityNetworkFetch        DynamicCapability = "dynamic.network.fetch"
	CapabilityNetworkPrivate      DynamicCapability = "dynamic.network.private"
	CapabilityCredentialsRead     DynamicCapability = "dynamic.credentials.read"
	CapabilityGitWrite            DynamicCapability = "dynamic.git.write"
	CapabilityGitPush             DynamicCapability = "dynamic.git.push"
	CapabilityToolRegister        DynamicCapability = "dynamic.tool.register"
	CapabilityToolExecute         DynamicCapability = "dynamic.tool.execute"
	CapabilitySkillStage          DynamicCapability = "dynamic.skill.stage"
	CapabilitySkillPromote        DynamicCapability = "dynamic.skill.promote"
	CapabilityWorkflowStage       DynamicCapability = "dynamic.workflow.stage"
	CapabilityWorkflowExecute     DynamicCapability = "dynamic.workflow.execute"
	CapabilityHookInstall         DynamicCapability = "dynamic.hook.install"
	CapabilityPuterAppStage       DynamicCapability = "dynamic.puter.app.stage"
	CapabilityPuterAppPreview     DynamicCapability = "dynamic.puter.app.preview"
	CapabilityPuterHostingPublish DynamicCapability = "dynamic.puter.hosting.publish"
	CapabilityPolicyModify        DynamicCapability = "dynamic.policy.modify"
	CapabilityGovernanceModify    DynamicCapability = "dynamic.governance.modify"
	CapabilityVCSESubmit          DynamicCapability = "dynamic.vcse.submit"
	CapabilityVCSEPromote         DynamicCapability = "dynamic.vcse.promote"
	CapabilityHostBridge          DynamicCapability = "dynamic.host.bridge"
)

var allowedCapabilities = map[DynamicCapability]bool{
	CapabilitySkillStage:      true,
	CapabilityWorkflowStage:   true,
	CapabilityPuterAppStage:   true,
	CapabilityPuterAppPreview: true,
	CapabilityVCSESubmit:      true,
}

var approvalCapabilities = map[DynamicCapability]bool{
	CapabilityWriteFile:           true,
	CapabilityToolRegister:        true,
	CapabilityToolExecute:         true,
	CapabilityWorkflowExecute:     true,
	CapabilityGitWrite:            true,
	CapabilityDeleteFile:          true,
	CapabilityNetworkFetch:        true,
	CapabilityHookInstall:         true,
	CapabilityPuterHostingPublish: true,
	CapabilitySkillPromote:        true,
}

var blockedCapabilities = map[DynamicCapability]bool{
	CapabilityCredentialsRead:  true,
	CapabilityNetworkPrivate:   true,
	CapabilityExec:             true,
	CapabilityHostBridge:       true,
	CapabilityPolicyModify:     true,
	CapabilityGovernanceModify: true,
	CapabilityVCSEPromote:      true,
	CapabilityGitPush:          true,
}

func normalizeCapability(raw string) DynamicCapability {
	v := strings.TrimSpace(strings.ToLower(raw))
	v = strings.ReplaceAll(v, "_", ".")
	v = strings.ReplaceAll(v, " ", "")
	return DynamicCapability(v)
}

func classifyCapabilities(caps []DynamicCapability) (requiresApproval bool, hardBlock bool, reason string, issues []string) {
	for _, cap := range caps {
		c := normalizeCapability(string(cap))
		if blockedCapabilities[c] {
			return false, true, REASON_DYNAMIC_CAPABILITY_FORBIDDEN, []string{string(c)}
		}
		if approvalCapabilities[c] {
			requiresApproval = true
			issues = append(issues, string(c))
			continue
		}
		if allowedCapabilities[c] {
			continue
		}
		return false, true, REASON_DYNAMIC_CAPABILITY_FORBIDDEN, []string{"unknown capability: " + string(c)}
	}
	if requiresApproval {
		return true, false, REASON_DYNAMIC_CAPABILITY_REQUIRES_APPROVAL, issues
	}
	return false, false, "", nil
}
