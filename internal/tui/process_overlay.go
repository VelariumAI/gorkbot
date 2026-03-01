package tui

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/process"
)

// ProcessOverlay displays a list of active processes.
type ProcessOverlay struct {
	table   table.Model
	manager *process.Manager
	styles  *Styles
	width   int
	height  int
	onClose func()
	dirty   bool
}

// NewProcessOverlay creates a new process overlay.
func NewProcessOverlay(pm *process.Manager, styles *Styles, w, h int, onClose func()) *ProcessOverlay {
	columns := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "State", Width: 10},
		{Title: "Command", Width: 30},
		{Title: "Time", Width: 10},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10), // Fixed height for overlay content
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &ProcessOverlay{
		table:   t,
		manager: pm,
		styles:  styles,
		width:   w,
		height:  h,
		onClose: onClose,
		dirty:   true,
	}
}

// Init initializes the overlay.
func (p *ProcessOverlay) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (p *ProcessOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			if p.onClose != nil {
				p.onClose()
			}
			return nil, nil
		case "k":
			if len(p.table.SelectedRow()) > 0 {
				id := p.table.SelectedRow()[0]
				p.manager.Stop(id)
				p.Refresh()
				return p, nil
			}
		case "r":
			p.dirty = true
			return p, nil
		}
	}
	p.table, cmd = p.table.Update(msg)
	return p, cmd
}

// Refresh updates the table data directly.
func (p *ProcessOverlay) Refresh() {
	procs := p.manager.ListProcesses()
	rows := make([]table.Row, 0, len(procs))
	for _, proc := range procs {
		rows = append(rows, table.Row{
			proc.ID,
			string(proc.State),
			proc.Command,
			proc.StartTime.Format("15:04:05"),
		})
	}
	p.table.SetRows(rows)
}

// View renders the overlay.
func (p *ProcessOverlay) View() string {
	if p.dirty {
		p.Refresh()
		p.dirty = false
	}

	header := p.styles.ActiveTab.Render(" Process Manager ")
	
	// Calculate center position
	// Overlay box
	box := p.styles.ConsultantBox.Render(
		lipgloss.JoinVertical(lipgloss.Center,
			header,
			p.table.View(),
			p.styles.Help.Render("esc: close • r: refresh • k: kill"),
		),
	)
	
	return lipgloss.Place(p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
