package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/designsystem"
)

// TUIStyles wraps Lip Gloss styles derived from design tokens.
// Used throughout the TUI for consistent styling.
type TUIStyles struct {
	// Backgrounds
	BGCanvas   lipgloss.Style
	BGSurface  lipgloss.Style
	BGElevated lipgloss.Style
	BGActive   lipgloss.Style

	// Text styles
	TextPrimary   lipgloss.Style
	TextSecondary lipgloss.Style
	TextTertiary  lipgloss.Style

	// Semantic colors
	AccentPrimary   lipgloss.Style
	AccentSecondary lipgloss.Style

	// Status states
	StatusSuccess lipgloss.Style
	StatusWarning lipgloss.Style
	StatusError   lipgloss.Style
	StatusInfo    lipgloss.Style
	StatusPending lipgloss.Style

	// Run states
	RunLive     lipgloss.Style
	RunBlocked  lipgloss.Style
	RunComplete lipgloss.Style

	// Tool states
	ToolActive   lipgloss.Style
	ToolComplete lipgloss.Style

	// Domain-specific
	MemoryInjected    lipgloss.Style
	SourceLinked      lipgloss.Style
	ArtifactGenerated lipgloss.Style

	// Typography (TUI styling-based)
	Title        lipgloss.Style
	Label        lipgloss.Style
	BodyText     lipgloss.Style
	MachineState lipgloss.Style

	// Borders
	BorderSubtle lipgloss.Style
	BorderStrong lipgloss.Style
}

// TokensToLipGloss converts design tokens to Lip Gloss styles.
// Creates a comprehensive style set for TUI use.
func TokensToLipGloss(tokens designsystem.ColorTokens) *TUIStyles {
	return &TUIStyles{
		// Backgrounds
		BGCanvas:   lipgloss.NewStyle().Background(lipgloss.Color(tokens.BG.Canvas)),
		BGSurface:  lipgloss.NewStyle().Background(lipgloss.Color(tokens.BG.Surface)),
		BGElevated: lipgloss.NewStyle().Background(lipgloss.Color(tokens.BG.Elevated)),
		BGActive:   lipgloss.NewStyle().Background(lipgloss.Color(tokens.BG.Active)),

		// Text styles
		TextPrimary:   lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Primary)),
		TextSecondary: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Secondary)),
		TextTertiary:  lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Tertiary)),

		// Semantic
		AccentPrimary:   lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Accent.Primary)).Bold(true),
		AccentSecondary: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Accent.Secondary)),

		// Status
		StatusSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Status.Success)),
		StatusWarning: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Status.Warning)),
		StatusError:   lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Status.Error)),
		StatusInfo:    lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Status.Info)),
		StatusPending: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Status.Pending)),

		// Run
		RunLive:     lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Run.Live)).Bold(true),
		RunBlocked:  lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Run.Blocked)),
		RunComplete: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Run.Complete)),

		// Tool
		ToolActive:   lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Tool.Active)).Bold(true),
		ToolComplete: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Tool.Complete)),

		// Domain-specific
		MemoryInjected:    lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Memory.Injected)),
		SourceLinked:      lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Source.Linked)),
		ArtifactGenerated: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Artifact.Generated)),

		// Typography (TUI styling-based)
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tokens.Text.Primary)),
		Label:        lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Secondary)).Faint(true),
		BodyText:     lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Primary)),
		MachineState: lipgloss.NewStyle().Foreground(lipgloss.Color(tokens.Text.Tertiary)).Faint(true),

		// Borders
		BorderSubtle: lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(tokens.Border.Subtle)),
		BorderStrong: lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(tokens.Border.Strong)),
	}
}

// ApplySpacing applies spacing from design tokens to a Lip Gloss style.
func ApplySpacing(style lipgloss.Style, padding, margin int) lipgloss.Style {
	if padding > 0 {
		style = style.Padding(padding)
	}
	if margin > 0 {
		style = style.Margin(margin)
	}
	return style
}

// ApplyBorder adds a border to a style using the given color.
func ApplyBorder(style lipgloss.Style, color string) lipgloss.Style {
	return style.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(color))
}

// PanelStyle creates a panel style with the given background and border color.
func PanelStyle(bgColor, borderColor string) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(1, 2)
}

// CardStyle creates a card style (elevated surface with subtle border).
func CardStyle(tokens designsystem.ColorTokens) lipgloss.Style {
	return PanelStyle(tokens.BG.Elevated, tokens.Border.Subtle)
}

// HighlightStyle creates a highlight style for active/focus states.
func HighlightStyle(tokens designsystem.ColorTokens) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(tokens.BG.Active)).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(tokens.Accent.Primary))
}
