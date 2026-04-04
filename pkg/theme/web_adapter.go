package theme

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/internal/designsystem"
)

// TokensToCSSVariables converts design tokens to CSS custom properties (variables).
// Output is suitable for inclusion in HTML <head> or served as a stylesheet.
// Format follows CSS standard for root-level custom properties.
func TokensToCSSVariables(tokens designsystem.ColorTokens, spacing designsystem.SpacingScale) string {
	var sb strings.Builder

	sb.WriteString(":root {\n")

	// Color variables
	sb.WriteString("  /* Core neutrals */\n")
	sb.WriteString(fmt.Sprintf("  --color-bg-canvas: %s;\n", tokens.BG.Canvas))
	sb.WriteString(fmt.Sprintf("  --color-bg-surface: %s;\n", tokens.BG.Surface))
	sb.WriteString(fmt.Sprintf("  --color-bg-elevated: %s;\n", tokens.BG.Elevated))
	sb.WriteString(fmt.Sprintf("  --color-bg-active: %s;\n", tokens.BG.Active))

	sb.WriteString("  /* Borders */\n")
	sb.WriteString(fmt.Sprintf("  --color-border-subtle: %s;\n", tokens.Border.Subtle))
	sb.WriteString(fmt.Sprintf("  --color-border-strong: %s;\n", tokens.Border.Strong))

	sb.WriteString("  /* Text */\n")
	sb.WriteString(fmt.Sprintf("  --color-text-primary: %s;\n", tokens.Text.Primary))
	sb.WriteString(fmt.Sprintf("  --color-text-secondary: %s;\n", tokens.Text.Secondary))
	sb.WriteString(fmt.Sprintf("  --color-text-tertiary: %s;\n", tokens.Text.Tertiary))

	sb.WriteString("  /* Accent */\n")
	sb.WriteString(fmt.Sprintf("  --color-accent-primary: %s;\n", tokens.Accent.Primary))
	sb.WriteString(fmt.Sprintf("  --color-accent-secondary: %s;\n", tokens.Accent.Secondary))

	sb.WriteString("  /* Status */\n")
	sb.WriteString(fmt.Sprintf("  --color-status-success: %s;\n", tokens.Status.Success))
	sb.WriteString(fmt.Sprintf("  --color-status-warning: %s;\n", tokens.Status.Warning))
	sb.WriteString(fmt.Sprintf("  --color-status-error: %s;\n", tokens.Status.Error))
	sb.WriteString(fmt.Sprintf("  --color-status-info: %s;\n", tokens.Status.Info))
	sb.WriteString(fmt.Sprintf("  --color-status-pending: %s;\n", tokens.Status.Pending))

	sb.WriteString("  /* Run states */\n")
	sb.WriteString(fmt.Sprintf("  --color-run-live: %s;\n", tokens.Run.Live))
	sb.WriteString(fmt.Sprintf("  --color-run-blocked: %s;\n", tokens.Run.Blocked))
	sb.WriteString(fmt.Sprintf("  --color-run-complete: %s;\n", tokens.Run.Complete))

	sb.WriteString("  /* Tool states */\n")
	sb.WriteString(fmt.Sprintf("  --color-tool-active: %s;\n", tokens.Tool.Active))
	sb.WriteString(fmt.Sprintf("  --color-tool-complete: %s;\n", tokens.Tool.Complete))

	sb.WriteString("  /* Domain-specific */\n")
	sb.WriteString(fmt.Sprintf("  --color-memory-injected: %s;\n", tokens.Memory.Injected))
	sb.WriteString(fmt.Sprintf("  --color-source-linked: %s;\n", tokens.Source.Linked))
	sb.WriteString(fmt.Sprintf("  --color-artifact-generated: %s;\n", tokens.Artifact.Generated))

	// Spacing variables
	sb.WriteString("\n  /* Spacing scale */\n")
	sb.WriteString(fmt.Sprintf("  --space-xs: %dpx;\n", spacing.Xs))
	sb.WriteString(fmt.Sprintf("  --space-sm: %dpx;\n", spacing.Sm))
	sb.WriteString(fmt.Sprintf("  --space-md: %dpx;\n", spacing.Md))
	sb.WriteString(fmt.Sprintf("  --space-base: %dpx;\n", spacing.Base))
	sb.WriteString(fmt.Sprintf("  --space-lg: %dpx;\n", spacing.Lg))
	sb.WriteString(fmt.Sprintf("  --space-xl: %dpx;\n", spacing.Xl))
	sb.WriteString(fmt.Sprintf("  --space-xxl: %dpx;\n", spacing.Xxl))
	sb.WriteString(fmt.Sprintf("  --space-xxxl: %dpx;\n", spacing.Xxxl))

	sb.WriteString("}\n")

	return sb.String()
}

// TokensToScssVariables converts design tokens to SCSS variable syntax.
// Useful for SCSS-based stylesheets.
func TokensToScssVariables(tokens designsystem.ColorTokens, spacing designsystem.SpacingScale) string {
	var sb strings.Builder

	sb.WriteString("// Design System Tokens (SCSS)\n")
	sb.WriteString("// Auto-generated from designsystem package\n\n")

	sb.WriteString("// Core Neutrals\n")
	sb.WriteString(fmt.Sprintf("$color-bg-canvas: %s;\n", tokens.BG.Canvas))
	sb.WriteString(fmt.Sprintf("$color-bg-surface: %s;\n", tokens.BG.Surface))
	sb.WriteString(fmt.Sprintf("$color-bg-elevated: %s;\n", tokens.BG.Elevated))
	sb.WriteString(fmt.Sprintf("$color-bg-active: %s;\n", tokens.BG.Active))

	sb.WriteString("\n// Borders\n")
	sb.WriteString(fmt.Sprintf("$color-border-subtle: %s;\n", tokens.Border.Subtle))
	sb.WriteString(fmt.Sprintf("$color-border-strong: %s;\n", tokens.Border.Strong))

	sb.WriteString("\n// Text\n")
	sb.WriteString(fmt.Sprintf("$color-text-primary: %s;\n", tokens.Text.Primary))
	sb.WriteString(fmt.Sprintf("$color-text-secondary: %s;\n", tokens.Text.Secondary))
	sb.WriteString(fmt.Sprintf("$color-text-tertiary: %s;\n", tokens.Text.Tertiary))

	sb.WriteString("\n// Spacing Scale\n")
	sb.WriteString(fmt.Sprintf("$space-xs: %dpx;\n", spacing.Xs))
	sb.WriteString(fmt.Sprintf("$space-sm: %dpx;\n", spacing.Sm))
	sb.WriteString(fmt.Sprintf("$space-base: %dpx;\n", spacing.Base))
	sb.WriteString(fmt.Sprintf("$space-lg: %dpx;\n", spacing.Lg))

	return sb.String()
}

// GenerateThemeCSSHTML generates a complete CSS theme suitable for HTML injection.
func GenerateThemeCSSHTML(tokens designsystem.ColorTokens, spacing designsystem.SpacingScale) string {
	return fmt.Sprintf("<style type=\"text/css\">\n%s\n</style>", TokensToCSSVariables(tokens, spacing))
}
