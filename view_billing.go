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

// drawBilling renders the full-screen billing view.
func (m rootModel) drawBilling() string {
	topBar := m.renderBillingTopBar()
	statusBar := m.renderBillingStatusBar()

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading billing data…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	contentH := m.height - topBarHeight - statusBarHeight - 6
	contentW := m.width - 8

	// Split: chart on left (60%), detail table on right (40%)
	chartW := (contentW * 6) / 10
	tableW := contentW - chartW - 1

	chart := m.renderBillingChart(chartW, contentH)
	table := m.renderBillingDetail(tableW, contentH)

	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			topBar,
			lipgloss.JoinHorizontal(lipgloss.Top, chart, table),
			statusBar,
		),
	)
}

// renderBillingTopBar shows the current period and total.
func (m rootModel) renderBillingTopBar() string {
	left := lipgloss.NewStyle().Foreground(colComment).Render("BILLING ") +
		lipgloss.NewStyle().Foreground(colPurple).Border(lipgloss.RoundedBorder()).
			BorderForeground(colPurple).Padding(0, 1).Render(" "+m.billingPeriod+" ")

	// Find current period total
	total := 0.0
	for _, bm := range m.billingMonths {
		if bm.period == m.billingPeriod {
			total = bm.totalExTax
			break
		}
	}
	totalStr := lipgloss.NewStyle().Foreground(colComment).Render("  Total excl. tax: ") +
		lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(fmt.Sprintf(" €%.2f ", total))

	exportMsg := ""
	if m.billingExportMsg != "" {
		exportMsg = "  " + lipgloss.NewStyle().Foreground(colGreen).Render(" ✓ "+m.billingExportMsg+" ")
	}

	clock := lipgloss.NewStyle().Foreground(colComment).Render(" " + time.Now().Format("15:04") + " ")
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, left, totalStr, exportMsg)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(clock)-8))

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(leftPart + spacer + clock)
}

func (m rootModel) renderBillingStatusBar() string {
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("←→", "Month"),
		hotkey("↑↓", "Navigate"),
		hotkey("E", "Export CSV"),
		hotkey("F5", "Refresh"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer))
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
	barAreaW := innerW - 8 // leave 8 cols for Y-axis labels
	barW := max(1, barAreaW/len(m.billingMonths))

	// Build chart lines top→bottom
	lines := make([]string, chartH)
	for row := 0; row < chartH; row++ {
		threshold := maxVal * float64(chartH-row) / float64(chartH)
		line := lipgloss.NewStyle().Foreground(colComment).Render(
			fmt.Sprintf("%6s ", formatEuroShort(threshold)),
		)
		for _, bm := range m.billingMonths {
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
		}
		lines[row] = line
	}

	// X-axis labels
	xAxis := strings.Repeat(" ", 7)
	for _, bm := range m.billingMonths {
		label := bm.period
		t, err := time.Parse("2006-01", bm.period)
		if err == nil {
			label = t.Format("Jan")
		}
		xAxis += padRight(label, barW)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append(lines, strings.Repeat("─", innerW), xAxis)...,
	)
	return panelBox("6-MONTH OVERVIEW", w, h, colPurple, content)
}

// renderBillingDetail shows the consumption table for the current period.
func (m rootModel) renderBillingDetail(w, h int) string {
	catW := 14
	prodW := max(1, w-catW-12-2) // remainder for product name
	valW := 10

	listH := max(1, h-listRowOverhead)

	// Scroll viewport
	scrollY := m.billingScrollY
	if m.billingCursor >= scrollY+listH {
		scrollY = m.billingCursor - listH + 1
	}
	if m.billingCursor < scrollY {
		scrollY = m.billingCursor
	}
	scrollY = max(0, scrollY)

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("CATEGORY", catW) +
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
		rowStr := padRight(r.category, catW) + padRight(r.product, prodW) + padRight(cost, valW)
		if i == m.billingCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(w - 2).Render("▌ " + rowStr)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(w - 2).Render("  " + rowStr)
		}
		rows = append(rows, rowStr)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", w-2),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(m.billingPeriod, w, h, colGreen, content)
}
