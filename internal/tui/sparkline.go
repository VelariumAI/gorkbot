// Package tui — sparkline.go
//
// Braille-based Sparkline Widget: a compact time-series visualisation using
// Unicode Braille characters to achieve sub-character vertical resolution.
//
// Inspired by termui's Sparkline/Plot widgets and its drawille/canvas system,
// which use the 2×4 braille dot grid (characters U+2800–U+28FF) to draw smooth
// curves with 4× vertical resolution per character row and 2× horizontal.
//
// This implementation is native Lip Gloss — no termui/termbox dependency.
//
// Usage:
//
//	sl := NewSparkline(20, 3) // 20 chars wide, 3 rows tall
//	sl.Add(42.0)              // push data points
//	sl.Add(55.0)
//	rendered := sl.Render(color)
package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────────────────────────────────────
// Braille dot encoding
//
// Each braille character encodes an 2×4 dot grid:
//
//	col 0  col 1
//	dot1   dot4   row 0 (top)
//	dot2   dot5   row 1
//	dot3   dot6   row 2
//	dot7   dot8   row 3 (bottom)
//
// Unicode: U+2800 + bitmask, where bit positions are:
//
//	bit 0 = dot1 (col0, row0)   bit 3 = dot4 (col1, row0)
//	bit 1 = dot2 (col0, row1)   bit 4 = dot5 (col1, row1)
//	bit 2 = dot3 (col0, row2)   bit 5 = dot6 (col1, row2)
//	bit 6 = dot7 (col0, row3)   bit 7 = dot8 (col1, row3)
// ─────────────────────────────────────────────────────────────────────────────

// brailleDotBit maps (col 0-1, row 0-3) to the bit position in the braille mask.
var brailleDotBit = [2][4]int{
	{0, 1, 2, 6}, // col 0
	{3, 4, 5, 7}, // col 1
}

// brailleChar returns the Unicode braille character for the given bitmask.
func brailleChar(mask int) rune {
	return rune(0x2800 + mask)
}

// ─────────────────────────────────────────────────────────────────────────────
// Sparkline
// ─────────────────────────────────────────────────────────────────────────────

// Sparkline renders a compact braille time-series chart.
type Sparkline struct {
	// Width in terminal columns.
	Width int
	// Height in terminal rows (each row = 4 braille dot rows).
	Height int
	// Data points (most recent last).
	data []float64
	// MaxPoints limits the history retained (default: Width * 2).
	MaxPoints int
}

// NewSparkline creates a new Sparkline of the given dimensions.
func NewSparkline(width, height int) *Sparkline {
	if width < 2 {
		width = 2
	}
	if height < 1 {
		height = 1
	}
	return &Sparkline{
		Width:     width,
		Height:    height,
		MaxPoints: width * 2 * 2, // 2 columns per char, keep 2× the display capacity
	}
}

// Add appends a data point. Older points are dropped if MaxPoints is exceeded.
func (s *Sparkline) Add(value float64) {
	s.data = append(s.data, value)
	if len(s.data) > s.MaxPoints {
		s.data = s.data[len(s.data)-s.MaxPoints:]
	}
}

// SetData replaces the internal dataset entirely.
func (s *Sparkline) SetData(data []float64) {
	s.data = make([]float64, len(data))
	copy(s.data, data)
}

// MaxVal returns the maximum value in the data set (or 0 if empty).
func (s *Sparkline) MaxVal() float64 {
	if len(s.data) == 0 {
		return 0
	}
	max := s.data[0]
	for _, v := range s.data {
		if v > max {
			max = v
		}
	}
	return max
}

// Last returns the most recent data point (or 0).
func (s *Sparkline) Last() float64 {
	if len(s.data) == 0 {
		return 0
	}
	return s.data[len(s.data)-1]
}

