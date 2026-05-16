package profile

import "strings"

type Mode string

const (
	ModeOff              Mode = "off"
	ModeAudit            Mode = "audit"
	ModeWarn             Mode = "warn"
	ModeApprovalRequired Mode = "approval_required"
	ModeAllowConfigured  Mode = "allow_configured"
	ModeEnforce          Mode = "enforce"
	ModeDisabled         Mode = "disabled"
	ModeUnknown          Mode = "unknown"
)

func NormalizeMode(raw string) Mode {
	m := Mode(strings.ToLower(strings.TrimSpace(raw)))
	switch m {
	case ModeOff, ModeAudit, ModeWarn, ModeApprovalRequired,
		ModeAllowConfigured, ModeEnforce, ModeDisabled:
		return m
	default:
		return ModeUnknown
	}
}

func normalizeModeValue(m Mode) Mode {
	return NormalizeMode(string(m))
}

func conservativeMode(m Mode) Mode {
	n := normalizeModeValue(m)
	if n == ModeUnknown {
		return ModeApprovalRequired
	}
	return n
}
