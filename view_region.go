package main

import (
	"charm.land/lipgloss/v2"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Region picker overlay
// ─────────────────────────────────────────────

// renderRegionPicker shows a centered region picker. The active region is
// marked with a ✓ and the cursor row is highlighted; Enter switches to the
// highlighted region, Esc cancels.
func (m rootModel) renderRegionPicker() string {
	regions := make([]string, 0, len(scw.AllRegions))
	for _, r := range scw.AllRegions {
		regions = append(regions, string(r))
	}

	hint := "  ↑↓ navigate · Enter switch · Esc cancel"

	// Overlay wide enough for the hint line and the longest region name.
	overlayW := lipgloss.Width(hint) + 8 // border(2) + padding(4) + breathing room(2)
	for _, label := range regions {
		if n := lipgloss.Width(label) + 12; n > overlayW {
			overlayW = n
		}
	}

	rowW := overlayW - 6 // inside border(2) + padding(4)
	var rows []string
	for i, label := range regions {
		active := label == m.activeRegion
		cursor := i == m.regionPickerCursor

		checkmark := "  "
		if active {
			checkmark = lipgloss.NewStyle().Foreground(colGreen).Render("✓ ")
		}

		rowContent := checkmark + label
		if cursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+rowContent))
		} else {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg2).Foreground(colFg).Width(rowW).Render("  "+rowContent))
		}
	}

	hintRendered := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Faint(true).Width(rowW).Render(hint)

	body := lipgloss.JoinVertical(lipgloss.Left,
		append(rows, "", hintRendered)...,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBlue).
		Background(colBg2).
		Padding(1, 2).
		Width(overlayW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(colBg)),
	)
}
