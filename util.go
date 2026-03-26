package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Render helpers
// ─────────────────────────────────────────────

// panelBox renders a btop-style bordered box with a title embedded in the top border.
func panelBox(title string, w, h int, borderColor lipgloss.Color, content string, rightGutter ...string) string {
	bc := lipgloss.NormalBorder()

	titleStr := " " + title + " "
	titleRendered := lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render(titleStr)

	dashCount := max(0, w-lipgloss.Width(titleStr)-3)
	borderSt := lipgloss.NewStyle().Foreground(borderColor)
	topLine := borderSt.Render(bc.TopLeft+bc.Top) +
		titleRendered +
		borderSt.Render(strings.Repeat(bc.Top, dashCount)+bc.TopRight)

	innerH := max(1, h-2)
	innerW := max(1, w-2)
	contentW := innerW

	contentLines := strings.Split(content, "\n")
	for len(contentLines) < innerH {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > innerH {
		contentLines = contentLines[:innerH]
	}

	side := borderSt.Render(bc.Left)
	defaultSideR := borderSt.Render(bc.Right)
	bottomLine := borderSt.Render(bc.BottomLeft + strings.Repeat(bc.Bottom, innerW) + bc.BottomRight)

	var sb strings.Builder
	sb.WriteString(topLine + "\n")
	for i, line := range contentLines {
		vis := lipgloss.Width(line)
		pad := ""
		if vis < contentW {
			pad = strings.Repeat(" ", contentW-vis)
		}
		sideR := defaultSideR
		if i < len(rightGutter) {
			sideR = rightGutter[i]
		}
		sb.WriteString(side + line + pad + sideR + "\n")
	}
	sb.WriteString(bottomLine)
	return sb.String()
}

// renderVScrollBar returns a slice of single-character strings representing a
// minimal vertical scrollbar, one string per visible row.
func renderVScrollBar(total, offset, visible int) []string {
	out := make([]string, visible)
	for i := range out {
		out[i] = lipgloss.NewStyle().Foreground(colBg3).Render("│")
	}
	if total <= visible {
		return out
	}
	thumbH := max(1, visible*visible/total)
	thumbTop := (offset * (visible - thumbH)) / max(1, total-visible)
	for i := thumbTop; i < thumbTop+thumbH && i < visible; i++ {
		out[i] = lipgloss.NewStyle().Foreground(colComment).Render("█")
	}
	return out
}

// highlightMatch wraps the first case-insensitive occurrence of needle in s
// with a yellow colour for display in filter lists.
func highlightMatch(s, needle string) string {
	lower := strings.ToLower(s)
	lowerN := strings.ToLower(needle)
	idx := strings.Index(lower, lowerN)
	if idx < 0 {
		return s
	}
	before := s[:idx]
	match := s[idx : idx+len(needle)]
	after := s[idx+len(needle):]
	return before +
		lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(match) +
		after
}

// padRight pads s to exactly n visible characters.
// If s is wider than n it is truncated and a trailing … is appended.
func padRight(s string, n int) string {
	if n <= 0 {
		return ""
	}
	vis := lipgloss.Width(s)
	if vis <= n {
		return s + strings.Repeat(" ", n-vis)
	}
	runes := []rune(s)
	cut := n - 1
	if cut < 0 {
		cut = 0
	}
	return string(runes[:cut]) + "…"
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// formatEuroShort formats a float as a short euro value (e.g. "€12K", "€1.2K", "€345").
func formatEuroShort(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("€%.0fK", v/1000)
	case v >= 1000:
		return fmt.Sprintf("€%.1fK", v/1000)
	default:
		return fmt.Sprintf("€%.0f", v)
	}
}

// parentPrefix returns the prefix one level up.
// e.g. "a/b/c/" → "a/b/"  "a/" → ""
func parentPrefix(prefix string) string {
	trimmed := strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return ""
	}
	return trimmed[:idx+1]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min is a helper for Go versions before 1.21.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hasRebootingNode returns true if any node in the slice has status "rebooting".
func hasRebootingNode(nodes []k8sNode) bool {
	for _, n := range nodes {
		if strings.ToLower(n.status) == "rebooting" {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────
// Billing helpers
// ─────────────────────────────────────────────

// moneyToFloat converts a *scw.Money to a float64 EUR value.
func moneyToFloat(m *scw.Money) float64 {
	if m == nil {
		return 0
	}
	return float64(m.Units) + float64(m.Nanos)/1e9
}

// prevMonth returns the YYYY-MM string one month before the given one.
func prevMonth(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// nextMonth returns the YYYY-MM string one month after the given one.
func nextMonth(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.AddDate(0, 1, 0).Format("2006-01")
}
