package tui

import (
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/designsystem"
	"github.com/velariumai/gorkbot/pkg/theme"
)

// CustomGlamourStyle returns a custom glamour style with light red code blocks
func CustomGlamourStyle() ansi.StyleConfig {
	style := ansi.StyleConfig{}

	// Document
	style.Document.BlockPrefix = "\n"
	style.Document.BlockSuffix = "\n"
	style.Document.Margin = uintPtr(0)

	// Code Block
	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.StylePrimitive.BackgroundColor = stringPtr("#1E0A0A") // Very dark blood red background
	style.CodeBlock.StylePrimitive.Color = stringPtr("#E6E6E6")           // Light gray text

	// Inline Code - Ensure no orange background here
	style.Code.StylePrimitive.BackgroundColor = stringPtr("#2D1212") // Slightly lighter blood red
	style.Code.StylePrimitive.Color = stringPtr("#FF8080")           // Pale red text
	// style.Code.StylePrimitive.Padding removed as it's not supported

	// Headers
	style.H1.Color = stringPtr("#FF5555") // Dracula Red
	style.H1.Bold = boolPtr(true)
	style.H1.BlockSuffix = "\n"

	style.H2.Color = stringPtr("#BD93F9") // Dracula Purple
	style.H2.Bold = boolPtr(true)
	style.H2.BlockSuffix = "\n"

	// Text
	style.Text.Color = stringPtr("#F8F8F2") // Dracula Foreground

	// Links
	style.Link.Color = stringPtr("#8BE9FD") // Dracula Cyan
	style.Link.Underline = boolPtr(true)

	return style
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }
func uintPtr(u uint) *uint       { return &u }

// Gorky identity
const (
	GorkyGlyph = "𝗚 ▸"
)

// Color palette
const (
	// Primary colors
	GrokBlue     = "#00D9FF"
	GeminiPurple = "#9945FF"
	GeminiPink   = "#FF0080"

	// UI colors
	BorderGray = "#3C3C3C"
	TextWhite  = "#FFFFFF"
	TextGray   = "#888888"
	BgDark     = "#0A0A0A"
	BgDarkAlt  = "#1A1A1A"

	// Status colors
	SuccessGreen  = "#50FA7B" // Dracula Green
	ErrorRed      = "#FF5555" // Dracula Red
	WarningYellow = "#F1FA8C" // Dracula Yellow

	// Dracula Theme Colors (Official)
	DraculaBg        = "#282A36"
	DraculaFg        = "#F8F8F2"
	DraculaSelection = "#44475A"
	DraculaComment   = "#6272A4"
	DraculaRed       = "#FF5555"
	DraculaOrange    = "#FFB86C" // Kept for reference but avoiding for blocks
	DraculaYellow    = "#F1FA8C"
	DraculaGreen     = "#50FA7B"
	DraculaPurple    = "#BD93F9"
	DraculaCyan      = "#8BE9FD"
	DraculaPink      = "#FF79C6"
)

// ── Arcane Blood Theme Colors ─────────────────────────────────────────────
// A dark crimson / ember palette designed for the mystical Arcane Forge look.
const (
	ArcaneBackground = "#0D0000" // near-black blood background
	ArcaneAlt        = "#1A0000" // slightly lighter alt background
	ArcanePrimary    = "#CC0000" // vivid blood red — primary accent
	ArcaneEmber      = "#FF3300" // bright ember / active glow
	ArcaneDim        = "#440000" // muted shadow for particle effects
	ArcaneGold       = "#B8860B" // sigil gold for borders and headings
	ArcaneGoldBright = "#FFD700" // bright gold for active elements
	ArcaneText       = "#FFD0D0" // warm off-white foreground
	ArcaneSubtext    = "#884444" // dimmed text / comments
	ArcaneSelection  = "#3A0A0A" // selection background
	ArcaneBorder     = "#5C1010" // border color
	ArcaneSuccess    = "#50FA7B" // keep Dracula green for success
	ArcaneError      = "#FF5555" // Dracula red for errors
	ArcaneWarn       = "#FFB86C" // amber for warnings
)

// ── Dashboard Sidebar Color Palette ───────────────────────────────────────
const (
	DashFgMuted      = "#A3A7AD" // muted metadata text
	DashMainGreen    = "#4AFEAD" // section header glyph+text (vibrant green)
	DashRuleCharcoal = "#2D333D" // horizontal rule fill
	DashBulletYellow = "#FEB800" // active/running agent bullet
	DashBulletBlue   = "#56B6F7" // pending/idle agent bullet
	DashBulletWhite  = "#FFFFFF" // completed checkmark
	DashBarMagenta   = "#FC107A" // top tool bar (rank 1)
	DashBarPurple    = "#8C54FB" // mid tool bar (rank 2)
	DashBarTeal      = "#29A6A3" // bottom tool bar (rank 3+)
	DashPillBg       = "#A8A3E7" // intent pill background (lavender)
	DashPillFg       = "#1A1D24" // intent pill text (dark on lavender)
)