// Render draws the sparkline and returns a Lip-Gloss-coloured string.
// color is a Lip Gloss / ANSI colour string (e.g. "#00D9FF" or "cyan").
func (s *Sparkline) Render(color string) string {
	if len(s.data) == 0 {
		return strings.Repeat("·", s.Width)
	}

	// Each terminal column encodes 2 data columns; each terminal row = 4 dot rows.
	dotCols := s.Width * 2
	dotRows := s.Height * 4

	// Select the most-recent dotCols data points.
	data := s.data
	if len(data) > dotCols {
		data = data[len(data)-dotCols:]
	}

	// Compute min/max for normalisation.
	minVal, maxVal := data[0], data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	rng := maxVal - minVal
	if rng == 0 {
		rng = 1
	}

	// Build a 2D dot grid: grid[dotRow][dotCol] = true if filled.
	grid := make([][]bool, dotRows)
	for r := range grid {
		grid[r] = make([]bool, dotCols)
	}

	// Fill dots from bottom up: a value of maxVal fills all rows,
	// a value of minVal fills none.
	for col, v := range data {
		// Normalise to 0.0–1.0.
		norm := (v - minVal) / rng
		// How many dot rows to fill (from bottom).
		filledDots := int(math.Round(norm * float64(dotRows)))
		if filledDots < 0 {
			filledDots = 0
		}
		if filledDots > dotRows {
			filledDots = dotRows
		}
		for i := 0; i < filledDots; i++ {
			row := dotRows - 1 - i // fill from bottom
			if row >= 0 {
				grid[row][col] = true
			}
		}
	}

	// Convert grid to braille characters.
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	var rows []string
	for termRow := 0; termRow < s.Height; termRow++ {
		var rowChars []rune
		for termCol := 0; termCol < s.Width; termCol++ {
			mask := 0
			for dotCol := 0; dotCol < 2; dotCol++ {
				dc := termCol*2 + dotCol
				if dc >= dotCols {
					break
				}
				for dotRow := 0; dotRow < 4; dotRow++ {
					dr := termRow*4 + dotRow
					if dr >= dotRows {
						break
					}
					if grid[dr][dc] {
						mask |= 1 << brailleDotBit[dotCol][dotRow]
					}
				}
			}
			rowChars = append(rowChars, brailleChar(mask))
		}
		rows = append(rows, st.Render(string(rowChars)))
	}

	return strings.Join(rows, "\n")
}

// RenderSparklineWithAxes renders a sparkline with Y-axis labels (max / mid / 0).
// color is a Lip Gloss color string.
func RenderSparklineWithAxes(sl *Sparkline, color string) string {
	braille := sl.Render(color)
	lines := strings.Split(braille, "\n")
	if len(lines) == 0 {
		return braille
	}

	maxVal := sl.MaxVal()
	midVal := maxVal / 2

	axisStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	var out []string
	for i, line := range lines {
		var label string
		switch i {
		case 0:
			label = fmt.Sprintf("%.0f", maxVal)
		case len(lines) / 2:
			label = fmt.Sprintf("%.0f", midVal)
		case len(lines) - 1:
			label = "0"
		}
		// Right-align label in a 5-char field
		padded := fmt.Sprintf("%5s", label)
		out = append(out, axisStyle.Render(padded)+" "+line)
	}
	return strings.Join(out, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// ASCII Gauge
// ─────────────────────────────────────────────────────────────────────────────

// RenderGauge draws a horizontal progress bar using Lip Gloss.
//
//	[████████░░░░░░] 63%
//
// pct should be 0.0–1.0.
func RenderGauge(pct float64, width int, filledColor, emptyColor string) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	inner := width - 2 // subtract bracket chars
	if inner < 1 {
		inner = 1
	}
	filled := int(math.Round(pct * float64(inner)))
	empty := inner - filled

	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(filledColor))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(emptyColor))

	bar := "[" +
		filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty)) +
		"]"

	pctStr := fmt.Sprintf(" %.0f%%", pct*100)
	return bar + pctStr
}

// ─────────────────────────────────────────────────────────────────────────────
// ASCII Bar Chart
// ─────────────────────────────────────────────────────────────────────────────

// BarChartEntry is one bar in the chart.
type BarChartEntry struct {
	Label string
	Value float64
	Color string // Lip Gloss color string
}

// RenderBarChart draws a horizontal bar chart.
//
//	bash          ████████████████████  42
//	read_file     ██████████           22
//	git_status    ████                 8
//
// width is the total display width (labels + bars + values).
func RenderBarChart(entries []BarChartEntry, width int) string {
	if len(entries) == 0 {
		return "(no data)"
	}

	// Find max label width.
	maxLabel := 0
	for _, e := range entries {
		if len(e.Label) > maxLabel {
			maxLabel = len(e.Label)
		}
	}
	if maxLabel > 20 {
		maxLabel = 20
	}

	// Find max value for normalisation.
	maxVal := 0.0
	for _, e := range entries {
		if e.Value > maxVal {
			maxVal = e.Value
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// Bar width = total width - label - space - value column.
	valueCol := 6
	barWidth := width - maxLabel - 2 - valueCol
	if barWidth < 4 {
		barWidth = 4
	}

	var lines []string
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	for _, e := range entries {
		// Truncate label.
		label := e.Label
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		// Compute bar length.
		barLen := int(math.Round((e.Value / maxVal) * float64(barWidth)))
		if barLen < 0 {
			barLen = 0
		}
		if barLen > barWidth {
			barLen = barWidth
		}

		barColor := e.Color
		if barColor == "" {
			barColor = GrokBlue
		}
		barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor))

		line := fmt.Sprintf("%-*s  %s%s",
			maxLabel,
			labelStyle.Render(label),
			barStyle.Render(strings.Repeat("█", barLen)),
			valueStyle.Render(fmt.Sprintf("  %.0f", e.Value)),
		)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
