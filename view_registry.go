package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Registry browser view
// ─────────────────────────────────────────────

func (m rootModel) drawRegistryBrowser() string {
	ns := m.regBrowserNamespace
	visible := m.filteredRegistryImages()

	// ── Top bar ──
	topBar := m.renderTopBar()

	// ── Status bar ──
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	var keys string
	if m.regBrowserFocus == 1 {
		keys = lipgloss.JoinHorizontal(lipgloss.Top,
			hotkey("↑↓", "Navigate"),
			hotkey("Tab", "Images"),
			hotkey("Enter", "Pull"),
			hotkey("Space", "Select"),
			hotkey("A", "Select all"),
			hotkey("D", "Delete"),
			hotkey("/", "Filter"),
			hotkey("Esc", "Back"),
			hotkey("Q", "Quit"),
		)
	} else {
		keys = lipgloss.JoinHorizontal(lipgloss.Top,
			hotkey("↑↓", "Navigate"),
			hotkey("Tab", "Versions"),
			hotkey("/", "Filter"),
			hotkey("Esc", "Back"),
			hotkey("F5", "Refresh"),
			hotkey("Q", "Quit"),
		)
	}
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
	const regDetailPaneW = 52
	contentW := m.width - 8
	contentH := m.height - topBarHeight - statusBarHeight - 6
	listW := contentW - regDetailPaneW - 1
	const scrollW = 1
	const sizeW = 10
	const modW = 16
	const prefixW = 2
	rowW := listW - 2
	nameW := rowW - prefixW - sizeW - modW - scrollW
	if nameW < 8 {
		nameW = 8
	}

	listH := max(1, contentH-listRowOverhead)
	scrollY := m.regBrowserScrollY
	if m.regBrowserCursor >= scrollY+listH {
		scrollY = m.regBrowserCursor - listH + 1
	}
	if m.regBrowserCursor < scrollY {
		scrollY = m.regBrowserCursor
	}

	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	listBorderColor := colPurple
	if m.regBrowserFocus == 1 {
		listBorderColor = colBorder
	}

	// ── Image list header / filter bar ──
	var listHeader string
	switch {
	case m.regBrowserFiltering:
		listHeader = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.regBrowserFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.regBrowserFilter != "":
		listHeader = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.regBrowserFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		listHeader = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("IMAGE", nameW) + padRight("MODIFIED", modW) + padRight("SIZE", sizeW),
		)
	}

	// ── Image rows ──
	var rows []string
	if len(visible) == 0 {
		noMsg := "  No images in this namespace."
		if m.regBrowserFilter != "" {
			noMsg = "  No images match \"" + m.regBrowserFilter + "\"."
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
		img := visible[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		statusColor := colGreen
		switch img.status {
		case "error", "locked":
			statusColor = colRed
		case "deleting":
			statusColor = colYellow
		}

		sizeStr := formatBytes(int64(img.sizeBytes))
		modStr := ""
		if !img.updatedAt.IsZero() {
			modStr = img.updatedAt.Format("2006-01-02")
		}

		var nameCol string
		if m.regBrowserFilter != "" {
			nameCol = padRight(highlightMatch(img.name, m.regBrowserFilter), nameW)
		} else {
			nameCol = lipgloss.NewStyle().Foreground(statusColor).Render(padRight(img.name, nameW))
		}
		modCol := lipgloss.NewStyle().Foreground(colComment).Render(padRight(modStr, modW))
		rowStr := nameCol + modCol + padRight(sizeStr, sizeW) + sb

		if i == m.regBrowserCursor {
			plainRowStr := padRight(img.name, nameW) + padRight(modStr, modW) + padRight(sizeStr, sizeW) + sb
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+plainRowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+rowStr))
		}
	}

	panelTitle := ns.endpoint
	if m.regBrowserFilter != "" {
		panelTitle = fmt.Sprintf("%s  %d/%d", ns.endpoint, len(visible), len(m.regBrowserImages))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		listHeader,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	listPane := panelBox(panelTitle, listW, contentH, listBorderColor, listContent)
	detailPane := m.renderRegistryVersionPane(regDetailPaneW, contentH)

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.regTagActionOverlay {
		return m.renderRegistryTagActionOverlay()
	}
	if m.regConfirmDeleteTags {
		return m.renderRegistryTagsDeleteConfirm(base)
	}
	return base
}

