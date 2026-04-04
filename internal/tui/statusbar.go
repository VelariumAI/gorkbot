package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/process"
)

// StatusBar represents the dynamic status bar component.
type StatusBar struct {
	width           int
	height          int
	spinner         spinner.Model
	activeProcesses int
	currentModel    string
	tokensUsed      int
	statusMessage   string
	statusTime      time.Time
	styles          *Styles

	// Enhanced fields (P0/P1)
	contextPct     float64 // 0.0–1.0 context window usage
	costUSD        float64 // session cost
	modeName       string  // "NORMAL", "PLAN", "AUTO"
	activeTools    int     // tools currently running
	gitBranch      string  // current git branch
	consultantModel string // consultant provider name (Phase 3)

	// Token rate history for mini sparkline
	tokenRateHistory []float64
}

// NewStatusBar creates a new status bar.
func NewStatusBar(styles *Styles) StatusBar {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.Spinner

	return StatusBar{
		spinner:  s,
		styles:   styles,
		height:   1,
		modeName: "NORMAL",
	}
}

// Init initializes the status bar.
func (s *StatusBar) Init() tea.Cmd {
	return s.spinner.Tick
}

// Update handles messages for the status bar.
func (s *StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return *s, cmd
}

// SetDimensions updates the dimensions.
func (s *StatusBar) SetDimensions(w, h int) {
	s.width = w
}

// SetStatus sets a temporary status message (shown for 5 seconds).
func (s *StatusBar) SetStatus(msg string) {
	s.statusMessage = msg
	s.statusTime = time.Now()
}

// UpdateState updates the persistent state information.
func (s *StatusBar) UpdateState(model string, tokens int, activeProcs []*process.Process) {
	s.currentModel = model
	s.tokensUsed = tokens

	running := 0
	for _, p := range activeProcs {
		if p.State == process.StateRunning {
			running++
		}
	}
	s.activeProcesses = running
}

// UpdateContext updates context window and cost information.
func (s *StatusBar) UpdateContext(usedPct float64, costUSD float64) {
	s.contextPct = usedPct
	s.costUSD = costUSD
}

// SetMode updates the displayed execution mode.
func (s *StatusBar) SetMode(name string) {
	s.modeName = name
}

// SetActiveTools updates the count of currently-executing tools.
func (s *StatusBar) SetActiveTools(count int) {
	s.activeTools = count
}

// SetGitBranch updates the displayed git branch name.
func (s *StatusBar) SetGitBranch(branch string) {
	s.gitBranch = branch
}

// SetProviders updates the primary and consultant provider names (Phase 3).
func (s *StatusBar) SetProviders(primary, consultant string) {
	s.currentModel = primary
	s.consultantModel = consultant
}

// SetTokenRateHistory sets the token rate history for the mini sparkline.
func (s *StatusBar) SetTokenRateHistory(data []float64) {
	s.tokenRateHistory = data
}

// ContextPct returns the current context window usage fraction.
func (s *StatusBar) ContextPct() float64 {
	return s.contextPct
}

// View renders the status bar.
func (s *StatusBar) View() string {
	barStyle := s.styles.StatusBar
	textStyle := s.styles.StatusBarKey
	metaStyle := s.styles.StatusBarValue

	// ── Left: Gorky identity + Status ────────────────────────────────────────
	gorkyGlyph := s.styles.GorkyGlyphStyle.Render(GorkyGlyph)
	left := fmt.Sprintf(" %s  Ready", gorkyGlyph)
	if time.Since(s.statusTime) < 5*time.Second && s.statusMessage != "" {
		left = fmt.Sprintf(" %s  %s %s", gorkyGlyph, s.spinner.View(), s.statusMessage)
	}

	// ── Center: Processes + Active tools ────────────────────────────────────
	center := ""
	if s.activeProcesses > 0 {
		center += fmt.Sprintf("procs:%d", s.activeProcesses)
	}
	if s.activeTools > 0 {
		if center != "" {
			center += " "
		}
		center += fmt.Sprintf("tools:%d", s.activeTools)
	}

	// ── Right: braille gauge | mini sparkline | cost | model | tokens | mode | git ──
	right := s.buildRight()

	// ── Layout ───────────────────────────────────────────────────────────────
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	centerWidth := lipgloss.Width(center)
	totalFixed := leftWidth + rightWidth + centerWidth
	available := s.width - totalFixed
	if available < 2 {
		available = 2
	}
	gapL := available / 2
	gapR := available - gapL

	gapL = max(gapL, 0)
	gapR = max(gapR, 0)

	gapStyleL := lipgloss.NewStyle().Width(gapL).Render("")
	gapStyleR := lipgloss.NewStyle().Width(gapR).Render("")

	content := lipgloss.JoinHorizontal(lipgloss.Center,
		textStyle.Render(left),
		gapStyleL,
		textStyle.Render(center),
		gapStyleR,
		metaStyle.Render(right),
	)

	return barStyle.Width(s.width).Render(content)
}

// buildRight assembles the right-side status string with braille gauge + sparkline.
func (s *StatusBar) buildRight() string {
	parts := []string{}

	// Context braille gauge (colour-coded by pressure)
	if s.contextPct > 0 {
		gaugeColor := GrokBlue
		if s.contextPct >= 0.95 {
			gaugeColor = ErrorRed
		} else if s.contextPct >= 0.80 {
			gaugeColor = WarningYellow
		}
		gauge := RenderGauge(s.contextPct, 8, gaugeColor, "#333333")
		parts = append(parts, gauge)
	}

	// Mini token-rate sparkline (1 row × 8 chars)
	if len(s.tokenRateHistory) > 0 {
		sl := NewSparkline(8, 1)
		sl.SetData(s.tokenRateHistory)
		parts = append(parts, sl.Render(GrokBlue))
	}

	// Cost
	if s.costUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.3f", s.costUSD))
	}

	// Model (shortened)
	if s.currentModel != "" {
		model := s.currentModel
		if len(model) > 12 {
			model = model[:12]
		}
		// Add consultant indicator if present (Phase 3)
		if s.consultantModel != "" {
			consultant := s.consultantModel
			if len(consultant) > 8 {
				consultant = consultant[:8]
			}
			model = fmt.Sprintf("%s·+%s", model, consultant)
		}
		parts = append(parts, model)
	}

	// Token count
	if s.tokensUsed > 0 {
		if s.tokensUsed >= 1000 {
			parts = append(parts, fmt.Sprintf("%.1fk tok", float64(s.tokensUsed)/1000))
		} else {
			parts = append(parts, fmt.Sprintf("%d tok", s.tokensUsed))
		}
	}

	// Mode badge (only shown when not NORMAL)
	if s.modeName != "" && s.modeName != "NORMAL" {
		parts = append(parts, fmt.Sprintf("[%s]", s.modeName))
	}

	// Git branch
	if s.gitBranch != "" {
		br := s.gitBranch
		if len(br) > 10 {
			br = br[:10]
		}
		parts = append(parts, br)
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " | "
		}
		result += p
	}
	return result + " "
}

// max returns the larger of two ints (Go 1.21+ builtin; polyfill for older toolchains).
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
