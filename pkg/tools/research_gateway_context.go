package tools

import (
	"context"
	"strings"

	"github.com/velariumai/gorkbot/pkg/researchgate"
)

type researchGatewayConfig struct {
	Gateway *researchgate.Gateway
	Mode    string
}

func getResearchGatewayConfig(ctx context.Context) researchGatewayConfig {
	if ctx == nil {
		return researchGatewayConfig{}
	}
	cfg, ok := ctx.Value(researchGatewayContextKey).(researchGatewayConfig)
	if !ok {
		return researchGatewayConfig{}
	}
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = "off"
	}
	return cfg
}

func (c researchGatewayConfig) isEnforce() bool {
	return c.Gateway != nil && c.Mode == "enforce"
}

func (c researchGatewayConfig) isAudit() bool {
	return c.Gateway != nil && c.Mode == "audit"
}
