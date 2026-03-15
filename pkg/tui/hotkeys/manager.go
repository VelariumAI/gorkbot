package hotkeys

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Command represents an action triggered by the user via a hotkey.
type Command string

const (
	CmdToolsMenu     Command = "TOOLS_MENU"
	CmdSettings      Command = "SETTINGS"
	CmdModelsSelect  Command = "MODELS_SELECTION"
	CmdExpandDetails Command = "EXPAND_DETAILS"
	CmdExpandAll     Command = "EXPAND_EVERYTHING"
	CmdOmniSearch    Command = "OMNI_SEARCH"
	CmdDebugToggle   Command = "DEBUG_TOGGLE"
	CmdContextStatus Command = "CONTEXT_STATUS"
	CmdClearReset    Command = "CLEAR_RESET"
	CmdDuplicateSess Command = "DUPLICATE_SESSION"
	CmdExportJSON    Command = "EXPORT_JSON"
	CmdForceRefresh  Command = "FORCE_REFRESH"
	CmdHistorySearch Command = "HISTORY_SEARCH"
	CmdHardQuit      Command = "HARD_QUIT"
	CmdUnknown       Command = "UNKNOWN"
)

// The primary hotkeys map (Ctrl+<key>)
//
// NOTE: 0x0D (Ctrl+M) is intentionally absent. In raw terminal mode Ctrl+M
// and the Enter key both send byte 0x0D — they are indistinguishable. We
// must never intercept 0x0D here; Enter must always reach BubbleTea.
// Model selection is instead bound to Ctrl+G (0x07, bell) which has no
// other meaning and also works via the Esc+M leader sequence.
var primaryKeys = map[rune]Command{
	0x07: CmdModelsSelect,  // Ctrl+G — open model selection
	0x14: CmdToolsMenu,     // Ctrl+T
	0x13: CmdSettings,      // Ctrl+S
	0x0F: CmdExpandDetails, // Ctrl+O
	0x05: CmdExpandAll,     // Ctrl+E
	0x0B: CmdOmniSearch,    // Ctrl+K
	0x10: CmdContextStatus, // Ctrl+P
	0x0C: CmdClearReset,    // Ctrl+L
	0x04: CmdDuplicateSess, // Ctrl+D
	0x12: CmdHistorySearch, // Ctrl+R — history search (was: force refresh)
	0x03: CmdHardQuit,      // Ctrl+C
}

// The leader sequence map (Esc, <key>)
var leaderKeys = map[rune]Command{
	'T': CmdToolsMenu,
	'S': CmdSettings,
	'M': CmdModelsSelect,
	'O': CmdExpandDetails,
	'E': CmdExpandAll,
	'K': CmdOmniSearch,
	'I': CmdDebugToggle,
	'P': CmdContextStatus,
	'L': CmdClearReset,
	'D': CmdDuplicateSess,
	'J': CmdExportJSON,
	'R': CmdHistorySearch,
	'Q': CmdHardQuit,
	't': CmdToolsMenu,
	's': CmdSettings,
	'm': CmdModelsSelect,
	'o': CmdExpandDetails,
	'e': CmdExpandAll,
	'k': CmdOmniSearch,
	'i': CmdDebugToggle,
	'p': CmdContextStatus,
	'l': CmdClearReset,
	'd': CmdDuplicateSess,
	'j': CmdExportJSON,
	'r': CmdHistorySearch,
	'q': CmdHardQuit,
}

// SaveFunc is the signature for the synchronous save routine.
type SaveFunc func() error

// Manager handles concurrent terminal input and signal trapping.
type Manager struct {
	Commands     chan Command
	saveRoutine  SaveFunc
	passthrough  io.Writer
	oldState     *term.State
	mu           sync.Mutex
	isLeaderMode bool
	leaderTimer  *time.Timer
	stopChan     chan struct{}
}

// NewManager creates a new HotkeyManager.
// The passthrough writer allows non-hotkey runes to be forwarded to a downstream reader (like BubbleTea).
func NewManager(saveRoutine SaveFunc, passthrough io.Writer) *Manager {
	return &Manager{
		Commands:    make(chan Command, 10),
		saveRoutine: saveRoutine,
		passthrough: passthrough,
		stopChan:    make(chan struct{}),
	}
}

// Start places the terminal in raw mode, intercepts signals, and starts the input listener.
func (m *Manager) Start() error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("stdin is not a terminal")
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to put terminal in raw mode: %v", err)
	}
	m.oldState = state

	go m.handleSignals()
	go m.listen()

	return nil
}