// renderRegistryVersionPane renders the right-hand versions/tags detail pane.
func (m rootModel) renderRegistryVersionPane(paneW, paneH int) string {
	borderColor := colBorder
	if m.regBrowserFocus == 1 {
		borderColor = colPurple
	}

	visible := m.filteredRegistryImages()
	if len(visible) == 0 || m.regBrowserCursor >= len(visible) {
		return panelBox("VERSIONS", paneW, paneH, borderColor,
			lipgloss.NewStyle().Faint(true).Render("Select an image"))
	}

	img := visible[m.regBrowserCursor]

	if m.regTagsLoading {
		return panelBox("VERSIONS", paneW, paneH, borderColor,
			lipgloss.NewStyle().Foreground(colComment).Render("Loading…"))
	}

	tags := m.filteredRegistryTags(img)

	const scrollW = 1
	const prefixW = 2
	const chkW = 4 // "[x] " or "[ ] "
	innerW := paneW - 2
	tagW := innerW - prefixW - scrollW - chkW

	tagListH := max(1, paneH-listRowOverhead)
	scrollY := m.regBrowserTagScrollY
	if m.regBrowserTagCursor >= scrollY+tagListH {
		scrollY = m.regBrowserTagCursor - tagListH + 1
	}
	if m.regBrowserTagCursor < scrollY {
		scrollY = m.regBrowserTagCursor
	}

	vScrollBar := renderVScrollBar(len(tags), scrollY, tagListH)

	// Title: show selected count, filter count, or plain count.
	var title string
	switch {
	case len(m.regTagSelected) > 0:
		title = fmt.Sprintf("VERSIONS (%d selected)", len(m.regTagSelected))
	case m.regTagFilter != "":
		title = fmt.Sprintf("VERSIONS (%d/%d)", len(tags), len(img.tags))
	default:
		title = fmt.Sprintf("VERSIONS (%d)", len(tags))
	}

	// Header: filter bar when filtering, otherwise column header with hint.
	var headerStr string
	switch {
	case m.regTagFiltering:
		headerStr = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.regTagFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.regTagFilter != "":
		headerStr = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.regTagFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	case m.regBrowserFocus == 1:
		const hintStr = "Enter  Pull"
		const hintW = len(hintStr)
		headerStr = strings.Repeat(" ", prefixW) +
			lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("TAG", tagW+chkW-hintW)) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hintStr)
	default:
		headerStr = strings.Repeat(" ", prefixW) +
			lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("TAG", tagW+chkW))
	}

	var rows []string
	if len(tags) == 0 {
		noMsg := "  No tags"
		if m.regTagFilter != "" {
			noMsg = "  No tags match \"" + m.regTagFilter + "\""
		}
		for si := 0; si < tagListH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(innerW-scrollW).Render(noMsg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", innerW-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+tagListH, len(tags))
	for i := scrollY; i < end; i++ {
		tag := tags[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		isSelected := m.regTagSelected[tag.name]
		isCursor := m.regBrowserFocus == 1 && i == m.regBrowserTagCursor

		var chk string
		if isSelected {
			chk = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x] ")
		} else {
			chk = lipgloss.NewStyle().Foreground(colComment).Render("[ ] ")
		}

		var tagCol string
		if m.regTagFilter != "" {
			tagCol = padRight(highlightMatch(tag.name, m.regTagFilter), tagW)
		} else {
			tagCol = lipgloss.NewStyle().Foreground(colGreen).Render(padRight(tag.name, tagW))
		}
		rowStr := chk + tagCol + sb

		if isCursor {
			plainChk := "[ ] "
			if isSelected {
				plainChk = "[x] "
			}
			plainRowStr := plainChk + padRight(tag.name, tagW) + sb
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(innerW).Render("▌ "+plainRowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(innerW).Render("  "+rowStr))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		headerStr,
		strings.Repeat("─", innerW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(title, paneW, paneH, borderColor, content)
}

// renderRegistryTagActionOverlay shows pull instructions for the tag currently
// selected in the versions pane.
func (m rootModel) renderRegistryTagActionOverlay() string {
	visible := m.filteredRegistryImages()
	if len(visible) == 0 || m.regBrowserCursor >= len(visible) {
		return ""
	}
	img := visible[m.regBrowserCursor]
	if len(img.tags) == 0 || m.regBrowserTagCursor >= len(img.tags) {
		return ""
	}
	tag := img.tags[m.regBrowserTagCursor]
	ns := m.regBrowserNamespace

	pullBase := ns.endpoint + "/" + img.name
	var pullCmd string
	if strings.HasPrefix(tag.name, "sha256-") {
		pullCmd = "docker pull " + pullBase + "@sha256:" + tag.name[len("sha256-"):]
	} else {
		pullCmd = "docker pull " + pullBase + ":" + tag.name
	}

	dialogW := min(m.width-8, 90)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	heading := bg.Foreground(colPurple).Bold(true).Width(innerW).Render("Pull Instructions")
	imgLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Image: ") +
			bg.Foreground(colFg).Render(img.name),
	)
	tagLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Tag:   ") +
			bg.Foreground(colGreen).Render(tag.name),
	)
	empty := bg.Width(innerW).Render("")

	codeBlock := lipgloss.NewStyle().
		Background(colBg3).Foreground(colGreen).
		Padding(0, 1).Width(innerW).
		Render("$ " + pullCmd)

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	closeBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(innerW).Align(lipgloss.Center).
		Render("Esc  Close")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, empty, imgLine, tagLine, empty, codeBlock, empty, divider, empty, closeBtn,
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

// renderRegistryTagsDeleteConfirm shows a delete confirmation for one or more tags.
func (m rootModel) renderRegistryTagsDeleteConfirm(base string) string {
	tags := m.regConfirmTagsToDelete
	if len(tags) == 0 {
		return base
	}
	n := len(tags)

	const dialogW = 54
	const innerW = dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	countStr := "1 tag"
	if n > 1 {
		countStr = fmt.Sprintf("%d tags", n)
	}
	heading := bg.Foreground(colRed).Bold(true).Width(innerW).Render("Delete " + countStr + "?")

	imgDisplay := m.regConfirmDeleteImgName
	if lipgloss.Width(imgDisplay) > innerW-8 {
		rr := []rune(imgDisplay)
		imgDisplay = string(rr[:innerW-9]) + "\u2026"
	}
	imgLine := bg.Width(innerW).Render(
		lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("Image: ") +
			lipgloss.NewStyle().Background(colBg2).Foreground(colFg).Render(imgDisplay),
	)

	const maxShow = 5
	var tagLines []string
	for i, t := range tags {
		if i >= maxShow {
			more := bg.Foreground(colComment).Faint(true).Width(innerW).
				Render(fmt.Sprintf("  \u2026 and %d more", n-maxShow))
			tagLines = append(tagLines, more)
			break
		}
		line := bg.Width(innerW).Render(
			lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("  \u00b7 ") +
				lipgloss.NewStyle().Background(colBg2).Foreground(colGreen).Bold(true).Render(t.name),
		)
		tagLines = append(tagLines, line)
	}

	warn := bg.Foreground(colComment).Width(innerW).Render("This action is irreversible.")
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("\u2500", innerW))
	empty := bg.Width(innerW).Render("")

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

	allLines := []string{heading, empty, imgLine, empty}
	allLines = append(allLines, tagLines...)
	allLines = append(allLines, empty, warn, empty, divider, empty, buttons)

	body := bg.Width(innerW).Render(lipgloss.JoinVertical(lipgloss.Left, allLines...))
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
