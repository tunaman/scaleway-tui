package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Secrets browser view
// ─────────────────────────────────────────────

func (m rootModel) drawSecretsBrowser() string {
	s := m.secBrowserSecret
	visible := m.filteredSecretVersions()

	// ── Top bar ──
	crumb := lipgloss.NewStyle().Foreground(colComment).Render("SECRETS ")
	sPart := lipgloss.NewStyle().
		Foreground(colPurple).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colPurple).
		Padding(0, 1).
		Render(s.name)
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, crumb, sPart)
	countStr := lipgloss.NewStyle().Foreground(colComment).Render(
		fmt.Sprintf("%d versions", len(m.secBrowserVersions)),
	)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(countStr)-8))
	topBar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).Padding(0, 1).
		Render(leftPart + spacer + countStr)

	// ── Status bar ──
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("↑↓", "Navigate"),
		hotkey("Enter", "View"),
		hotkey("N", "New version"),
		hotkey("U", "Update desc"),
		hotkey("/", "Filter"),
		hotkey("F5", "Refresh"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	barW := m.width - 4
	spacerBar := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	statusBar := lipgloss.NewStyle().Background(colBg2).Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacerBar))

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	// ── Column layout ──
	const secDetailPaneW = 42
	contentW := m.width - 8
	contentH := m.height - topBarHeight - statusBarHeight - 6
	listW := contentW - secDetailPaneW - 1
	const scrollW = 1
	const revW = 5
	const statusW = 14
	const latestW = 8
	const prefixW = 2
	rowW := listW - 2
	descW := rowW - prefixW - revW - statusW - latestW - scrollW
	if descW < 8 {
		descW = 8
	}

	listH := max(1, contentH-listRowOverhead)
	scrollY := m.secBrowserScrollY
	if m.secBrowserCursor >= scrollY+listH {
		scrollY = m.secBrowserCursor - listH + 1
	}
	if m.secBrowserCursor < scrollY {
		scrollY = m.secBrowserCursor
	}

	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	// ── Version list header / filter bar ──
	var listHeader string
	switch {
	case m.secBrowserFiltering:
		listHeader = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.secBrowserFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.secBrowserFilter != "":
		listHeader = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.secBrowserFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		listHeader = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("REV", revW)+padRight("STATUS", statusW)+padRight("LATEST", latestW)+padRight("DESCRIPTION", descW),
		)
	}

	// ── Version rows ──
	var rows []string
	if len(visible) == 0 {
		noMsg := "  No versions found."
		if m.secBrowserFilter != "" {
			noMsg = "  No versions match \"" + m.secBrowserFilter + "\"."
		}
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(rowW-scrollW).Render(noMsg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", rowW-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		v := visible[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		statusColor := colGreen
		switch v.status {
		case "disabled", "scheduled_for_deletion":
			statusColor = colYellow
		case "deleted", "unknown_status":
			statusColor = colRed
		}

		revStr := fmt.Sprintf("v%d", v.revision)
		latestStr := ""
		if v.latest {
			latestStr = lipgloss.NewStyle().Foreground(colBlue).Render("latest")
		}
		statusStr := lipgloss.NewStyle().Foreground(statusColor).Render(padRight(v.status, statusW))
		descStr := lipgloss.NewStyle().Foreground(colComment).Render(padRight(v.description, descW))

		rowStr := padRight(revStr, revW) + statusStr + padRight(latestStr, latestW) + descStr + sb

		if i == m.secBrowserCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+rowStr))
		}
	}

	panelTitle := s.name
	if m.secBrowserFilter != "" {
		panelTitle = fmt.Sprintf("%s  %d/%d", s.name, len(visible), len(m.secBrowserVersions))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		listHeader,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	listPane := panelBox(panelTitle, listW, contentH, colPurple, listContent)
	detailPane := m.renderSecretVersionDetailPane(secDetailPaneW, contentH)

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.secShowContent {
		return m.renderSecretContentOverlay()
	}
	if m.input.active {
		return m.renderInputOverlay(base)
	}
	return base
}

