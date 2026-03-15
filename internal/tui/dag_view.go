package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/dag"
)

// ─── TUI message types for DAG integration ───────────────────────────────────

// DAGEventMsg is sent by the background goroutine that drains Graph.Events().
type DAGEventMsg struct{ dag.Event }

// DAGTickMsg fires periodically to refresh elapsed time displays in dagView.
type DAGTickMsg time.Time

// dagTick schedules the next DAG view refresh (100 ms — smooth elapsed display).
func dagTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return DAGTickMsg(t)
	})
}

// ─── DAGViewModel ─────────────────────────────────────────────────────────────

// DAGViewModel is the Bubble Tea sub-model for the DAG task graph view.
// It is embedded in the main TUI Model via the dagVM field.
// The view is activated by navigating to dagView state.
type DAGViewModel struct {
	graph    *dag.Graph
	resolver *dag.Resolver
	cancelFn context.CancelFunc

	// Progress bars keyed by task ID — one bar per running task.
	bars     map[string]progress.Model
	width    int
	height   int
	selected int // cursor row in the task list
	done     bool
	err      error
}

// NewDAGViewModel creates a DAGViewModel for an already-configured Graph.
// Use StartDAG to also launch the resolver goroutine.
func NewDAGViewModel(g *dag.Graph, r *dag.Resolver, width, height int) *DAGViewModel {
	return &DAGViewModel{
		graph:    g,
		resolver: r,
		bars:     make(map[string]progress.Model),
		width:    width,
		height:   height,
	}
}

// StartDAG launches the resolver in a goroutine and returns a Cmd that begins
// draining Graph.Events() into DAGEventMsg messages for Bubble Tea.
func (vm *DAGViewModel) StartDAG(ctx context.Context) tea.Cmd {
	ctx, cancel := context.WithCancel(ctx)
	vm.cancelFn = cancel

	// Resolver goroutine.
	go func() {
		err := vm.resolver.Run(ctx)
		_ = err // errors are surfaced via individual task statuses
	}()

	// Event drain Cmd — runs continuously until the graph channel closes.
	return tea.Batch(
		vm.drainEvents(),
		dagTick(),
	)
}

// drainEvents returns a Cmd that reads one event from the graph and sends it
// to Bubble Tea, then schedules itself again. This keeps events flowing without
// blocking the Bubble Tea event loop.
func (vm *DAGViewModel) drainEvents() tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-vm.graph.Events():
			if !ok {
				return nil
			}
			return DAGEventMsg{ev}
		case <-time.After(200 * time.Millisecond):
			// Timeout so Bubble Tea isn't blocked indefinitely waiting for events.
			return nil
		}
	}
}

// Update handles DAG-related messages routed from the main Model.Update.
func (vm *DAGViewModel) Update(msg tea.Msg) (*DAGViewModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case DAGEventMsg:
		cmds = append(cmds, vm.handleEvent(m.Event)...)
		cmds = append(cmds, vm.drainEvents())

	case DAGTickMsg:
		// Animate progress bars for running tasks.
		for id, bar := range vm.bars {
			t := vm.graph.Get(id)
			if t == nil {
				continue
			}
			cmd := bar.SetPercent(t.Progress)
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, dagTick())

	case progress.FrameMsg:
		// Let all progress bars process animation frames.
		for id, bar := range vm.bars {
			newModel, cmd := bar.Update(m)
			vm.bars[id] = newModel.(progress.Model)
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			if vm.selected > 0 {
				vm.selected--
			}
		case "down", "j":
			tasks := vm.graph.All()
			if vm.selected < len(tasks)-1 {
				vm.selected++
			}
		case "esc", "q":
			if vm.cancelFn != nil {
				vm.cancelFn()
			}
		}
	}

	return vm, tea.Batch(cmds...)
}

