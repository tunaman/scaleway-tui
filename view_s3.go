package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Object browser view
// ─────────────────────────────────────────────

// drawObjectBrowser renders the full-screen object browser.
func (m rootModel) drawObjectBrowser() string {
	topBar := m.renderBrowserTopBar()
	statusBar := m.renderBrowserStatusBar()

	if m.upload.active {
		// Show progress overlay — render the content behind it first.
		contentH := m.height - topBarHeight - statusBarHeight - 6
		content := m.renderBrowserContent(m.width-8, contentH)
		base := lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
		)
		return m.renderUploadProgress(base)
	}

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

	contentH := m.height - topBarHeight - statusBarHeight - 6
	content := m.renderBrowserContent(m.width-8, contentH)

	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.showConfirm {
		return m.renderConfirmDialog(base)
	}
	if m.input.active {
		return m.renderInputOverlay(base)
	}
	return base
}

func (m rootModel) renderBrowserTopBar() string {
	crumb := lipgloss.NewStyle().Foreground(colComment).Render("BUCKET ")
	bucketPart := lipgloss.NewStyle().
		Foreground(colGreen).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Render(m.browserBucket)

	path := ""
	if m.browserPrefix != "" {
		parts := strings.Split(strings.TrimSuffix(m.browserPrefix, "/"), "/")
		for _, p := range parts {
			path += lipgloss.NewStyle().Foreground(colComment).Render(" › ") +
				lipgloss.NewStyle().Foreground(colBlue).Render(p)
		}
	}

	// Right side: selected count (if any) + total items
	countStr := fmt.Sprintf("%d items", len(m.browserEntries))
	if len(m.browserSelected) > 0 {
		countStr = lipgloss.NewStyle().Foreground(colGreen).Render(
			fmt.Sprintf("%d selected", len(m.browserSelected)),
		) + lipgloss.NewStyle().Foreground(colComment).Render(
			fmt.Sprintf(" / %d items", len(m.browserEntries)),
		)
	} else {
		countStr = lipgloss.NewStyle().Foreground(colComment).Render(countStr)
	}

	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, crumb, bucketPart, path)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(countStr)-8))

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(leftPart + spacer + countStr)
}

func (m rootModel) renderBrowserStatusBar() string {
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("↑↓", "Navigate"),
		hotkey("Enter", "Open"),
		hotkey("Space", "Select"),
		hotkey("A", "All"),
		hotkey("C", "New folder"),
		hotkey("U", "Upload"),
		hotkey("D", "Delete"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	count := fmt.Sprintf("%d items", len(m.browserEntries))
	status := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(count)
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys)-lipgloss.Width(status))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer, status))
}

