package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Billing — views
// ─────────────────────────────────────────────

// renderBillingContent renders the billing chart and detail table inside the
// dashboard content panel (nav remains visible).
func (m rootModel) renderBillingContent(totalW, height int, borderColor lipgloss.Color) string {
	if m.loading {
		inner := lipgloss.Place(
			totalW-4, height-listRowOverhead,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading billing data…",
		)
		return panelBox("BILLING", totalW, height, borderColor, inner)
	}

	// Split: chart on left (60%), detail table on right (40%).
	chartW := (totalW * 6) / 10
	tableW := totalW - chartW

	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderBillingChart(chartW, height),
		m.renderBillingDetail(tableW, height),
	)
}

// renderBillingProjectOverlay shows a centered project picker.
func (m rootModel) renderBillingProjectOverlay() string {
	// Build entry list: "All projects" first, then each project.
	type entry struct {
		label string
		id    string
	}
	entries := make([]entry, 0, len(m.projects)+1)
	entries = append(entries, entry{label: "All projects", id: ""})
	for _, p := range m.projects {
		entries = append(entries, entry{label: p.name, id: p.id})
	}

	hint := "  ↑↓ navigate · Enter select · Esc cancel"

	// Overlay wide enough for the hint line and all project names.
	overlayW := lipgloss.Width(hint) + 8 // border(2) + padding(4) + breathing room(2)
	for _, e := range entries {
		if n := lipgloss.Width(e.label) + 12; n > overlayW {
			overlayW = n
		}
	}

	rowW := overlayW - 6 // inside border(2) + padding(4)
	var rows []string
	for i, e := range entries {
		active := i == m.billingProjectIdx
		cursor := i == m.billingProjectCursor

		checkmark := "  "
		if active {
			checkmark = lipgloss.NewStyle().Foreground(colGreen).Render("✓ ")
		}

		rowContent := checkmark + e.label
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
		BorderForeground(colPurple).
		Background(colBg2).
		Padding(1, 2).
		Width(overlayW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderBillingExportOverlay shows a date-range picker before exporting CSV.
func (m rootModel) renderBillingExportOverlay() string {
	fmtPeriod := func(p string) string {
		t, err := time.Parse("2006-01", p)
		if err != nil {
			return p
		}
		return t.Format("Jan 2006")
	}

	hintText := "Tab switch · ←→ adjust · Enter export · Esc cancel"
	// overlayW: border(2) + padding(4) + content
	overlayW := lipgloss.Width(hintText) + 6
	rowW := overlayW - 4 // inside border(2) + padding(2 each side)

	renderField := func(label, value string, focused bool) string {
		var box string
		if focused {
			box = lipgloss.NewStyle().Foreground(colGreen).Border(lipgloss.RoundedBorder()).
				BorderForeground(colGreen).Padding(0, 1).Bold(true).Render(" " + fmtPeriod(value) + " ")
		} else {
			box = lipgloss.NewStyle().Foreground(colPurple).Border(lipgloss.RoundedBorder()).
				BorderForeground(colPurple).Padding(0, 1).Render(" " + fmtPeriod(value) + " ")
		}
		labelStr := lipgloss.NewStyle().Foreground(colComment).Render(label + ": ")
		row := lipgloss.JoinHorizontal(lipgloss.Center, labelStr, box)
		return lipgloss.NewStyle().Background(colBg2).Width(rowW).Render(row)
	}

	fromField := renderField("From", m.billingExportFrom, m.billingExportField == 0)
	toField := renderField("To  ", m.billingExportTo, m.billingExportField == 1)

	hint := lipgloss.NewStyle().Foreground(colComment).Faint(true).Width(rowW).
		Background(colBg2).Render(hintText)

	blank := lipgloss.NewStyle().Background(colBg2).Width(rowW).Render("")

	body := lipgloss.JoinVertical(lipgloss.Left,
		fromField,
		blank,
		toField,
		blank,
		hint,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colPurple).
		Background(colBg2).
		Padding(1, 2).
		Width(overlayW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderBillingChart draws an ASCII bar chart of the last N months.
func (m rootModel) renderBillingChart(w, h int) string {
	innerW := w - 2 // inside panel borders

	if len(m.billingMonths) == 0 {
		return panelBox("6-MONTH OVERVIEW", w, h, colPurple,
			lipgloss.NewStyle().Faint(true).Render("No data"))
	}

	// Find max for scaling
	maxVal := 0.0
	for _, bm := range m.billingMonths {
		if bm.totalExTax > maxVal {
			maxVal = bm.totalExTax
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// Chart area height: innerH - header(1) - divider(1) - x-axis(1) - labels(1)
	chartH := max(4, h-listRowOverhead-2)
	n := len(m.billingMonths)
	barAreaW := innerW - 8 // leave 8 cols for Y-axis labels
	// Reserve 1-col gap between each bar; at least 1 col wide per bar.
	barW := max(1, (barAreaW-(n-1))/n)

	// Build chart lines top→bottom
	lines := make([]string, chartH)
	for row := 0; row < chartH; row++ {
		threshold := maxVal * float64(chartH-row) / float64(chartH)
		line := lipgloss.NewStyle().Foreground(colComment).Render(
			fmt.Sprintf("%6s ", formatEuroShort(threshold)),
		)
		for bi, bm := range m.billingMonths {
			filled := bm.totalExTax >= threshold
			barColor := colPurple
			if bm.period == m.billingPeriod {
				barColor = colGreen
			}
			cell := strings.Repeat(" ", barW)
			if filled {
				cell = lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", barW))
			}
			line += cell
			if bi < n-1 {
				line += " " // gap between bars
			}
		}
		lines[row] = line
	}

	// X-axis labels
	xAxis := strings.Repeat(" ", 7)
	for bi, bm := range m.billingMonths {
		label := bm.period
		t, err := time.Parse("2006-01", bm.period)
		if err == nil {
			label = t.Format("Jan")
		}
		xAxis += padRight(label, barW)
		if bi < n-1 {
			xAxis += " " // gap between labels
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append(lines, strings.Repeat("─", innerW), xAxis)...,
	)
	return panelBox("6-MONTH OVERVIEW", w, h, colPurple, content)
}

// renderBillingDetail shows the consumption table for the current period.
func (m rootModel) renderBillingDetail(w, h int) string {
	const scrollW = 1
	innerW := w - 2
	catW := 22
	valW := 10
	prodW := max(1, innerW-2-catW-valW-scrollW) // prefix(2) + catW + prodW + valW + scrollW = innerW

	// Reserve 2 rows below the scroll area for the divider + TOTAL row.
	listH := max(1, h-listRowOverhead-2)

	// Scroll viewport
	scrollY := m.billingScrollY
	if m.billingCursor >= scrollY+listH {
		scrollY = m.billingCursor - listH + 1
	}
	if m.billingCursor < scrollY {
		scrollY = m.billingCursor
	}
	scrollY = max(0, scrollY)

	vScrollBar := renderVScrollBar(len(m.billingDetail), scrollY, listH)

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		"  " + padRight("CATEGORY", catW) +
			padRight("PRODUCT", prodW) +
			padRight("COST (€)", valW),
	)

	var rows []string
	if len(m.billingDetail) == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("  No data for this period."))
	}

	end := min(scrollY+listH, len(m.billingDetail))
	for i := scrollY; i < end; i++ {
		r := m.billingDetail[i]
		cost := fmt.Sprintf("%.2f", r.valueEUR)
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}
		rowStr := padRight(r.category, catW) + padRight(r.product, prodW) + padRight(cost, valW) + sb
		if i == m.billingCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(innerW).Render("▌ " + rowStr)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(innerW).Render("  " + rowStr)
		}
		rows = append(rows, rowStr)
	}

	// Pinned TOTAL row — sum all detail rows regardless of scroll position.
	totalAmt := 0.0
	for _, r := range m.billingDetail {
		totalAmt += r.valueEUR
	}
	totalCostStr := fmt.Sprintf("€%.2f", totalAmt)
	totalRowStr := padRight("TOTAL", catW+prodW) + padRight(totalCostStr, valW)
	totalRow := lipgloss.NewStyle().Foreground(colGreen).Bold(true).Width(innerW).Render("  " + totalRowStr)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", innerW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		strings.Repeat("─", innerW),
		totalRow,
	)
	return panelBox(m.billingPeriod, w, h, colGreen, content)
}