// Restore returns the terminal to its original state.
func (m *Manager) Restore() {
	if m.oldState != nil {
		fd := int(os.Stdin.Fd())
		_ = term.Restore(fd, m.oldState)
	}
}

// Stop halts the listener cleanly.
func (m *Manager) Stop() {
	close(m.stopChan)
	m.Restore()
}

func (m *Manager) handleSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigs:
		m.shutdownSequence()
	case <-m.stopChan:
		return
	}
}

func (m *Manager) shutdownSequence() {
	select {
	case m.Commands <- CmdHardQuit:
	default:
	}

	if m.saveRoutine != nil {
		_ = m.saveRoutine()
	}
	m.Restore()
	os.Exit(0)
}

func (m *Manager) listen() {
	reader := bufio.NewReader(os.Stdin)
	var writer *bufio.Writer
	if m.passthrough != nil {
		writer = bufio.NewWriter(m.passthrough)
	}

	for {
		select {
		case <-m.stopChan:
			return
		default:
			r, size, err := reader.ReadRune()
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			m.mu.Lock()
			leaderMode := m.isLeaderMode
			m.mu.Unlock()

			if leaderMode {
				m.handleLeaderInput(writer, r, size)
			} else {
				m.handleNormalInput(reader, writer, r, size)
			}

			// If the OS delivered a chunk of bytes (like a mouse sequence), process them all
			// before flushing to BubbleTea. This preserves the packet boundaries and prevents
			// BubbleTea's CSI parser from breaking due to fragmentation.
			if writer != nil && reader.Buffered() == 0 {
				writer.Flush()
			}
		}
	}
}

func (m *Manager) handleNormalInput(reader *bufio.Reader, writer *bufio.Writer, r rune, size int) {
	if r == 0x1B {
		// If there are bytes already buffered, this is part of a terminal escape sequence
		// (like an arrow key or mouse click), NOT a human pressing the Esc key.
		if reader.Buffered() > 0 {
			m.passToDownstream(writer, 0x1B)
			return
		}

		m.mu.Lock()
		m.isLeaderMode = true
		m.leaderTimer = time.AfterFunc(750*time.Millisecond, func() {
			m.mu.Lock()
			m.isLeaderMode = false
			m.mu.Unlock()
			if writer != nil {
				m.passToDownstream(writer, 0x1B)
				writer.Flush()
			}
		})
		m.mu.Unlock()
		return
	}

	if cmd, exists := primaryKeys[r]; exists {
		if cmd == CmdHardQuit {
			m.shutdownSequence()
			return
		}
		m.dispatch(cmd)
		return
	}

	m.passToDownstream(writer, r)
}

func (m *Manager) handleLeaderInput(writer *bufio.Writer, r rune, size int) {
	m.mu.Lock()
	m.isLeaderMode = false
	if m.leaderTimer != nil {
		m.leaderTimer.Stop()
	}
	m.mu.Unlock()

	if cmd, exists := leaderKeys[r]; exists {
		if cmd == CmdHardQuit {
			m.shutdownSequence()
			return
		}
		m.dispatch(cmd)
		return
	}

	// Not a leader hotkey sequence, so it might be an arrow key (Esc + [) or alt key.
	// Flush the buffered Esc, then the current rune.
	m.passToDownstream(writer, 0x1B)
	m.passToDownstream(writer, r)
}

func (m *Manager) dispatch(cmd Command) {
	select {
	case m.Commands <- cmd:
	default:
	}
}

func (m *Manager) passToDownstream(writer *bufio.Writer, r rune) {
	if writer != nil {
		writer.WriteString(string(r))
	}
}

// ── ANSI Utilities ─────────────────────────────────────────────────────────

func FormatDebugBlock(title, content string) string {
	const (
		BgColor    = "\x1b[48;5;235m"
		DimText    = "\x1b[38;5;244m"
		AccentText = "\x1b[38;5;214m"
		Reset      = "\x1b[0m"
	)

	return fmt.Sprintf(
		"\n%s%s╭── [ %sDEBUG: %s%s ] %s\n%s%s%s\n%s%s╰────────────────────────────────────────────────%s\n",
		BgColor, AccentText, AccentText, title, AccentText, Reset,
		BgColor, DimText, content,
		BgColor, AccentText, Reset,
	)
}

func ClearTerminal() {
	fmt.Print("\x1b[2J\x1b[H")
}
