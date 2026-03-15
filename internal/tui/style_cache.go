package tui

// style_cache.go — pre-allocated lipgloss styles for render hot-paths.
//
// Every lipgloss.NewStyle() call allocates a struct on the heap. Render
// functions called on every tick (loading indicator ~150ms, search bar on
// every keystroke, autocomplete popup, rewind menu) were each creating 3–5
// fresh Style objects per frame, generating constant GC pressure that caused
// visible "catch-up" pauses in output rendering.
//
// Styles that depend on runtime values (e.g. .Width(m.width)) still need
// partial construction per-call; we cache the base and append the dynamic
// field, which is a cheap value-type copy.

import "github.com/charmbracelet/lipgloss"

// ── renderLoadingIndicator ────────────────────────────────────────────────

var (
	loadingPhraseStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(GrokBlue)).
				Italic(false)

	loadingRowWrapper = lipgloss.NewStyle().Padding(0, 2)
)

// ── renderHookSection ─────────────────────────────────────────────────────

var hookSectionWrapper = lipgloss.NewStyle().PaddingLeft(2)

// ── renderInputHelp (generating badge) ───────────────────────────────────

// generatingBadge is the fully-rendered static string shown when the AI is
// streaming.  Pre-computed once since the text and styles never change.
var (
	generatingBadgeInner   = lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Italic(true)
	generatingBadgeWrapper = lipgloss.NewStyle().Padding(0, 1)
	generatingBadge        = generatingBadgeWrapper.Render(
		generatingBadgeInner.Render("generating… (Esc to cancel)"),
	)
)

// ── renderSearchBar / renderHistSearchBar ─────────────────────────────────

var (
	// searchBarBase is the base style; callers append .Width(n) for the
	// dynamic width — this is a value-type copy, much cheaper than NewStyle().
	searchBarBase = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	searchIconStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	searchCounterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	searchQueryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
)

// ── renderAtCompletePopup ─────────────────────────────────────────────────

var (
	// atCompleteBase is the border box base; callers set Width dynamically.
	atCompleteBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(GrokBlue)).
			Padding(0, 1)

	atCompleteItemNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
	atCompleteItemSelected = lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Bold(true)
	atCompleteWrapper      = lipgloss.NewStyle().PaddingLeft(2)
)

// ── renderRewindMenu ──────────────────────────────────────────────────────

var (
	// rewindBoxBase: callers append .Width(boxWidth).
	rewindBoxBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(GeminiPurple)).
			Padding(1, 2)

	rewindTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(GeminiPurple)).Bold(true)
	rewindHintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).Italic(true)
	rewindActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Bold(true)
	rewindDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
)
