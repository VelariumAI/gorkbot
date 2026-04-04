package designsystem

import (
	"fmt"
	"strings"
)

// ColorTokens defines all color values per the directive (section 6.6).
// Designed for dark-luxury technical modernism aesthetic.
type ColorTokens struct {
	// Core neutrals — base surfaces and borders
	BG struct {
		Canvas   string // #0a0a0a — darkest, main surface
		Surface  string // #141414 — elevated panels
		Elevated string // #1e1e1e — highest elevation
		Active   string // #2a2a2a — hover/focus state
	}
	Border struct {
		Subtle string // #2a2a2a — low contrast
		Strong string // #454545 — high contrast, active
	}
	Text struct {
		Primary   string // #f5f5f5 — main body text
		Secondary string // #a8a8a8 — labels, metadata
		Tertiary  string // #696969 — disabled, muted
	}

	// Semantic colors — meaningful intent
	Accent struct {
		Primary   string // #7c3aed — primary action, focus
		Secondary string // #6b21a8 — secondary action
	}
	Status struct {
		Success string // #10b981 — completion, health
		Warning string // #f59e0b — caution, attention
		Error   string // #ef4444 — errors, critical
		Info    string // #3b82f6 — informational
		Pending string // #8b5cf6 — in-progress
	}

	// State-specific colors — domain-aware semantic
	Run struct {
		Live     string // #7c3aed — computation active
		Blocked  string // #f59e0b — waiting/blocked
		Complete string // #10b981 — execution complete
	}
	Tool struct {
		Active   string // #7c3aed — tool executing
		Complete string // #10b981 — tool success
	}
	Memory struct {
		Injected string // #06b6d4 — context injected
	}
	Source struct {
		Linked string // #8b5cf6 — external reference
	}
	Artifact struct {
		Generated string // #ec4899 — AI-generated content
	}
}

// SpacingScale defines the spacing system (section 6.2).
// Uses multiples of 4 for consistency and terminal alignment.
type SpacingScale struct {
	Xs    int // 4 — minimal spacing
	Sm    int // 8 — small spacing
	Md    int // 12 — medium spacing
	Base  int // 16 — standard spacing
	Lg    int // 24 — large spacing
	Xl    int // 32 — extra large
	Xxl   int // 40 — 2x extra large
	Xxxl  int // 48 — 3x extra large
}

// RadiusTokens defines corner radius values (section 6.3).
type RadiusTokens struct {
	Compact  int // 8 — form inputs, compact elements
	Standard int // 12 — cards, panels (WebUI default)
	Large    int // 16 — hero cards, prominent surfaces
}

// ElevationTokens defines surface elevation levels (section 6.4).
// Three levels only — no visual mud.
type ElevationTokens struct {
	None    int // 0 — background
	Surface int // 1 — elevated panels
	Active  int // 2 — foreground overlays, live computation
}

// TypographyRole identifies semantic text roles (section 6.5).
type TypographyRole string

const (
	// WebUI typography roles
	RoleDisplay     TypographyRole = "display"       // Workspace title, 32px
	RoleSectionHead TypographyRole = "section_head"  // Section heading, 20px
	RoleCardTitle   TypographyRole = "card_title"    // Card title, 16px
	RoleBody        TypographyRole = "body"          // Main reading text, 14px
	RoleMeta        TypographyRole = "meta"          // Metadata, labels, 12px
	RoleMono        TypographyRole = "mono"          // Code, metrics, 13px
	RoleInlineCode  TypographyRole = "inline_code"   // Inline code, 13px

	// TUI typography roles (applied via styling, not font changes)
	RoleTitle        TypographyRole = "title"         // Workspace/section title
	RoleLabel        TypographyRole = "label"         // Field labels
	RoleBodyText     TypographyRole = "body_text"     // Main reading
	RoleMachineState TypographyRole = "machine_state" // System state
)

// TypographyDef defines a typography style.
type TypographyDef struct {
	Size          int     // Font size in pixels (WebUI) or relative (TUI)
	Weight        int     // 400=normal, 500=medium, 600=semibold, 700=bold
	LineHeight    float64 // Relative to font size (1.2, 1.5, etc.)
	LetterSpacing float64 // em units
}

// TypographyTokens defines all typography styles.
type TypographyTokens struct {
	// WebUI typography (actual font sizes and weights)
	Display     TypographyDef // 32px, 600, 1.2
	SectionHead TypographyDef // 20px, 600, 1.3
	CardTitle   TypographyDef // 16px, 600, 1.4
	Body        TypographyDef // 14px, 400, 1.5
	Meta        TypographyDef // 12px, 400, 1.4
	Mono        TypographyDef // 13px, 400, 1.4 (JetBrains Mono)
	InlineCode  TypographyDef // 13px, 500, 1.4 (JetBrains Mono)

	// TUI typography (styling-based, no font changes)
	Title        TypographyDef // Bold + bright
	Label        TypographyDef // Dim + normal
	BodyText     TypographyDef // Normal + normal
	MachineState TypographyDef // Dim + mono
}

