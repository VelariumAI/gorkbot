# Design System: Phase 1 Token System

This package provides the unified design token system for Gorkbot's TUI and WebUI, implementing the Gorkbot Premium UI Directive (section 6: Shared Design System).

## Overview

The designsystem package manages all design tokens used across both user-facing clients:

- **Colors** — 24 semantic colors for text, backgrounds, status, states, and domain-specific uses
- **Spacing** — 8-value scale (4px–48px) for consistent layout
- **Radius** — 3 corner radius values for WebUI elements
- **Elevation** — 3 surface levels (no visual mud)
- **Typography** — 7 WebUI roles + 4 TUI roles
- **Icons** — 8 semantic icon roles
- **Density Modes** — Focus / Operator (default) / Orchestration

## Quick Start

### Initialization

```go
import "github.com/velariumai/gorkbot/internal/designsystem"

func main() {
    // Initialize the global registry (once at startup)
    logger := slog.Default()
    err := designsystem.Init(logger)
    if err != nil {
        log.Fatal(err)
    }

    // Access tokens via the global registry
    reg := designsystem.Get()
    colors := reg.GetColors()
}
```

### TUI Usage

```go
import "github.com/velariumai/gorkbot/pkg/theme"
import "github.com/velariumai/gorkbot/internal/designsystem"

// Get token-based Lip Gloss styles
tokens := designsystem.Get().GetColors()
styles := theme.TokensToLipGloss(tokens)

// Use in rendering
message := styles.TextPrimary.Render("Hello")
statusText := styles.StatusSuccess.Render("✓ Complete")
```

### WebUI Usage

```go
import "github.com/velariumai/gorkbot/pkg/theme"
import "github.com/velariumai/gorkbot/internal/designsystem"

// Generate CSS variables for HTML
tokens := designsystem.Get().GetColors()
spacing := designsystem.Get().GetSpacing()
cssVars := theme.TokensToCSSVariables(tokens, spacing)

// Serve as stylesheet or inline in HTML
// <style>{{ cssVars }}</style>
// Use in CSS: background-color: var(--color-bg-canvas);
```

## Token Structure

### Colors (24 tokens)