func (m rootModel) renderBrowserContent(totalW, height int) string {
	// ── Column widths ──
	// Row layout: prefix(2) + chk(4) + icon(2) + name(nameW) + mod(11) + size(11) + class(11) + scroll(1)
	// All of that must equal innerW = totalW-2 (panelBox adds left+right border).
	// So: nameW = (totalW-2) - 2 - 4 - 2 - 11 - 11 - 11 - 1 = totalW - 44
	chkW := 4
	iconW := 2
	modW := 11
	sizeW := 11
	classW := 11
	scrollW := 1
	nameW := totalW - 2 - 2 - chkW - iconW - modW - sizeW - classW - scrollW

	listH := max(1, height-listRowOverhead)

	scrollY := m.browserScrollY
	if m.browserCursor >= scrollY+listH {
		scrollY = m.browserCursor - listH + 1
	}
	if m.browserCursor < scrollY {
		scrollY = m.browserCursor
	}
	scrollY = max(0, scrollY)

	selectedCount := len(m.browserSelected)
	totalCount := len(m.browserEntries)

	// ── Scrollbar column (computed once, appended per row) ──
	vScrollBar := renderVScrollBar(totalCount, scrollY, listH)

	// ── Select-all checkbox state ──
	var selectAllBox string
	switch {
	case totalCount == 0 || selectedCount == 0:
		selectAllBox = lipgloss.NewStyle().Foreground(colComment).Render("[ ]")
	case selectedCount == totalCount:
		selectAllBox = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x]")
	default:
		selectAllBox = lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("[~]")
	}

	// ── Header ──
	hint := ""
	if m.browserScrollX > 0 {
		hint = fmt.Sprintf(" ◀+%d", m.browserScrollX)
	}
	hintRendered := lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hint)
	hintW := lipgloss.Width(hint)
	headerNameW := max(1, nameW-hintW)

	headerCols := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("NAME", headerNameW),
	) + hintRendered +
		lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("MODIFIED", modW)+padRight("SIZE", sizeW)+padRight("CLASS", classW),
		)
	headerPrefix := "  " + selectAllBox + " " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("", iconW))
	// Header scrollbar cell — always plain track char
	headerSB := lipgloss.NewStyle().Foreground(colBg3).Render("│")
	header := headerPrefix + headerCols + headerSB

	// ── Rows ──
	var rows []string
	if totalCount == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("  Empty folder."))
	}

	end := min(scrollY+listH, totalCount)
	for i := scrollY; i < end; i++ {
		e := m.browserEntries[i]
		isSelected := m.browserSelected[e.fullKey]

		chk := lipgloss.NewStyle().Foreground(colComment).Render("[ ] ")
		if isSelected {
			chk = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x] ")
		}

		var icon string
		if e.isDir {
			icon = lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("▸ ")
		} else {
			icon = lipgloss.NewStyle().Foreground(colComment).Render("· ")
		}

		name := e.name
		runes := []rune(name)
		if m.browserScrollX > 0 {
			if m.browserScrollX >= len(runes) {
				name = ""
			} else {
				name = string(runes[m.browserScrollX:])
			}
		}

		modStr, sizeStr, classStr := "", "", ""
		if !e.isDir {
			modStr = e.lastModified.Format("2006-01-02")
			sizeStr = formatBytes(e.size)
			classStr = e.storageClass
		}

		// Scrollbar char for this row
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		inner := chk + icon +
			padRight(name, nameW) +
			padRight(modStr, modW) +
			padRight(sizeStr, sizeW) +
			padRight(classStr, classW) +
			sb

		var rowStr string
		if i == m.browserCursor {
			prefix := lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("▌ ")
			rowStr = prefix + lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW-4). // innerW(totalW-2) - prefix(2)
				Render(inner)
		} else {
			rowStr = "  " + lipgloss.NewStyle().Foreground(colFg).Render(inner)
		}
		rows = append(rows, rowStr)
	}

	// Divider also needs a scrollbar-column placeholder to stay aligned
	dividerSB := lipgloss.NewStyle().Foreground(colBg3).Render("│")
	divider := strings.Repeat("─", totalW-2-scrollW) + dividerSB

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		divider,
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	// No gutter — right border is always the plain panel border.
	return panelBox(m.browserBucket, totalW, height, colBlue, listContent)
}