// handleEvent processes a Task state-change event and updates the progress bar map.
func (vm *DAGViewModel) handleEvent(ev dag.Event) []tea.Cmd {
	var cmds []tea.Cmd

	switch ev.Status {
	case dag.StatusRunning:
		// Spin up a new progress bar for this task.
		bar := progress.New(
			progress.WithDefaultGradient(),
			progress.WithWidth(vm.width/2),
			progress.WithoutPercentage(),
		)
		vm.bars[ev.TaskID] = bar

	case dag.StatusCompleted:
		// Animate to 100 % then remove.
		if bar, ok := vm.bars[ev.TaskID]; ok {
			cmd := bar.SetPercent(1.0)
			_ = bar // bar value unchanged; animation runs via FrameMsg
			cmds = append(cmds, cmd)
		}

	case dag.StatusFailed, dag.StatusSkipped:
		delete(vm.bars, ev.TaskID)
	}

	// Check if the whole graph has finished.
	if vm.allDone() {
		vm.done = true
		if vm.cancelFn != nil {
			vm.cancelFn()
		}
	}

	return cmds
}

// allDone reports whether every task has reached a terminal state.
func (vm *DAGViewModel) allDone() bool {
	for _, t := range vm.graph.All() {
		switch t.Status {
		case dag.StatusCompleted, dag.StatusFailed, dag.StatusSkipped, dag.StatusRolledBack:
		default:
			return false
		}
	}
	return true
}

// ─── DAG View rendering ───────────────────────────────────────────────────────

// Render draws the full DAG task list into a string for the TUI viewport.
func (vm *DAGViewModel) Render(width, height int) string {
	vm.width = width
	vm.height = height

	tasks := vm.graph.All()
	if len(tasks) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).
			Padding(2, 4).Render("No tasks in graph.")
	}

	var sb strings.Builder

	// ── Header ────────────────────────────────────────────────────────────
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true).
		Padding(0, 2)

	stats := vm.computeStats(tasks)
	title := fmt.Sprintf("DAG Executor — %s  [%d/%d tasks]",
		vm.graph.ID, stats.done, len(tasks))
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(vm.renderProgressSummary(stats, width))
	sb.WriteString("\n\n")

	// ── Task list ─────────────────────────────────────────────────────────
	for i, t := range tasks {
		sb.WriteString(vm.renderTask(t, i == vm.selected, width))
		sb.WriteString("\n")
	}

	// ── Footer ────────────────────────────────────────────────────────────
	if vm.done {
		footer := dagStatusStyle(dag.StatusCompleted).Render("  ✓ Graph complete — Esc to return  ")
		sb.WriteString("\n" + footer + "\n")
	} else {
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).Italic(true)
		sb.WriteString("\n" + hintStyle.Render("  ↑↓ navigate   Esc cancel graph") + "\n")
	}

	return sb.String()
}

// renderTask draws a single task row with status icon, description, progress
// bar (if running), elapsed time, and RCA report (if failed).
func (vm *DAGViewModel) renderTask(t *dag.Task, selected bool, width int) string {
	iconStyle := dagStatusStyle(t.Status)
	icon := iconStyle.Render(t.Status.Icon())

	descStyle := lipgloss.NewStyle().Width(width/2 - 6)
	if selected {
		descStyle = descStyle.Bold(true).Foreground(lipgloss.Color(TextWhite))
	} else {
		descStyle = descStyle.Foreground(lipgloss.Color(TextGray))
	}

	desc := descStyle.Render(ellipsis(t.Description, width/2-6))

	// Elapsed time.
	elapsed := elapsedStr(t)

	var lines []string

	// Main row: [icon] [description] [elapsed]
	row := lipgloss.JoinHorizontal(lipgloss.Center,
		"  ", icon, "  ", desc, "  ",
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(elapsed),
	)
	if selected {
		row = lipgloss.NewStyle().Background(lipgloss.Color("236")).Width(width).Render(row)
	}
	lines = append(lines, row)

	// Progress bar (running tasks only).
	if t.Status == dag.StatusRunning {
		if bar, ok := vm.bars[t.ID]; ok {
			barStr := "    " + bar.View()
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Render(barStr))
		}
	}

	// Dependencies.
	if len(t.Dependencies) > 0 && selected {
		depStr := fmt.Sprintf("    deps: %s", strings.Join(t.Dependencies, ", "))
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(TextGray)).Italic(true).Render(depStr))
	}

	// Retry count.
	if t.RetryCount > 0 {
		retryStr := fmt.Sprintf("    retried %d×", t.RetryCount)
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(WarningYellow)).Render(retryStr))
	}

	// RCA report (failed tasks only, shown when selected).
	if t.Status == dag.StatusFailed && t.RCAReport != "" && selected {
		rcaStyle := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(ErrorRed)).
			Foreground(lipgloss.Color(TextWhite)).
			Padding(0, 1).
			Width(width - 8)
		lines = append(lines, rcaStyle.Render("RCA: "+t.RCAReport))
	}

	// Error message (failed/skipped).
	if (t.Status == dag.StatusFailed || t.Status == dag.StatusSkipped) && t.ErrMsg != "" && selected {
		errStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ErrorRed)).Italic(true)
		short := ellipsis(t.ErrMsg, width-10)
		lines = append(lines, "    "+errStyle.Render(short))
	}

	return strings.Join(lines, "\n")
}