// Styles holds all Lip Gloss styles for the TUI
type HookStyles struct {
	Bullet   lipgloss.Style
	Task     lipgloss.Style
	Meta     lipgloss.Style
	Hook     lipgloss.Style
	Pulse    lipgloss.Style
	FoldIcon string
	Particle lipgloss.Style
}

type Styles struct {
	// Hook styles
	Hook HookStyles

	// HITL approval overlay border style (theme-aware)
	HITL lipgloss.Style

	// Consultant box - distinctive styling for Gemini responses
	ConsultantBox lipgloss.Style

	// User message styling
	UserMessage lipgloss.Style

	// AI message styling
	AIMessage lipgloss.Style

	// Status bar at the bottom
	StatusBar      lipgloss.Style
	StatusBarKey   lipgloss.Style
	StatusBarValue lipgloss.Style

	// Loading spinner and phrase
	Spinner lipgloss.Style
	Phrase  lipgloss.Style

	// Error message
	Error lipgloss.Style

	// Help text
	Help lipgloss.Style

	// App container
	App lipgloss.Style

	// Viewport
	Viewport lipgloss.Style

	// Text input area
	InputArea lipgloss.Style

	// Command output
	CommandOutput lipgloss.Style

	// Tool execution box
	ToolBox       lipgloss.Style
	ToolBoxActive lipgloss.Style

	// Tabs
	Tab       lipgloss.Style
	ActiveTab lipgloss.Style
	TabGap    lipgloss.Style

	// Notifications
	Toast lipgloss.Style

	// Gorky identity glyph styling (𝗚 ▸)
	GorkyGlyphStyle lipgloss.Style

	// TokenStyles holds semantic styles from design tokens.
	// Nil when designsystem is not initialized — always nil-check before use.
	TokenStyles *theme.TUIStyles
}

// tryGetTokenStyles safely retrieves TUIStyles from design tokens.
// Returns nil if designsystem is not initialized.
// Uses recover() to prevent panics.
func tryGetTokenStyles() *theme.TUIStyles {
	defer func() { recover() }()
	return theme.TokensToLipGloss(designsystem.Get().GetColors())
}

// NewStyles creates a new Styles instance with the given theme.
// If theme is nil, uses Dracula as default.
func NewStyles(t *theme.Theme) *Styles {
	s := &Styles{}

	// Use provided theme or fall back to defaults
	var colors theme.Colors
	if t != nil {
		colors = t.Colors
	} else {
		// Default Dracula colors if no theme provided
		colors = theme.Colors{
			Primary:      GrokBlue,
			Secondary:    GeminiPurple,
			Border:       BorderGray,
			Text:         DraculaFg,
			TextDim:      DraculaComment,
			Success:      SuccessGreen,
			Error:        ErrorRed,
			Warning:      WarningYellow,
			CodeBg:       "#1E0A0A",
			CodeFg:       "#E6E6E6",
			InlineCodeBg: "#2D1212",
			InlineCodeFg: "#FF8080",
			Header1:      DraculaRed,
			Header2:      DraculaPurple,
			Link:         DraculaCyan,
			StatusBg:     DraculaBg,
			StatusFg:     DraculaFg,
		}
	}

	// Hook styles using theme colors
	s.Hook.Bullet = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Primary)).Bold(true)
	s.Hook.Task = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Text)).Bold(true)
	s.Hook.Meta = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.TextDim)).Italic(true)
	s.Hook.Hook = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Border))
	s.Hook.Pulse = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.Primary)).Blink(true)
	s.Hook.FoldIcon = "▶"
	s.Hook.Particle = lipgloss.NewStyle().Foreground(lipgloss.Color(colors.TextDim))

	// HITL approval overlay - uses theme warning color
	s.HITL = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colors.Warning)).
		Padding(1, 2)

	// Consultant Box - Uses theme secondary color for Gemini advice
	s.ConsultantBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colors.Secondary)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	// User message - uses theme primary color
	s.UserMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Primary)).
		Bold(true).
		MarginBottom(1)

	// AI message - uses theme text color
	s.AIMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Text)).
		MarginBottom(1)

	// Status bar components - uses theme status colors
	s.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.StatusFg)).
		Background(lipgloss.Color(colors.StatusBg)).
		Padding(0, 1)

	s.StatusBarKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.TextDim)).
		Bold(true)

	s.StatusBarValue = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Primary))

	// Spinner styling - uses theme primary color
	s.Spinner = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Primary))

	// Loading phrase - uses theme text dim color
	s.Phrase = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.TextDim)).
		Italic(true).
		MarginLeft(1)

	// Error message - uses theme error color
	s.Error = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Error)).
		Bold(true).
		Padding(0, 1)

	// Help text - uses theme text dim color
	s.Help = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.TextDim)).
		Italic(true).
		Padding(1, 0)

	// App container
	s.App = lipgloss.NewStyle().
		Padding(1, 2)

	// Viewport - no border for cleaner look like Claude Code
	s.Viewport = lipgloss.NewStyle()

	// Input area - minimal styling
	s.InputArea = lipgloss.NewStyle()

	// Command output - uses theme success color
	s.CommandOutput = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Success)).
		Padding(0, 1)

	// Tool execution box - Success border for successful tool executions
	s.ToolBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colors.Success)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	s.ToolBoxActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colors.Primary)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1).
		Blink(true)

	// Tabs
	s.Tab = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.TextDim)).
		Padding(0, 1)

	s.ActiveTab = s.Tab.Copy().
		Foreground(lipgloss.Color(colors.Primary)).
		Bold(true).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(colors.Primary))

	s.TabGap = lipgloss.NewStyle().
		Width(1).
		Foreground(lipgloss.Color(colors.Border)).
		SetString("|")

	// Notification Toast - uses theme primary color
	s.Toast = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colors.Primary)).
		Padding(0, 1).
		Foreground(lipgloss.Color(colors.Text))

	// Gorky glyph - uses theme primary color
	s.GorkyGlyphStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.Primary)).
		Bold(true)

	// Wire design token styles
	s.TokenStyles = tryGetTokenStyles()

	return s
}

