package tui

// model_extensions.go adds new methods to Model without touching the original
// model.go (which contains the touch-scroll logic that must not be disturbed).
// Go allows a type's methods to be spread across multiple files in the same package.

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/discovery"
)

// GetCommandRegistry returns the command registry so main.go can wire up
// orchestrator adapters, skill loaders, and rule engines after model creation.
func (m *Model) GetCommandRegistry() *commands.Registry {
	return m.commands
}

// SetIntegrationCallbacks wires the integration settings getter/setter into the
// Model so the SettingsOverlay Integrations tab can read and persist env vars.
func (m *Model) SetIntegrationCallbacks(
	getter func() map[string]string,
	setter func(key, value string) error,
) {
	m.integrationGetter = getter
	m.integrationSetter = setter
}

// SetExecutionMode updates the mode badge displayed in the status bar.
func (m *Model) SetExecutionMode(modeName string) {
	m.statusBar.SetMode(modeName)
}

// UpdateContextStats pushes context window and cost info to the status bar.
func (m *Model) UpdateContextStats(usedPct float64, costUSD float64) {
	m.statusBar.UpdateContext(usedPct, costUSD)
}

// SetDiscoveryManager wires a discovery.Manager into the TUI.
// It subscribes to model-list updates, converts them to the internal
// discoveryModel type, and forwards them to m.discoverySub so the Update()
// loop can refresh the Cloud Brains tab without touching model.go directly.
func (m *Model) SetDiscoveryManager(dm *discovery.Manager) {
	if dm == nil {
		return
	}
	raw := dm.Subscribe()
	ch := make(chan []discoveryModel, 1)
	m.discoverySub = ch

	// Seed immediately with whatever is already cached.
	if seed := dm.Models(); len(seed) > 0 {
		m.discoveredModels = convertModels(seed)
	}

	// Bridge goroutine: convert discovery.DiscoveredModel → discoveryModel.
	// This goroutine is cheap (only fires on 30-min poll boundaries) and
	// does not hold any TUI state — it just forwards converted snapshots.
	go func() {
		for models := range raw {
			converted := convertModels(models)
			select {
			case ch <- converted:
			default:
			}
		}
	}()
}

func convertModels(src []discovery.DiscoveredModel) []discoveryModel {
	out := make([]discoveryModel, len(src))
	for i, m := range src {
		out[i] = discoveryModel{
			ID:       m.ID,
			Name:     m.Name,
			Provider: m.Provider,
			BestCap:  m.BestCap().String(),
		}
	}
	return out
}

// ── Toast queue management ─────────────────────────────────────────────────

// pushToast inserts msg into the toast queue with full priority ordering,
// ID-based in-place update, and deduplication.  Returns the dismiss tick Cmd.
//
// Queue rules:
//   - If msg.ID != "" and a toast with that ID exists → update in place
//   - If msg.Text matches an existing toast created <2 s ago → skip (dedup)
//   - Otherwise insert; queue is re-sorted by Priority DESC, CreatedAt DESC
//   - Queue capped at 5 items; only the top 3 are ever rendered
//
// Callers must batch the returned Cmd with any other Cmds they return.
func (m *Model) pushToast(msg ToastMsg) tea.Cmd {
	now := time.Now()

	// Resolve defaults.
	priority := msg.Priority
	if priority == 0 {
		priority = PriorityInfo
	}
	color := msg.Color
	if color == "" {
		color = priority.defaultColor()
	}
	ttl := msg.TTL
	if ttl == 0 {
		ttl = priority.defaultTTL()
	}
	// Completed progress toasts: collapse to a short success flash.
	if msg.Kind == KindProgress && msg.Progress >= 1.0 {
		ttl = 2 * time.Second
	}
	var expiresAt time.Time
	if msg.Kind != KindPersistent {
		expiresAt = now.Add(ttl)
	}

	item := toastItem{
		ID:        msg.ID,
		Icon:      msg.Icon,
		Text:      msg.Text,
		Color:     color,
		Priority:  priority,
		Kind:      msg.Kind,
		Progress:  msg.Progress,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	// ID-based update: replace matching toast in-place.
	if msg.ID != "" {
		for i, t := range m.toasts {
			if t.ID == msg.ID {
				m.toasts[i] = item
				m.recalcViewportHeight()
				return toastDismissTick()
			}
		}
	}

	// Deduplication: skip if identical text was enqueued within the last 2 s.
	for _, t := range m.toasts {
		if t.Text == msg.Text && now.Sub(t.CreatedAt) < 2*time.Second {
			return nil // already visible, no-op
		}
	}

	// Append then sort: Priority DESC, CreatedAt DESC within same priority.
	m.toasts = append(m.toasts, item)
	sort.Slice(m.toasts, func(i, j int) bool {
		if m.toasts[i].Priority != m.toasts[j].Priority {
			return m.toasts[i].Priority > m.toasts[j].Priority
		}
		return m.toasts[i].CreatedAt.After(m.toasts[j].CreatedAt)
	})

	// Cap queue at 5; lowest-priority (tail) items are dropped first.
	if len(m.toasts) > 5 {
		m.toasts = m.toasts[:5]
	}

	m.recalcViewportHeight()
	return toastDismissTick()
}