// IconRole identifies semantic icon usage (section 6.7).
type IconRole string

const (
	IconChat      IconRole = "chat"
	IconTasks     IconRole = "tasks"
	IconTools     IconRole = "tools"
	IconAgents    IconRole = "agents"
	IconMemory    IconRole = "memory"
	IconAnalytics IconRole = "analytics"
	IconSettings  IconRole = "settings"
	IconRunLive   IconRole = "run_live"
)

// IconTokens maps icon roles to symbols.
type IconTokens struct {
	// TUI symbols
	ChatSymbol      string // "💬" or custom
	TasksSymbol     string // "📋" or custom
	ToolsSymbol     string // "🔧" or custom
	AgentsSymbol    string // "🤖" or custom
	MemorySymbol    string // "🧠" or custom
	AnalyticsSymbol string // "📊" or custom
	SettingsSymbol  string // "⚙️" or custom
	RunLiveSymbol   string // "⚡" or custom
}

// DensityMode controls information density and spacing.
type DensityMode string

const (
	DensityFocus        DensityMode = "focus"        // Spacious, minimal information
	DensityOperator     DensityMode = "operator"     // Balanced (default)
	DensityOrchestration DensityMode = "orchestration" // Dense, all information
)

// DensitySettings adjusts spacing and visibility per mode.
type DensitySettings struct {
	Mode                 DensityMode
	VerticalPadding      int  // Pixels
	HorizontalPadding    int  // Pixels
	ShowMetrics          bool
	ShowToolDetails      bool
	ShowMemoryBreakdown  bool
	ShowExecutionTrace   bool
}

// NewColorTokens returns a new ColorTokens with dark-luxury defaults.
func NewColorTokens() ColorTokens {
	ct := ColorTokens{}
	ct.BG.Canvas = "#0a0a0a"
	ct.BG.Surface = "#141414"
	ct.BG.Elevated = "#1e1e1e"
	ct.BG.Active = "#2a2a2a"
	ct.Border.Subtle = "#2a2a2a"
	ct.Border.Strong = "#454545"
	ct.Text.Primary = "#f5f5f5"
	ct.Text.Secondary = "#a8a8a8"
	ct.Text.Tertiary = "#696969"
	ct.Accent.Primary = "#7c3aed"
	ct.Accent.Secondary = "#6b21a8"
	ct.Status.Success = "#10b981"
	ct.Status.Warning = "#f59e0b"
	ct.Status.Error = "#ef4444"
	ct.Status.Info = "#3b82f6"
	ct.Status.Pending = "#8b5cf6"
	ct.Run.Live = "#7c3aed"
	ct.Run.Blocked = "#f59e0b"
	ct.Run.Complete = "#10b981"
	ct.Tool.Active = "#7c3aed"
	ct.Tool.Complete = "#10b981"
	ct.Memory.Injected = "#06b6d4"
	ct.Source.Linked = "#8b5cf6"
	ct.Artifact.Generated = "#ec4899"
	return ct
}

// NewSpacingScale returns a new SpacingScale with standard values.
func NewSpacingScale() SpacingScale {
	return SpacingScale{
		Xs:    4,
		Sm:    8,
		Md:    12,
		Base:  16,
		Lg:    24,
		Xl:    32,
		Xxl:   40,
		Xxxl:  48,
	}
}

// NewRadiusTokens returns a new RadiusTokens with standard values.
func NewRadiusTokens() RadiusTokens {
	return RadiusTokens{
		Compact:  8,
		Standard: 12,
		Large:    16,
	}
}

// NewElevationTokens returns a new ElevationTokens with standard values.
func NewElevationTokens() ElevationTokens {
	return ElevationTokens{
		None:    0,
		Surface: 1,
		Active:  2,
	}
}

// NewTypographyTokens returns a new TypographyTokens with standard values.
func NewTypographyTokens() TypographyTokens {
	return TypographyTokens{
		// WebUI typography
		Display:     TypographyDef{Size: 32, Weight: 600, LineHeight: 1.2, LetterSpacing: -0.01},
		SectionHead: TypographyDef{Size: 20, Weight: 600, LineHeight: 1.3, LetterSpacing: 0},
		CardTitle:   TypographyDef{Size: 16, Weight: 600, LineHeight: 1.4, LetterSpacing: 0},
		Body:        TypographyDef{Size: 14, Weight: 400, LineHeight: 1.5, LetterSpacing: 0},
		Meta:        TypographyDef{Size: 12, Weight: 400, LineHeight: 1.4, LetterSpacing: 0},
		Mono:        TypographyDef{Size: 13, Weight: 400, LineHeight: 1.4, LetterSpacing: 0.02},
		InlineCode:  TypographyDef{Size: 13, Weight: 500, LineHeight: 1.4, LetterSpacing: 0.02},
		// TUI typography (relative sizing via styling)
		Title:        TypographyDef{Weight: 700, LineHeight: 1.2},
		Label:        TypographyDef{Weight: 400, LineHeight: 1.4},
		BodyText:     TypographyDef{Weight: 400, LineHeight: 1.5},
		MachineState: TypographyDef{Weight: 400, LineHeight: 1.4},
	}
}