// renderSecretVersionDetailPane renders the right-hand detail pane.
func (m rootModel) renderSecretVersionDetailPane(paneW, paneH int) string {
	visible := m.filteredSecretVersions()
	if len(visible) == 0 || m.secBrowserCursor >= len(visible) {
		return panelBox("VERSION DETAIL", paneW, paneH, colBorder,
			lipgloss.NewStyle().Faint(true).Render("Select a version"))
	}

	v := visible[m.secBrowserCursor]
	innerW := paneW - 4

	row := func(label, val string, valColor lipgloss.Color) string {
		k := lipgloss.NewStyle().Foreground(colComment).Render(padRight(label, 12))
		vv := lipgloss.NewStyle().Foreground(valColor).Render(val)
		return k + vv
	}

	statusColor := colGreen
	switch v.status {
	case "disabled", "scheduled_for_deletion":
		statusColor = colYellow
	case "deleted", "unknown_status":
		statusColor = colRed
	}

	latestStr := "no"
	if v.latest {
		latestStr = lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render("yes")
	}
	createdStr := ""
	if !v.createdAt.IsZero() {
		createdStr = v.createdAt.Format("2006-01-02 15:04")
	}
	updatedStr := ""
	if !v.updatedAt.IsZero() {
		updatedStr = v.updatedAt.Format("2006-01-02 15:04")
	}

	desc := v.description
	if desc == "" {
		desc = lipgloss.NewStyle().Faint(true).Render("(none)")
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(colPurple).Bold(true).Render(
			fmt.Sprintf(" v%d ", v.revision),
		),
		"",
		row("Status:", v.status, statusColor),
		row("Latest:", latestStr, colFg),
		"",
		lipgloss.NewStyle().Foreground(colComment).Render("Description:"),
		lipgloss.NewStyle().Foreground(colFg).Width(innerW).Render("  " + desc),
		"",
		row("Created:", createdStr, colComment),
		row("Updated:", updatedStr, colComment),
		"",
		lipgloss.NewStyle().Foreground(colBg3).Render(strings.Repeat("─", innerW)),
		"",
		lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("Enter  View content"),
		lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("N      New version"),
		lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("U      Update desc"),
	}

	return panelBox("VERSION DETAIL", paneW, paneH, colBorder,
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// renderSecretContentOverlay shows the secret content as a full-screen overlay.
func (m rootModel) renderSecretContentOverlay() string {
	s := m.secBrowserSecret

	dialogW := min(m.width-8, 100)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	heading := bg.Foreground(colPurple).Bold(true).Width(innerW).Render("Secret Content")
	nameLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Secret:  ") +
			bg.Foreground(colFg).Render(s.name),
	)
	revLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Version: ") +
			bg.Foreground(colPurple).Render(fmt.Sprintf("v%d", m.secContentRevision)),
	)
	empty := bg.Width(innerW).Render("")

	content := m.secContent
	const maxContentLen = 2000
	truncated := false
	if len([]rune(content)) > maxContentLen {
		runes := []rune(content)
		content = string(runes[:maxContentLen])
		truncated = true
	}

	// Wrap content lines to fit innerW-2 (2 for padding).
	contentW := innerW - 2
	var contentLines []string
	for _, line := range strings.Split(content, "\n") {
		runes := []rune(line)
		if len(runes) == 0 {
			contentLines = append(contentLines, "")
			continue
		}
		for len(runes) > 0 {
			end := contentW
			if end > len(runes) {
				end = len(runes)
			}
			contentLines = append(contentLines, string(runes[:end]))
			runes = runes[end:]
		}
	}
	if truncated {
		contentLines = append(contentLines, lipgloss.NewStyle().Faint(true).Render("… (truncated)"))
	}

	// Cap displayed lines to avoid overflowing the terminal.
	maxLines := m.height - 16
	if maxLines < 1 {
		maxLines = 1
	}
	if len(contentLines) > maxLines {
		contentLines = contentLines[:maxLines]
		contentLines = append(contentLines, lipgloss.NewStyle().Faint(true).Render("… (more lines hidden)"))
	}

	codeBlock := lipgloss.NewStyle().
		Background(colBg3).Foreground(colGreen).
		Padding(0, 1).Width(innerW).
		Render(strings.Join(contentLines, "\n"))

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))
	closeBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(innerW).Align(lipgloss.Center).
		Render("Any key  Close")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, empty, nameLine, revLine, empty, codeBlock, empty, divider, empty, closeBtn,
		),
	)
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colPurple).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}
