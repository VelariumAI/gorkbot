package inline

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

// Terminal codes
const (
	codeMoveBOL   = "\r"
	codeClearLine = "\033[2K"
)

// Printer handles concurrent output of text tokens and animated tool hooks.
type Printer struct {
	mu sync.Mutex
	w  io.Writer

	activeTool string
	lastFrame  string
	ticker     *time.Ticker
	done       chan struct{}

	p termenv.Profile
}

func NewPrinter() *Printer {
	return &Printer{
		w: os.Stdout,
		p: termenv.ColorProfile(),
	}
}

// WriteStream prints a text token, handling cursor management if a tool is active.
func (pr *Printer) WriteStream(token string) {
	if token == "" {
		return
	}
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if pr.activeTool != "" {
		// Erase active tool line
		fmt.Fprint(pr.w, codeMoveBOL, codeClearLine)
	}

	fmt.Fprint(pr.w, token)

	if pr.activeTool != "" {
		// Redraw tool line at new position
		pr.drawToolLineInternal()
	}
}

// StartTool begins an animated tool hook.
func (pr *Printer) StartTool(name string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if pr.activeTool != "" {
		pr.stopToolInternal()
	}

	pr.activeTool = name
	pr.lastFrame = "◐"
	pr.done = make(chan struct{})
	pr.ticker = time.NewTicker(150 * time.Millisecond)

	fmt.Fprint(pr.w, "\n")
	pr.drawToolLineInternal()

	go pr.animate()
}

// StopTool finalizes the tool hook.
func (pr *Printer) StopTool(name string, success bool, elapsed time.Duration) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	// Handle case where name might be different but it's the current active tool
	// (e.g. background agent status updates)
	if pr.activeTool == "" {
		return
	}

	pr.stopToolInternal()

	// Final overwrite
	symbol := "✓"
	color := "42" // Green
	if !success {
		symbol = "✗"
		color = "1" // Red
	}

	status := termenv.String(symbol).Foreground(pr.p.Color(color)).String()
	fmt.Fprint(pr.w, codeMoveBOL, codeClearLine)
	if elapsed > 0 {
		fmt.Fprintf(pr.w, "%s %s (%s)\n", status, pr.activeTool, formatDuration(elapsed))
	} else {
		fmt.Fprintf(pr.w, "%s %s\n", status, pr.activeTool)
	}
	pr.activeTool = ""
}

func (pr *Printer) animate() {
	frames := []string{"◐", "◓", "◑", "◒"}
	i := 0
	for {
		select {
		case <-pr.ticker.C:
			pr.mu.Lock()
			if pr.activeTool == "" {
				pr.mu.Unlock()
				return
			}
			pr.lastFrame = frames[i%len(frames)]
			fmt.Fprint(pr.w, codeMoveBOL, codeClearLine)
			pr.drawToolLineInternal()
			i++
			pr.mu.Unlock()
		case <-pr.done:
			return
		}
	}
}

func (pr *Printer) drawToolLineInternal() {
	frame := termenv.String(pr.lastFrame).Foreground(pr.p.Color("33")).String()
	fmt.Fprintf(pr.w, "%s running: %s...", frame, pr.activeTool)
}

func (pr *Printer) stopToolInternal() {
	if pr.ticker != nil {
		pr.ticker.Stop()
	}
	if pr.done != nil {
		select {
		case <-pr.done:
			// already closed
		default:
			close(pr.done)
		}
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