**Core Neutrals** (4):
- `bg.canvas` (#0a0a0a) — Main background surface
- `bg.surface` (#141414) — Elevated panels
- `bg.elevated` (#1e1e1e) — Highest elevation
- `bg.active` (#2a2a2a) — Hover/focus state

**Text** (3):
- `text.primary` (#f5f5f5) — Body text
- `text.secondary` (#a8a8a8) — Labels, metadata
- `text.tertiary` (#696969) — Disabled, muted

**Semantic** (6):
- `accent.primary` (#7c3aed) — Primary actions, focus
- `accent.secondary` (#6b21a8) — Secondary actions
- `status.success` (#10b981) — Success state
- `status.warning` (#f59e0b) — Warning/attention
- `status.error` (#ef4444) — Errors
- `status.info` (#3b82f6) — Info messages
- `status.pending` (#8b5cf6) — In-progress

**State-Specific** (5):
- `run.live` (#7c3aed) — Active execution
- `run.blocked` (#f59e0b) — Blocked/waiting
- `run.complete` (#10b981) — Complete
- `tool.active` (#7c3aed) — Tool executing
- `tool.complete` (#10b981) — Tool success
- `memory.injected` (#06b6d4) — Memory context
- `source.linked` (#8b5cf6) — External source
- `artifact.generated` (#ec4899) — AI-generated

### Spacing (8 values)

| Token | Value | Use Cases |
|-------|-------|-----------|
| xs | 4px | Minimal spacing, tight groups |
| sm | 8px | Small gaps between elements |
| md | 12px | Compact sections |
| base | 16px | Standard padding/margin (primary) |
| lg | 24px | Large spacing between sections |
| xl | 32px | Extra large gaps |
| xxl | 40px | Page-level spacing |
| xxxl | 48px | Major layout sections |

### Radius (3 values)

- `compact` (8px) — Form inputs, tight elements
- `standard` (12px) — Cards, panels (default)
- `large` (16px) — Hero cards, prominent surfaces

### Elevation (3 levels)

- Level 0 — Background
- Level 1 — Elevated panels
- Level 2 — Active/live foreground overlays

### Typography

**WebUI Roles** (7):
- Display (32px, 600 weight) — Workspace titles
- SectionHead (20px, 600) — Section headings
- CardTitle (16px, 600) — Card titles
- Body (14px, 400) — Main reading text
- Meta (12px, 400) — Labels, metadata
- Mono (13px, 400) — Code, metrics, IDs
- InlineCode (13px, 500) — Inline code blocks

**TUI Roles** (4, via styling):
- Title — Workspace/section title
- Label — Field labels
- BodyText — Main reading
- MachineState — System state

### Density Modes

```go
// Switch between density modes
reg := designsystem.Get()
reg.SetDensity(designsystem.DensityFocus)        // Spacious
reg.SetDensity(designsystem.DensityOperator)     // Balanced (default)
reg.SetDensity(designsystem.DensityOrchestration) // Dense
```

**Focus Mode:**
- Vertical padding: 24px
- Horizontal padding: 24px
- Hides metrics, tool details, memory breakdown, execution trace

**Operator Mode (default):**
- Vertical padding: 16px
- Horizontal padding: 16px
- Shows metrics only
- Hides tool details and deeper diagnostics

**Orchestration Mode:**
- Vertical padding: 12px
- Horizontal padding: 12px
- Shows all metrics, tool details, memory breakdown, execution trace
- Maximum information density

## Implementation Reference

### File Structure

```
internal/designsystem/
├── tokens.go          # Token definitions (colors, spacing, typography, etc.)
├── tokens_test.go     # Token validation tests (19 tests)
├── registry.go        # Global token access and density management
├── registry_test.go   # Registry tests (13 tests)
└── README.md          # This file

pkg/theme/
├── tui_adapter.go     # Converts tokens to Lip Gloss styles
├── web_adapter.go     # Converts tokens to CSS variables
└── generator_test.go  # (future) Integration tests
```

### Key Functions

**Tokens Package:**
- `NewColorTokens()` → Returns default colors
- `NewSpacingScale()` → Returns spacing values
- `NewTypographyTokens()` → Returns typography definitions
- `NewDensitySettings(mode)` → Returns settings for density mode

**Registry Package:**
- `Init(logger)` → Initialize global registry (call once at startup)
- `Get()` → Access global registry
- `reg.GetColors()` / `GetSpacing()` / `GetTypography()` → Retrieve tokens
- `reg.SetDensity(mode)` → Change density mode
- `reg.GetDensitySettings()` → Get settings for current density

**Theme Adapters:**
- `TokensToLipGloss(colors)` → Generate Lip Gloss styles for TUI
- `TokensToCSSVariables(colors, spacing)` → Generate CSS variables for WebUI
- `TokensToScssVariables(colors, spacing)` → Generate SCSS for SCSS-based stylesheets

## Testing

All token definitions are validated:

```bash
go test ./internal/designsystem/... -v
# 32 tests passing
# - Color hex validation
# - Spacing scale validation
# - Typography weight validation
# - Density mode configuration
# - Registry access and updates
```

## CSS Usage Example (WebUI)

```html
<!-- Option 1: Inline CSS variables -->
<head>
  {{ TokensToCSSVariables }}
</head>
<body>
  <div style="background-color: var(--color-bg-canvas); color: var(--color-text-primary);">
    <h1 style="font-size: 32px; font-weight: 600;">Workspace Title</h1>
    <p style="padding: var(--space-base);">Content with standard spacing</p>
  </div>
</body>

<!-- Option 2: Serve as stylesheet -->
GET /api/theme/tokens.css → TokensToCSSVariables
```

## Lip Gloss Usage Example (TUI)

```go
styles := theme.TokensToLipGloss(designsystem.Get().GetColors())

// Simple text
fmt.Println(styles.TextPrimary.Render("Hello"))

// Colored status
fmt.Println(styles.StatusSuccess.Render("✓ Complete"))

// Card-like panel
card := theme.CardStyle(designsystem.Get().GetColors())
fmt.Println(card.Render("Content"))

// With spacing
padded := theme.ApplySpacing(styles.BodyText, 16, 0)
fmt.Println(padded.Render("Padded text"))
```

## Directive Compliance

✅ **Section 6.1** (Layout Grid) — Tokens support both 12-column (WebUI) and pane ratio (TUI) systems
✅ **Section 6.2** (Spacing) — 8-value scale, multiples of 4
✅ **Section 6.3** (Radius) — 3 radius values, TUI simulates via spacing
✅ **Section 6.4** (Elevation) — 3 levels only, no visual mud
✅ **Section 6.5** (Typography) — 7 WebUI + 4 TUI roles
✅ **Section 6.6** (Colors) — 24 semantic colors, dark-luxury palette
✅ **Section 6.7** (Icons) — 8 semantic icon roles
✅ **Section 2.5** (Same product, different medium) — Unified tokens, medium-specific adapters

## Next Steps (Phase 2+)

- **Phase 2:** WebUI shell overhaul (use tokens in layout)
- **Phase 3:** TUI shell overhaul (refactor view.go, style.go)
- **Phase 4:** Parity polish (ensure TUI/WebUI consistency)
- **Phase 5:** Signature moments (startup sequences, live computation)

## Notes

- All colors use 24-bit hex (#RRGGBB) for portability
- Spacing values are multiples of 4 for terminal alignment
- Density modes are stateful (set once, affects all subsequent renders)
- Registry is thread-safe for concurrent access
- No breaking changes to existing code (opt-in adoption)
