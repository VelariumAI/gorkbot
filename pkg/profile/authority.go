package profile

import "strings"

type AuthoritySurface string

const (
	SurfaceToolAuthority            AuthoritySurface = "tool_authority"
	SurfaceFileAuthority            AuthoritySurface = "file_authority"
	SurfaceNetworkAuthority         AuthoritySurface = "network_authority"
	SurfacePrivateNetworkAuthority  AuthoritySurface = "private_network_authority"
	SurfaceShellAuthority           AuthoritySurface = "shell_authority"
	SurfaceSelfmodAuthority         AuthoritySurface = "selfmod_authority"
	SurfacePromotionAuthority       AuthoritySurface = "promotion_authority"
	SurfacePlannerMutationAuthority AuthoritySurface = "planner_mutation_authority"
	SurfaceReleaseAuthority         AuthoritySurface = "release_authority"
	SurfaceHostBridgeAuthority      AuthoritySurface = "host_bridge_authority"
	SurfaceWorkspaceEscapeAuthority AuthoritySurface = "workspace_escape_authority"
	SurfaceVectorRetrievalAuthority AuthoritySurface = "vector_retrieval_authority"
)

type AuthorityMode string

const (
	AuthorityDisabled        AuthorityMode = "disabled"
	AuthorityAuditOnly       AuthorityMode = "audit_only"
	AuthorityPromptOnce      AuthorityMode = "prompt_once"
	AuthorityPromptSession   AuthorityMode = "prompt_session"
	AuthorityPromptAlways    AuthorityMode = "prompt_always"
	AuthorityAllowConfigured AuthorityMode = "allow_configured"
	AuthorityAllow           AuthorityMode = "allow"
	AuthorityDeny            AuthorityMode = "deny"
	AuthorityUnknown         AuthorityMode = "unknown"
)

func NormalizeAuthorityMode(raw string) AuthorityMode {
	m := AuthorityMode(strings.ToLower(strings.TrimSpace(raw)))
	switch m {
	case AuthorityDisabled, AuthorityAuditOnly, AuthorityPromptOnce,
		AuthorityPromptSession, AuthorityPromptAlways, AuthorityAllowConfigured,
		AuthorityAllow, AuthorityDeny:
		return m
	default:
		return AuthorityUnknown
	}
}

func normalizeAuthorityValue(mode AuthorityMode) AuthorityMode {
	return NormalizeAuthorityMode(string(mode))
}

func authorizesDirectly(mode AuthorityMode) bool {
	return normalizeAuthorityValue(mode) == AuthorityAllow
}

func requiresPrompt(mode AuthorityMode) bool {
	n := normalizeAuthorityValue(mode)
	return n == AuthorityPromptOnce || n == AuthorityPromptSession || n == AuthorityPromptAlways
}

type AuthorityConfig struct {
	ToolAuthority            AuthorityMode `json:"tool_authority"`
	FileAuthority            AuthorityMode `json:"file_authority"`
	NetworkAuthority         AuthorityMode `json:"network_authority"`
	PrivateNetworkAuthority  AuthorityMode `json:"private_network_authority"`
	ShellAuthority           AuthorityMode `json:"shell_authority"`
	SelfmodAuthority         AuthorityMode `json:"selfmod_authority"`
	PromotionAuthority       AuthorityMode `json:"promotion_authority"`
	PlannerMutationAuthority AuthorityMode `json:"planner_mutation_authority"`
	ReleaseAuthority         AuthorityMode `json:"release_authority"`
	HostBridgeAuthority      AuthorityMode `json:"host_bridge_authority"`
	WorkspaceEscapeAuthority AuthorityMode `json:"workspace_escape_authority"`
	VectorRetrievalAuthority AuthorityMode `json:"vector_retrieval_authority"`
}

func (c AuthorityConfig) Normalized() AuthorityConfig {
	out := c
	out.ToolAuthority = normalizeAuthorityValue(out.ToolAuthority)
	out.FileAuthority = normalizeAuthorityValue(out.FileAuthority)
	out.NetworkAuthority = normalizeAuthorityValue(out.NetworkAuthority)
	out.PrivateNetworkAuthority = normalizeAuthorityValue(out.PrivateNetworkAuthority)
	out.ShellAuthority = normalizeAuthorityValue(out.ShellAuthority)
	out.SelfmodAuthority = normalizeAuthorityValue(out.SelfmodAuthority)
	out.PromotionAuthority = normalizeAuthorityValue(out.PromotionAuthority)
	out.PlannerMutationAuthority = normalizeAuthorityValue(out.PlannerMutationAuthority)
	out.ReleaseAuthority = normalizeAuthorityValue(out.ReleaseAuthority)
	out.HostBridgeAuthority = normalizeAuthorityValue(out.HostBridgeAuthority)
	out.WorkspaceEscapeAuthority = normalizeAuthorityValue(out.WorkspaceEscapeAuthority)
	out.VectorRetrievalAuthority = normalizeAuthorityValue(out.VectorRetrievalAuthority)
	return out
}

func (c AuthorityConfig) SurfaceMode(surface AuthoritySurface) AuthorityMode {
	switch surface {
	case SurfaceToolAuthority:
		return c.ToolAuthority
	case SurfaceFileAuthority:
		return c.FileAuthority
	case SurfaceNetworkAuthority:
		return c.NetworkAuthority
	case SurfacePrivateNetworkAuthority:
		return c.PrivateNetworkAuthority
	case SurfaceShellAuthority:
		return c.ShellAuthority
	case SurfaceSelfmodAuthority:
		return c.SelfmodAuthority
	case SurfacePromotionAuthority:
		return c.PromotionAuthority
	case SurfacePlannerMutationAuthority:
		return c.PlannerMutationAuthority
	case SurfaceReleaseAuthority:
		return c.ReleaseAuthority
	case SurfaceHostBridgeAuthority:
		return c.HostBridgeAuthority
	case SurfaceWorkspaceEscapeAuthority:
		return c.WorkspaceEscapeAuthority
	case SurfaceVectorRetrievalAuthority:
		return c.VectorRetrievalAuthority
	default:
		return AuthorityUnknown
	}
}
