package bootstrap

import (
	"log/slog"

	"github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/discovery"
	prov "github.com/velariumai/gorkbot/pkg/providers"
)

func NewProviderCoordinator(provMgr *prov.Manager, primary, consultant ai.AIProvider, discMgr *discovery.Manager, logger *slog.Logger) *providers.ProviderCoordinator {
	return providers.NewProviderCoordinator(provMgr, primary, consultant, discMgr, events.NewBus(), logger)
}
