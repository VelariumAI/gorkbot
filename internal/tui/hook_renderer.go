// Package tui — hook_renderer.go
//
// Renders the live hook output tree in Claude Code style: each HookEntry is
// one line with an icon, label, metadata, and optional indented output
// preview.  Nested SubEntries are indented by two spaces per depth level.
//
// Architecture notes:
//   - All rendering is purely functional (no state mutation).
//   - Depth is capped at maxHookDepth (5) both here and during ingestion.
//   - The embedded spinner is a simple rotating glyph array driven by
//     m.hookSpinFrame (advanced 150 ms in HookTickMsg) — no separate tick.
//   - Width constraints are enforced via lipgloss.Width + truncate() to
//     prevent wrapping on narrow terminals.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// hookSpinFrames are the rotating glyphs shown while a hook is Active.
// Deliberately sparse so the "active" glyph doesn't visually dominate.
var hookSpinFrames = []string{
	"◐", "◓", "◑", "◒",
}

// hookSuccessIcon is shown when a hook completes successfully.
const hookSuccessIcon = "✓"

// hookErrorIcon is shown when a hook fails.
const hookErrorIcon = "✗"

// hookPendingIcon is shown for sub-entries that haven't started yet.
const hookPendingIcon = "·"

// ── Color palette for hook renderer ─────────────────────────────────────────

const (
	hookColorActive  = "#00D9FF" // GrokBlue — currently running
	hookColorSuccess = "#50FA7B" // DraculaGreen — completed OK
	hookColorError   = "#FF5555" // DraculaRed — failed
	hookColorLabel   = "#F8F8F2" // DraculaFg — label text
	hookColorMeta    = "#6272A4" // DraculaComment — metadata/elapsed
	hookColorOutput  = "#44475A" // DraculaSelection — output preview
	hookColorIndent  = "#3C3C3C" // BorderGray — indent connector
	hookColorFold    = "#888888" // TextGray — fold indicator
)

// hookIndentUnit is the two-character indent per depth level (legacy fallback).
const hookIndentUnit = "  "

// Tree connector characters for ├─/└─ style rendering.
const (
	hookConnectorMid  = "├─ " // non-last child
	hookConnectorLast = "└─ " // last child
	hookConnectorBar  = "│  " // continuing ancestor
	hookConnectorGap  = "   " // terminated ancestor
)

// ── RenderHookTree renders the full hook tree ─────────────────────────────

// hookConnectors holds pre-rendered connector strings for one RenderHookTree
// call.  Rendering the lipgloss style onto each constant connector once per
// tree call (4 renders) instead of once per node × depth eliminates repeated
// style.Render() allocations inside the recursion.
type hookConnectors struct {
	mid  string // ├─
	last string // └─
	bar  string // │
	gap  string //
}

// RenderHookTree renders all top-level HookEntry items and their children
// into a multi-line string.  maxWidth is the available terminal width;
// spinFrame is the current animation frame index (from m.hookSpinFrame).
//
// Returns "" when hooks is nil/empty.
func RenderHookTree(hooks []HookEntry, maxWidth, spinFrame int, styles HookStyles) string {
	if len(hooks) == 0 {
		return ""
	}
	// Pre-render connector strings once for the entire tree traversal.
	conn := hookConnectors{
		mid:  styles.Hook.Render(hookConnectorMid),
		last: styles.Hook.Render(hookConnectorLast),
		bar:  styles.Hook.Render(hookConnectorBar),
		gap:  styles.Hook.Render(hookConnectorGap),
	}
	var sb strings.Builder
	for i := range hooks {
		isLast := i == len(hooks)-1
		renderHookNode(&sb, &hooks[i], 0, isLast, nil, maxWidth, spinFrame, styles, conn)
	}
	result := sb.String()
	// Trim trailing newline for clean joining.
	return strings.TrimRight(result, "\n")
}

// renderHookNode recursively renders one hook node and its children.
func renderHookNode(sb *strings.Builder, h *HookEntry, depth int, isLast bool, lastFlags []bool, maxWidth, spinFrame int, styles HookStyles, conn hookConnectors) {
	if depth > maxHookDepth {
		return
	}
	sb.WriteString(renderHookLine(h, depth, isLast, lastFlags, maxWidth, spinFrame, styles, conn))
	sb.WriteString("\n")

	// Output preview: at most 2 lines, indented one level deeper.
	if !h.Collapsed && h.Output != "" {
		childFlags := append(lastFlags, isLast)
		outIndent := buildIndentStr(depth+1, true, childFlags, conn)
		outStyle := styles.Particle
		lines := strings.SplitN(h.Output, "\n", 3)
		for i, line := range lines {
			if i >= 2 {
				ellipsis := outIndent + styles.Meta.Render("…")
				sb.WriteString(ellipsis)
				sb.WriteString("\n")
				break
			}
			if line == "" {
				continue
			}
			avail := maxWidth - lipgloss.Width(outIndent) - 2
			if avail < 4 {
				avail = 4
			}
			rendered := outIndent + outStyle.Render(truncate(line, avail))
			sb.WriteString(rendered)
			sb.WriteString("\n")
		}
	}

	// Children
	if !h.Collapsed {
		childFlags := append(lastFlags, isLast)
		for i := range h.SubEntries {
			childDepth := depth + 1
			if childDepth > maxHookDepth {
				break
			}
			childIsLast := i == len(h.SubEntries)-1
			renderHookNode(sb, &h.SubEntries[i], childDepth, childIsLast, childFlags, maxWidth, spinFrame, styles, conn)
		}
	} else if len(h.SubEntries) > 0 {
		// Collapsed indicator
		childFlags := append(lastFlags, isLast)
		indent := buildIndentStr(depth+1, true, childFlags, conn)
		foldIcon := styles.FoldIcon
		if foldIcon == "" {
			foldIcon = "▶"
		}
		sb.WriteString(indent)
		sb.WriteString(styles.Hook.Render(fmt.Sprintf("%s %d hidden", foldIcon, len(h.SubEntries))))
		sb.WriteString("\n")
	}
}

