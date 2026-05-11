package governance

// RiskClass classifies execution risk.
type RiskClass string

const (
	RISK_READ_ONLY            RiskClass = "RISK_READ_ONLY"
	RISK_LOCAL_MUTATION       RiskClass = "RISK_LOCAL_MUTATION"
	RISK_EXTERNAL_SIDE_EFFECT RiskClass = "RISK_EXTERNAL_SIDE_EFFECT"
	RISK_PRIVILEGED_BRIDGE    RiskClass = "RISK_PRIVILEGED_BRIDGE"
	RISK_DESTRUCTIVE          RiskClass = "RISK_DESTRUCTIVE"
	RISK_SELF_MODIFICATION    RiskClass = "RISK_SELF_MODIFICATION"
	RISK_UNKNOWN              RiskClass = "RISK_UNKNOWN"
)