// renderProgressSummary draws the graph-level summary bar (completed / running / failed).
func (vm *DAGViewModel) renderProgressSummary(s dagStats, width int) string {
	total := s.completed + s.running + s.pending + s.failed + s.skipped
	if total == 0 {
		return ""
	}

	pct := float64(s.completed) / float64(total)
	filled := int(pct * float64(width-4))
	if filled < 0 {
		filled = 0
	}
	if filled > width-4 {
		filled = width - 4
	}

	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("76")).
		Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("238")).
			Render(strings.Repeat("░", width-4-filled))

	stats := fmt.Sprintf("  ✓%d  ●%d  ✗%d  ⊘%d  ○%d",
		s.completed, s.running, s.failed, s.skipped, s.pending)
	statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))

	return "  " + bar + "\n" + statsStyle.Render(stats)
}

// ─── Stats helpers ────────────────────────────────────────────────────────────

type dagStats struct {
	done, pending, running, completed, failed, skipped int
}

func (vm *DAGViewModel) computeStats(tasks []*dag.Task) dagStats {
	var s dagStats
	for _, t := range tasks {
		switch t.Status {
		case dag.StatusCompleted:
			s.completed++
			s.done++
		case dag.StatusRunning:
			s.running++
		case dag.StatusFailed:
			s.failed++
			s.done++
		case dag.StatusSkipped:
			s.skipped++
			s.done++
		default:
			s.pending++
		}
	}
	return s
}

// ─── Style helpers ────────────────────────────────────────────────────────────

func dagStatusStyle(st dag.TaskStatus) lipgloss.Style {
	switch st {
	case dag.StatusCompleted:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(SuccessGreen)).Bold(true)
	case dag.StatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Bold(true)
	case dag.StatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ErrorRed)).Bold(true)
	case dag.StatusSkipped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
	case dag.StatusQueued:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	case dag.StatusRolledBack:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
	}
}

func elapsedStr(t *dag.Task) string {
	if t.StartedAt.IsZero() {
		return ""
	}
	var d time.Duration
	if !t.CompletedAt.IsZero() {
		d = t.CompletedAt.Sub(t.StartedAt)
	} else {
		d = time.Since(t.StartedAt)
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func ellipsis(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len([]rune(s)) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen-1]) + "…"
}

// ─── Main Model integration ───────────────────────────────────────────────────
// The following functions are called from internal/tui/update.go and
// internal/tui/view.go once the dagView state is active.

// renderDAGView is called by Model.View() when m.state == dagView.
// It delegates to the embedded DAGViewModel if one is attached.
func (m *Model) renderDAGView() string {
	if m.dagVM == nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).
			Padding(2, 4).Render("No active task graph. Use /dag or /run to start one.")
	}
	return m.dagVM.Render(m.width, m.height-4)
}

// updateDAGView is called by Model.Update() when m.state == dagView.
func (m *Model) updateDAGView(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dagVM == nil {
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.state = chatView
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.dagVM, cmd = m.dagVM.Update(msg)

	// Close the DAG view automatically when the graph finishes.
	if m.dagVM.done {
		m.state = chatView
		n := len(m.dagVM.graph.All())
		m.addSystemMessage(fmt.Sprintf("DAG complete — %d tasks executed.", n))
		m.updateViewportContent()
	}

	return m, cmd
}