// renderHookLine renders a single hook entry as one terminal line.
func renderHookLine(h *HookEntry, depth int, isLast bool, lastFlags []bool, maxWidth, spinFrame int, styles HookStyles, conn hookConnectors) string {
	indent := buildIndentStr(depth, isLast, lastFlags, conn)
	indentW := lipgloss.Width(indent)

	// Icon
	icon, isError := resolveHookIcon(h, spinFrame)
	var iconStr string
	if !h.IsFinal && h.Active {
		iconStr = styles.Pulse.Render(icon)
	} else {
		iconStr = styles.Bullet.Render(icon)
	}

	// Label
	labelStyle := styles.Task
	if isError {
		labelStyle = labelStyle.Foreground(lipgloss.Color(hookColorError))
	}

	// Metadata (elapsed, progress, etc.)
	metaStr := ""
	if h.Metadata != "" {
		metaStr = " " + styles.Meta.Render(h.Metadata)
	} else if h.IsFinal && h.Elapsed > 0 {
		metaStr = " " + styles.Meta.Render(formatElapsed(h.Elapsed))
	}

	// Calculate available width for label.
	// indent(N) + icon(1) + space(1) + label + meta
	fixedW := indentW + 1 + 1 + lipgloss.Width(metaStr)
	availLabel := maxWidth - fixedW - 2
	if availLabel < 4 {
		availLabel = 4
	}

	labelStr := labelStyle.Render(truncate(h.Label, availLabel))

	// Fold indicator for collapsed entries with children.
	foldStr := ""
	if h.Collapsed && len(h.SubEntries) > 0 {
		foldIcon := styles.FoldIcon
		if foldIcon == "" {
			foldIcon = "▶"
		}
		foldStr = styles.Hook.Render(" " + foldIcon)
	}

	return indent + iconStr + " " + labelStr + foldStr + metaStr
}

// buildIndentStr creates the ├─/└─ style indent string for a given depth.
// isLast indicates whether h is the last sibling at its depth.
// lastFlags records whether each ancestor was the last child at its level.
// conn holds the four pre-rendered connector strings (computed once per tree).
func buildIndentStr(depth int, isLast bool, lastFlags []bool, conn hookConnectors) string {
	if depth == 0 {
		return ""
	}
	var sb strings.Builder
	// Ancestor levels: │  for continuing, "   " for terminated.
	for _, wasLast := range lastFlags {
		if wasLast {
			sb.WriteString(conn.gap)
		} else {
			sb.WriteString(conn.bar)
		}
	}
	// Current level connector.
	if isLast {
		sb.WriteString(conn.last)
	} else {
		sb.WriteString(conn.mid)
	}
	return sb.String()
}

// resolveHookIcon returns the glyph and error status for a hook entry.
func resolveHookIcon(h *HookEntry, spinFrame int) (string, bool) {
	if !h.IsFinal && h.Active {
		// Spinning glyph while active.
		frame := spinFrame % len(hookSpinFrames)
		return hookSpinFrames[frame], false
	}
	if h.IsFinal {
		if h.IsError {
			return hookErrorIcon, true
		}
		return hookSuccessIcon, false
	}
	// Pending (not yet started).
	return hookPendingIcon, false
}

// formatElapsed formats a duration as a compact string like "0.3s" or "1m2s".
func formatElapsed(d time.Duration) string {
	if d < time.Millisecond {
		return "0ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

// ── Hook tree header ─────────────────────────────────────────────────────────

// RenderHookHeader renders a thin separator + "Actions" label above the hook
// tree, only when there are active or recent hooks to show.
func RenderHookHeader(width int, styles HookStyles) string {
	label := styles.Meta.Bold(false).Render("  actions")
	sep := styles.Hook.Render(strings.Repeat("─", width-lipgloss.Width(label)-1))
	return label + sep
}

// ── Active-hook summary for side panel ───────────────────────────────────────

// RenderHookSummary renders a compact list of running hooks suitable for the
// side panel (innerW wide).  At most maxLines hooks are shown.
func RenderHookSummary(hooks []HookEntry, innerW, spinFrame, maxLines int, styles HookStyles) string {
	if len(hooks) == 0 {
		return ""
	}

	var lines []string
	for _, h := range hooks {
		if len(lines) >= maxLines {
			break
		}
		icon, isError := resolveHookIcon(&h, spinFrame)
		label := truncate(h.Label, innerW-3)
		var s lipgloss.Style
		if h.Active {
			s = styles.Pulse
		} else if isError {
			s = lipgloss.NewStyle().Foreground(lipgloss.Color(hookColorError))
		} else {
			s = styles.Bullet
		}
		lines = append(lines, s.Render(icon+" "+label))
	}
	return strings.Join(lines, "\n")
}