// NewIconTokens returns a new IconTokens with default symbols.
func NewIconTokens() IconTokens {
	return IconTokens{
		ChatSymbol:      "💬",
		TasksSymbol:     "📋",
		ToolsSymbol:     "🔧",
		AgentsSymbol:    "🤖",
		MemorySymbol:    "🧠",
		AnalyticsSymbol: "📊",
		SettingsSymbol:  "⚙️",
		RunLiveSymbol:   "⚡",
	}
}

// NewDensitySettings returns settings for the given density mode.
func NewDensitySettings(mode DensityMode) DensitySettings {
	switch mode {
	case DensityFocus:
		return DensitySettings{
			Mode:                DensityFocus,
			VerticalPadding:     24,
			HorizontalPadding:   24,
			ShowMetrics:         false,
			ShowToolDetails:     false,
			ShowMemoryBreakdown: false,
			ShowExecutionTrace:  false,
		}
	case DensityOrchestration:
		return DensitySettings{
			Mode:                DensityOrchestration,
			VerticalPadding:     12,
			HorizontalPadding:   12,
			ShowMetrics:         true,
			ShowToolDetails:     true,
			ShowMemoryBreakdown: true,
			ShowExecutionTrace:  true,
		}
	default: // DensityOperator
		return DensitySettings{
			Mode:                DensityOperator,
			VerticalPadding:     16,
			HorizontalPadding:   16,
			ShowMetrics:         true,
			ShowToolDetails:     false,
			ShowMemoryBreakdown: false,
			ShowExecutionTrace:  false,
		}
	}
}

// ValidateColors checks that all color hex values are valid.
func (ct ColorTokens) ValidateColors() error {
	colors := []struct {
		name  string
		value string
	}{
		{"bg.canvas", ct.BG.Canvas},
		{"bg.surface", ct.BG.Surface},
		{"bg.elevated", ct.BG.Elevated},
		{"bg.active", ct.BG.Active},
		{"border.subtle", ct.Border.Subtle},
		{"border.strong", ct.Border.Strong},
		{"text.primary", ct.Text.Primary},
		{"text.secondary", ct.Text.Secondary},
		{"text.tertiary", ct.Text.Tertiary},
		{"accent.primary", ct.Accent.Primary},
		{"accent.secondary", ct.Accent.Secondary},
		{"status.success", ct.Status.Success},
		{"status.warning", ct.Status.Warning},
		{"status.error", ct.Status.Error},
		{"status.info", ct.Status.Info},
		{"status.pending", ct.Status.Pending},
		{"run.live", ct.Run.Live},
		{"run.blocked", ct.Run.Blocked},
		{"run.complete", ct.Run.Complete},
		{"tool.active", ct.Tool.Active},
		{"tool.complete", ct.Tool.Complete},
		{"memory.injected", ct.Memory.Injected},
		{"source.linked", ct.Source.Linked},
		{"artifact.generated", ct.Artifact.Generated},
	}

	for _, c := range colors {
		if !isValidHex(c.value) {
			return fmt.Errorf("invalid hex color for %s: %s", c.name, c.value)
		}
	}

	return nil
}

// isValidHex checks if a string is a valid hex color.
func isValidHex(hex string) bool {
	if !strings.HasPrefix(hex, "#") {
		return false
	}
	if len(hex) != 7 {
		return false
	}
	for _, c := range hex[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ValidateTypography checks that typography definitions are sensible.
func (tt TypographyTokens) ValidateTypography() error {
	defs := []struct {
		name string
		def  TypographyDef
	}{
		{"display", tt.Display},
		{"section_head", tt.SectionHead},
		{"card_title", tt.CardTitle},
		{"body", tt.Body},
		{"meta", tt.Meta},
		{"mono", tt.Mono},
		{"inline_code", tt.InlineCode},
	}

	validWeights := map[int]bool{400: true, 500: true, 600: true, 700: true}

	for _, d := range defs {
		if d.def.Size > 0 && (d.def.Size < 8 || d.def.Size > 48) {
			return fmt.Errorf("invalid font size for %s: %d (must be 8-48)", d.name, d.def.Size)
		}
		if !validWeights[d.def.Weight] {
			return fmt.Errorf("invalid font weight for %s: %d (must be 400, 500, 600, or 700)", d.name, d.def.Weight)
		}
		if d.def.LineHeight < 1.0 || d.def.LineHeight > 3.0 {
			return fmt.Errorf("invalid line height for %s: %.2f (must be 1.0-3.0)", d.name, d.def.LineHeight)
		}
	}

	return nil
}