// renderConfirmDialog overlays a centred confirmation box on top of the base view.
func (m rootModel) renderConfirmDialog(base string) string {
	n := len(m.confirmItems)

	hasDir := false
	for _, e := range m.confirmItems {
		if e.isDir {
			hasDir = true
			break
		}
	}

	const dialogW = 54
	const innerW = dialogW - 6 // border(1*2) + padding(2*2)

	bg := lipgloss.NewStyle().Background(colBg2)

	// ── Header ──
	countStr := "1 item"
	if n > 1 {
		countStr = fmt.Sprintf("%d items", n)
	}
	heading := bg.Foreground(colRed).Bold(true).Width(innerW).
		Render("Delete " + countStr + "?")

	warnText := "This action cannot be undone."
	if hasDir {
		warnText = "Folders will be deleted recursively."
	}
	warn := bg.Foreground(colComment).Width(innerW).Render(warnText)

	// ── Item list (max 5 shown) ──
	const maxShow = 5
	var itemLines []string
	for i, e := range m.confirmItems {
		if i >= maxShow {
			more := bg.Foreground(colComment).Faint(true).Width(innerW).
				Render(fmt.Sprintf("  … and %d more", n-maxShow))
			itemLines = append(itemLines, more)
			break
		}
		var itemIcon string
		if e.isDir {
			itemIcon = lipgloss.NewStyle().Background(colBg2).Foreground(colYellow).Render("▸ ")
		} else {
			itemIcon = lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("  ")
		}
		name := e.name
		maxNameW := innerW - 4
		if lipgloss.Width(name) > maxNameW {
			rr := []rune(name)
			name = string(rr[:maxNameW-1]) + "…"
		}
		line := bg.Width(innerW).Render(
			"  " + itemIcon + lipgloss.NewStyle().Background(colBg2).Foreground(colFg).Render(name),
		)
		itemLines = append(itemLines, line)
	}

	// ── Divider ──
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	// ── Buttons ──
	btnW := (innerW - 1) / 2
	leftW := btnW + ((innerW - 1) % 2)

	yesBtn := lipgloss.NewStyle().
		Background(colRed).Foreground(lipgloss.Color("#ffffff")).
		Bold(true).Width(leftW).Align(lipgloss.Center).
		Render("Y  Yes, delete")

	noBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(btnW).Align(lipgloss.Center).
		Render("N  Cancel")

	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		yesBtn,
		lipgloss.NewStyle().Background(colBg2).Width(1).Render(""),
		noBtn,
	)

	empty := bg.Width(innerW).Render("")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, warn, empty,
			lipgloss.JoinVertical(lipgloss.Left, itemLines...),
			empty, divider, empty,
			buttons,
		),
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colRed).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderUploadProgress draws a centred upload progress overlay.
func (m rootModel) renderUploadProgress(base string) string {
	const dialogW = 56
	const innerW = dialogW - 6 // border(1*2) + padding(2*2) = 6

	bg := lipgloss.NewStyle().Background(colBg2)

	pct := 0.0
	if m.upload.total > 0 {
		pct = float64(m.upload.bytesRead) / float64(m.upload.total)
		if pct > 1 {
			pct = 1
		}
	}

	// ── Title ──
	titleStr := bg.Foreground(colGreen).Bold(true).Width(innerW).Render("UPLOADING")

	// ── Filename ──
	fname := m.upload.filename
	if fname == "" {
		fname = "—"
	}
	if lipgloss.Width(fname) > innerW {
		rr := []rune(fname)
		fname = "…" + string(rr[len(rr)-(innerW-1):])
	}
	fileLabel := bg.Foreground(colFg).Width(innerW).Render(fname)

	// ── Progress bar: barW chars + 7-char label "100.0%" right-aligned ──
	const pctW = 7 // " 100.0%"
	barW := innerW - pctW - 1
	filled := int(float64(barW) * pct)
	if filled > barW {
		filled = barW
	}
	barFilled := lipgloss.NewStyle().Background(colBg2).Foreground(colGreen).Render(strings.Repeat("█", filled))
	barEmpty := lipgloss.NewStyle().Background(colBg2).Foreground(colBg3).Render(strings.Repeat("░", barW-filled))
	pctLabel := bg.Foreground(colComment).Width(pctW + 1).Align(lipgloss.Right).
		Render(fmt.Sprintf("%.1f%%", pct*100))
	barLine := lipgloss.JoinHorizontal(lipgloss.Top, barFilled, barEmpty, pctLabel)

	// ── Stats ──
	transferred := fmt.Sprintf("%s / %s", formatBytes(m.upload.bytesRead), formatBytes(m.upload.total))
	statsStr := bg.Foreground(colComment).Width(innerW).Render(transferred)

	// ── Divider ──
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	hint := bg.Foreground(colComment).Faint(true).Width(innerW).Render("Esc not available during upload")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStr, fileLabel, "",
			barLine, statsStr,
			"", divider, hint,
		),
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