// RemapForTheme updates styles for a new theme.
// Called when the user switches themes to refresh TUI styles without restarting.
func (s *Styles) RemapForTheme(t *theme.Theme) {
	// Simply replace the receiver with fresh styles from new theme
	newStyles := NewStyles(t)
	*s = *newStyles
}

// NewArcaneBloodStyles creates a brand-new Styles instance with the full
// Arcane Blood palette applied.  It does NOT mutate an existing Styles; the
// caller must replace m.styles with the returned pointer so that lipgloss
// immutability is respected.
func NewArcaneBloodStyles() *Styles {
	s := &Styles{}

	s.Hook.Bullet = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // bright blood ●
	s.Hook.Task = lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true)   // medium red
	s.Hook.Meta = lipgloss.NewStyle().Foreground(lipgloss.Color("88")).Italic(true)  // dark red
	s.Hook.Hook = lipgloss.NewStyle().Foreground(lipgloss.Color("52"))               // deepest red ⎿
	s.Hook.Pulse = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Blink(true)
	s.Hook.FoldIcon = "🩸"
	s.Hook.Particle = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))

	s.HITL = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ArcaneGold)).
		Padding(1, 2)

	s.ConsultantBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ArcaneGold)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	s.UserMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneEmber)).
		Bold(true).
		MarginBottom(1)

	s.AIMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneText)).
		MarginBottom(1)

	s.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSubtext)).
		Background(lipgloss.Color(ArcaneSelection)).
		Padding(0, 1)

	s.StatusBarKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSubtext)).
		Bold(true)

	s.StatusBarValue = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneEmber))

	s.Spinner = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneEmber))

	s.Phrase = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSubtext)).
		Italic(true).
		MarginLeft(1)

	s.Error = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneError)).
		Bold(true).
		Padding(0, 1)

	s.Help = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSubtext)).
		Italic(true).
		Padding(1, 0)

	s.App = lipgloss.NewStyle().
		Background(lipgloss.Color(ArcaneBackground)).
		Padding(1, 2)

	s.Viewport = lipgloss.NewStyle()

	s.InputArea = lipgloss.NewStyle()

	s.CommandOutput = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSuccess)).
		Padding(0, 1)

	s.ToolBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ArcanePrimary)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	s.ToolBoxActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ArcaneEmber)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1).
		Blink(true)

	s.Tab = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneSubtext)).
		Padding(0, 1)

	s.ActiveTab = s.Tab.Copy().
		Foreground(lipgloss.Color(ArcaneGoldBright)).
		Bold(true).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(ArcaneEmber))

	s.TabGap = lipgloss.NewStyle().
		Width(1).
		Foreground(lipgloss.Color(ArcaneBorder)).
		SetString("|")

	s.Toast = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ArcanePrimary)).
		Padding(0, 1).
		Foreground(lipgloss.Color(ArcaneText))

	// Gorky glyph - Arcane Blood theme uses gold
	s.GorkyGlyphStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ArcaneGold)).
		Bold(true)

	// Wire design token styles
	s.TokenStyles = tryGetTokenStyles()

	return s
}

// UpdateForArcaneBloodTheme replaces every field in the receiver with the
// Arcane Blood palette.  Must be called when m.theme == "arcane-blood".
func (s *Styles) UpdateForArcaneBloodTheme() {
	fresh := NewArcaneBloodStyles()
	*s = *fresh
}
