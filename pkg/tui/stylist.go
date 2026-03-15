package tui

import (
	"fmt"
	"math/rand"
	"strings"
)

// ANSI Color Codes
const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"
)

// Unicode Characters
const (
	Vertical = "│"
	Branch   = "├"
	CornerTL = "╭"
	CornerTR = "╮"
	CornerBL = "╰"
	CornerBR = "╯"
	Success  = "✓"
	Failure  = "✗"
)

// Stylist handles the visual formatting of the CLI output
type Stylist struct {
	reasoningIcon string
	indent        int
}

// NewStylist creates a new Stylist with a random reasoning icon
func NewStylist() *Stylist {
	icons := []string{"🅖", "Ⓖ", "🇬"}
	icon := icons[rand.Intn(len(icons))]

	return &Stylist{
		reasoningIcon: icon,
		indent:        0,
	}
}

// PrintReasoning prints the AI's thought/reasoning
func (s *Stylist) PrintReasoning(text string) {
	if text == "" {
		return
	}
	// Clean up text
	text = strings.TrimSpace(text)

	fmt.Printf("\n%s%s %s%s\n", Magenta, s.reasoningIcon, text, Reset)
}

// StartActionBlock starts a visual block for actions
func (s *Stylist) StartActionBlock() {
	fmt.Printf("\n%s%s%s\n", Gray, CornerTL, strings.Repeat("─", 40)) // Header line
}

// LogToolExecution logs the start of a tool execution inside the block
func (s *Stylist) LogToolExecution(toolName, params string) {
	fmt.Printf("%s%s %sTool: %s%s %s%s%s\n",
		Gray, Vertical,
		Cyan, toolName, Reset,
		White, params, Reset)
}

// LogToolResult logs the result of a tool execution inside the block
func (s *Stylist) LogToolResult(success bool, outputSnippet string) {
	icon := Green + Success + Reset
	if !success {
		icon = Red + Failure + Reset
	}

	// Truncate output snippet if too long
	snippet := strings.TrimSpace(outputSnippet)
	if len(snippet) > 100 {
		snippet = snippet[:97] + "..."
	}
	// Escape newlines for single line display or handle multi-line indentation
	snippet = strings.ReplaceAll(snippet, "\n", " ")

	fmt.Printf("%s%s %s Result: %s\n", Gray, Vertical, icon, snippet)

	// Close the block segment
	fmt.Printf("%s%s%s\n", Gray, CornerBL, strings.Repeat("─", 40))
}

// CloseBlock closes any open visual blocks (if needed, though LogToolResult closes segments)
func (s *Stylist) CloseBlock() {
	// Optional: Use if we want to group multiple tools in one big block
	// For now, LogToolResult handles the closing of the immediate action.
}
