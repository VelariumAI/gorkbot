package tui

import (
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"
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
	style.Code.StylePrimitive.BackgroundColor = stringPtr("#2D1212")      // Slightly lighter blood red
	style.Code.StylePrimitive.Color = stringPtr("#FF8080")                // Pale red text
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
func boolPtr(b bool) *bool { return &b }
func uintPtr(u uint) *uint { return &u }

// Color palette
const (
	// Primary colors
	GrokBlue      = "#00D9FF"
	GeminiPurple  = "#9945FF"
	GeminiPink    = "#FF0080"

	// UI colors
	BorderGray    = "#3C3C3C"
	TextWhite     = "#FFFFFF"
	TextGray      = "#888888"
	BgDark        = "#0A0A0A"
	BgDarkAlt     = "#1A1A1A"

	// Status colors
	SuccessGreen  = "#50FA7B" // Dracula Green
	ErrorRed      = "#FF5555" // Dracula Red
	WarningYellow = "#F1FA8C" // Dracula Yellow

	// Dracula Theme Colors (Official)
	DraculaBg       = "#282A36"
	DraculaFg       = "#F8F8F2"
	DraculaSelection = "#44475A"
	DraculaComment  = "#6272A4"
	DraculaRed      = "#FF5555"
	DraculaOrange   = "#FFB86C" // Kept for reference but avoiding for blocks
	DraculaYellow   = "#F1FA8C"
	DraculaGreen    = "#50FA7B"
	DraculaPurple   = "#BD93F9"
	DraculaCyan     = "#8BE9FD"
	DraculaPink     = "#FF79C6"
)

// Styles holds all Lip Gloss styles for the TUI
type Styles struct {
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
	ToolBox lipgloss.Style

	// Tabs
	Tab       lipgloss.Style
	ActiveTab lipgloss.Style
	TabGap    lipgloss.Style

	// Notifications
	Toast lipgloss.Style
}

// NewStyles creates a new Styles instance with default dark theme
func NewStyles() *Styles {
	s := &Styles{}

	// Consultant Box - Purple/Pink gradient border for Gemini advice
	s.ConsultantBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(GeminiPurple)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	// User message - subtle, left-aligned
	s.UserMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true).
		MarginBottom(1)

	// AI message - standard response
	s.AIMessage = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextWhite)).
		MarginBottom(1)

	// Status bar components
	s.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Background(lipgloss.Color(BgDarkAlt)).
		Padding(0, 1)

	s.StatusBarKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Bold(true)

	s.StatusBarValue = lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue))

	// Spinner styling
	s.Spinner = lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue))

	// Loading phrase
	s.Phrase = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Italic(true).
		MarginLeft(1)

	// Error message
	s.Error = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ErrorRed)).
		Bold(true).
		Padding(0, 1)

	// Help text
	s.Help = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Italic(true).
		Padding(1, 0)

	// App container
	s.App = lipgloss.NewStyle().
		Padding(1, 2)

	// Viewport - no border for cleaner look like Claude Code
	s.Viewport = lipgloss.NewStyle()

	// Input area - minimal styling
	s.InputArea = lipgloss.NewStyle()

	// Command output
	s.CommandOutput = lipgloss.NewStyle().
		Foreground(lipgloss.Color(SuccessGreen)).
		Padding(0, 1)

	// Tool execution box - Green border for successful tool executions
	s.ToolBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(SuccessGreen)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	// Tabs
	s.Tab = lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Padding(0, 1)

	s.ActiveTab = s.Tab.Copy().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color(GrokBlue))

	s.TabGap = lipgloss.NewStyle().
		Width(1).
		Foreground(lipgloss.Color(BorderGray)).
		SetString("|")

	// Notification Toast
	s.Toast = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(GrokBlue)).
		Padding(0, 1).
		Foreground(lipgloss.Color(TextWhite))

	return s
}

// UpdateForLightTheme updates styles for light theme
func (s *Styles) UpdateForLightTheme() {
	s.ConsultantBox = s.ConsultantBox.
		BorderForeground(lipgloss.Color(GeminiPurple))

	s.UserMessage = s.UserMessage.
		Foreground(lipgloss.Color("#0066CC"))

	s.AIMessage = s.AIMessage.
		Foreground(lipgloss.Color("#000000"))

	s.StatusBar = s.StatusBar.
		Foreground(lipgloss.Color("#555555")).
		Background(lipgloss.Color("#F0F0F0"))

	s.Spinner = s.Spinner.
		Foreground(lipgloss.Color("#0066CC"))

	s.Phrase = s.Phrase.
		Foreground(lipgloss.Color("#666666"))

	s.Error = s.Error.
		Foreground(lipgloss.Color("#CC0000"))

	s.Viewport = s.Viewport.
		BorderForeground(lipgloss.Color("#CCCCCC"))

	s.InputArea = s.InputArea.
		BorderForeground(lipgloss.Color("#CCCCCC"))
}

// UpdateForDarkTheme updates styles for dark theme
func (s *Styles) UpdateForDarkTheme() {
	*s = *NewStyles() // Reset to default dark theme
}

// UpdateForDraculaTheme updates styles for Dracula theme
func (s *Styles) UpdateForDraculaTheme() {
	s.ConsultantBox = s.ConsultantBox.
		BorderForeground(lipgloss.Color(DraculaPurple))

	s.UserMessage = s.UserMessage.
		Foreground(lipgloss.Color(DraculaCyan))

	s.AIMessage = s.AIMessage.
		Foreground(lipgloss.Color(DraculaFg))

	s.StatusBar = s.StatusBar.
		Foreground(lipgloss.Color(DraculaComment)).
		Background(lipgloss.Color(DraculaSelection))

	s.Spinner = s.Spinner.
		Foreground(lipgloss.Color(DraculaPurple))

	s.Phrase = s.Phrase.
		Foreground(lipgloss.Color(DraculaComment))

	s.Error = s.Error.
		Foreground(lipgloss.Color(DraculaRed))

	s.Viewport = s.Viewport.
		BorderForeground(lipgloss.Color(DraculaComment))

	s.InputArea = s.InputArea.
		BorderForeground(lipgloss.Color(DraculaComment))
		
	s.CommandOutput = s.CommandOutput.
		Foreground(lipgloss.Color(DraculaGreen))
		
	s.ToolBox = s.ToolBox.
		BorderForeground(lipgloss.Color(DraculaRed)) // Changed from Orange to Red for horror theme

	s.Tab = s.Tab.
		Foreground(lipgloss.Color(DraculaComment))

	s.ActiveTab = s.ActiveTab.
		Foreground(lipgloss.Color(DraculaCyan)).
		BorderForeground(lipgloss.Color(DraculaPink))

	s.Toast = s.Toast.
		BorderForeground(lipgloss.Color(DraculaPurple)).
		Foreground(lipgloss.Color(DraculaFg))
}

