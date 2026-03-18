package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Dashboard view
// ─────────────────────────────────────────────

func (m rootModel) drawDashboard() string {
	topBar := m.renderTopBar()
	statusBar := m.renderStatusBar()

	if m.err != nil {
		errPane := panelBox("ERROR", m.width-4, m.height-topBarHeight-statusBarHeight-4, colRed,
			lipgloss.NewStyle().Foreground(colRed).Render("✗ "+m.err.Error()),
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, errPane, statusBar),
		)
	}

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Syncing Scaleway...",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	contentH := m.height - topBarHeight - statusBarHeight - 6
	nav := m.renderNav(contentH)
	content := m.renderContent(contentH)

	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			topBar,
			lipgloss.JoinHorizontal(lipgloss.Top, nav, content),
			statusBar,
		),
	)

	if m.input.active {
		return m.renderInputOverlay(base)
	}
	return base
}

// ─────────────────────────────────────────────
// Top bar
// ─────────────────────────────────────────────

func (m rootModel) renderTopBar() string {
	projectLabel := lipgloss.NewStyle().Foreground(colComment).Render("PROJECT ")
	projectVal := lipgloss.NewStyle().
		Foreground(colGreen).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Render(" " + m.project + " ")

	region := lipgloss.NewStyle().Foreground(colComment).Render("  Region: ") +
		lipgloss.NewStyle().Foreground(colBlue).Render(" "+m.activeRegion+" ")
	clock := lipgloss.NewStyle().Foreground(colComment).Render(" " + time.Now().Format("15:04") + " ")

	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, projectLabel, projectVal, region)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(clock)-8))
	row := leftPart + spacer + clock

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(row)
}

// ─────────────────────────────────────────────
// Status bar
// ─────────────────────────────────────────────

func (m rootModel) renderStatusBar() string {
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("F5", "Refresh"),
		hotkey("Tab", "Focus"),
		hotkey("↑↓", "Navigate"),
		hotkey("Enter", "Open"),
		hotkey("/", "Filter"),
		hotkey("C", "New bucket"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	// barW matches the Width passed to the outer style — no extra Padding() so
	// there is no off-by-two and no background-colour rectangle at the right edge.
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer))
}

// ─────────────────────────────────────────────
// Nav panel
// ─────────────────────────────────────────────

func (m rootModel) renderNav(height int) string {
	services := []struct{ label string }{
		{"Object Storage"},
		{"K8s Clusters"},
		{"Billing"},
		{"Container Registry"},
		{"Secrets Manager"},
	}

	sectionHeader := lipgloss.NewStyle().Foreground(colComment).PaddingLeft(1).PaddingBottom(1).Render("SERVICES")

	var rows []string
	rows = append(rows, sectionHeader)
	for i, svc := range services {
		if i == m.activeService {
			label := lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(svc.label)
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).PaddingLeft(1).Width(navWidth-2).
				Render(label))
		} else {
			label := lipgloss.NewStyle().Foreground(colComment).Render(svc.label)
			rows = append(rows, lipgloss.NewStyle().PaddingLeft(1).Width(navWidth-2).Render(label))
		}
	}

	focusColor := colBorder
	if m.focus == focusNav {
		focusColor = colRed
	}
	return panelBox("NAV", navWidth, height, focusColor,
		lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// ─────────────────────────────────────────────
// Content panel
// ─────────────────────────────────────────────

func (m rootModel) renderContent(height int) string {
	contentW := m.width - navWidth - 8
	focusColor := colBorder
	if m.focus == focusContent {
		focusColor = colBlue
	}
	switch m.activeService {
	case serviceObjectStorage:
		return m.renderBuckets(contentW, height, focusColor)
	case serviceK8s:
		return m.renderClusters(contentW, height, focusColor)
	case serviceBilling:
		return m.renderBillingPreview(contentW, height, focusColor)
	case serviceRegistry:
		return m.renderRegistry(contentW, height, focusColor)
	case serviceSecrets:
		return m.renderSecrets(contentW, height, focusColor)
	}
	return ""
}

// ─────────────────────────────────────────────
// Object Storage view
// ─────────────────────────────────────────────

func (m rootModel) renderBuckets(totalW, height int, borderColor lipgloss.Color) string {
	listW := totalW - detailPaneWidth - 1
	// scrollW=1 col reserved inside content for the vertical scrollbar.
	// Row layout: prefix(2) + name(nameW) + scrollbar(1) = innerW = listW-2
	scrollW := 1
	nameW := listW - 2 - 2 - scrollW // innerW(listW-2) - prefix(2) - scrollbar(1)

	visible := m.filteredBuckets()
	listH := max(1, height-listRowOverhead)

	// ── Scroll viewport ──
	scrollY := m.bucketScrollY
	if m.bucketCursor >= scrollY+listH {
		scrollY = m.bucketCursor - listH + 1
	}
	if m.bucketCursor < scrollY {
		scrollY = m.bucketCursor
	}
	scrollY = max(0, scrollY)

	// ── Scrollbar column ──
	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	// ── Build visible rows ──
	var rows []string
	if len(visible) == 0 {
		msg := "  No buckets found in this project."
		if m.bucketFilter != "" {
			msg = "  No buckets match \"" + m.bucketFilter + "\"."
		}
		// Pad with scrollbar chars on the right.
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(listW-2-scrollW).Render(msg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", listW-2-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		b := visible[i]
		var name string
		if m.bucketFilter != "" {
			name = highlightMatch(b.name, m.bucketFilter)
		} else {
			name = b.name
			runes := []rune(name)
			if m.bucketScrollX > 0 {
				if m.bucketScrollX >= len(runes) {
					name = ""
				} else {
					name = string(runes[m.bucketScrollX:])
				}
			}
		}
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}
		var rowStr string
		if i == m.bucketCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(listW - 2).Render("▌ " + padRight(name, nameW) + sb)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(listW - 2).Render("  " + padRight(name, nameW) + sb)
		}
		rows = append(rows, rowStr)
	}

	// ── Header / filter bar ──
	var header string
	switch {
	case m.bucketFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.bucketFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.bucketFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.bucketFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		hint := ""
		if m.bucketScrollX > 0 {
			hint = fmt.Sprintf(" ◀+%d", m.bucketScrollX)
		}
		hintW := lipgloss.Width(hint)
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("NAME", nameW-hintW))
		if hint != "" {
			header += lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hint)
		}
	}

	panelTitle := "OBJECT STORAGE"
	if m.bucketFilter != "" {
		panelTitle = fmt.Sprintf("OBJECT STORAGE  %d/%d", len(visible), len(m.buckets))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", listW-2),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	// No gutter — right border is always the plain panel border.
	listPane := panelBox(panelTitle, listW, height, borderColor, listContent)
	detailPane := panelBox("BUCKET INFO", detailPaneWidth, height, colPurple, m.renderBucketDetail())
	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
}

func (m rootModel) renderBucketDetail() string {
	fb := m.filteredBuckets()
	if len(fb) == 0 || m.bucketCursor >= len(fb) {
		return lipgloss.NewStyle().Faint(true).Render("Select a bucket")
	}
	b := fb[m.bucketCursor]

	// Inner width: subtract borders (2) and padding (2).
	innerW := detailPaneWidth - 4
	nameDisplay := b.name
	if lipgloss.Width(nameDisplay) > innerW {
		nameDisplay = string([]rune(nameDisplay)[:innerW-1]) + "…"
	}

	// Align values in the Usage block by padding keys to the same width.
	usageKey := func(key, val string, valColor lipgloss.Color) string {
		k := lipgloss.NewStyle().Foreground(colComment).Render(padRight(key, 9))
		v := lipgloss.NewStyle().Foreground(valColor).Render(" " + val + " ")
		return k + v
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(" " + nameDisplay + " "),
		"",
		lipgloss.NewStyle().Foreground(colComment).Render("Created: ") +
			lipgloss.NewStyle().Foreground(colFg).Render(" "+b.created+" "),
		lipgloss.NewStyle().Foreground(colComment).Render("Region:  ") +
			lipgloss.NewStyle().Foreground(colBlue).Render(" "+m.activeRegion+" "),
		"",
		lipgloss.NewStyle().Foreground(colComment).Bold(true).Render("Usage:"),
	}

	if b.sizeReady {
		lines = append(lines,
			usageKey("Objects:", fmt.Sprintf("%d", b.objCount), colGreen),
			usageKey("Size:", formatBytes(b.sizeBytes), colBlue),
		)
	} else {
		lines = append(lines,
			lipgloss.NewStyle().Faint(true).Render("  Calculating…"),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ─────────────────────────────────────────────
// K8s Clusters view
// ─────────────────────────────────────────────

func (m rootModel) renderClusters(totalW, height int, borderColor lipgloss.Color) string {
	nameW := totalW - 30
	statusW := 12
	versionW := 10

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("CLUSTER", nameW) + padRight("VERSION", versionW) + padRight("STATUS", statusW),
	)

	var rows []string
	if len(m.clusters) == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("No clusters found in this region."))
	}
	for i, cl := range m.clusters {
		statusColor := colGreen
		switch strings.ToLower(cl.status) {
		case "warning", "upgrading", "scaling":
			statusColor = colYellow
		case "error", "locked", "unknown":
			statusColor = colRed
		}
		status := lipgloss.NewStyle().Foreground(statusColor).Render(cl.status)
		rowStr := padRight(cl.name, nameW) + padRight(cl.version, versionW) + status
		if i == m.clusterCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW - 4).Render("▌ " + rowStr)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(totalW - 4).Render("  " + rowStr)
		}
		rows = append(rows, rowStr)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", totalW-4),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox("K8S CLUSTERS", totalW, height, borderColor, content)
}

// ─────────────────────────────────────────────
// Container Registry view
// ─────────────────────────────────────────────

func (m rootModel) renderRegistry(totalW, height int, borderColor lipgloss.Color) string {
	nameW := totalW - 32
	imagesW := 8
	sizeW := 10
	visW := 8

	visible := m.filteredRegistryNamespaces()
	listH := max(1, height-listRowOverhead)
	scrollY := m.registryScrollY
	if m.registryCursor >= scrollY+listH {
		scrollY = m.registryCursor - listH + 1
	}
	if m.registryCursor < scrollY {
		scrollY = m.registryCursor
	}

	var rows []string
	if len(visible) == 0 {
		msg := "  No container registry namespaces found."
		if m.registryFilter != "" {
			msg = "  No namespaces match \"" + m.registryFilter + "\"."
		}
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render(msg))
	}
	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		ns := visible[i]

		statusColor := colGreen
		switch ns.status {
		case "error", "locked":
			statusColor = colRed
		case "deleting":
			statusColor = colYellow
		}
		vis := "private"
		if ns.isPublic {
			vis = "public"
		}
		sizeStr := formatBytes(int64(ns.sizeBytes))
		imagesStr := fmt.Sprintf("%d", ns.imageCount)

		var nameStr string
		if m.registryFilter != "" {
			nameStr = highlightMatch(ns.name, m.registryFilter)
		} else {
			nameStr = lipgloss.NewStyle().Foreground(statusColor).Render(ns.name)
		}
		rowStr := padRight(nameStr, nameW) + padRight(imagesStr, imagesW) + padRight(sizeStr, sizeW) + padRight(vis, visW)

		if i == m.registryCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW-4).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(totalW-4).Render("  "+rowStr))
		}
	}

	// ── Header / filter bar ──
	var header string
	switch {
	case m.registryFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.registryFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.registryFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.registryFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("NAMESPACE", nameW)+padRight("IMAGES", imagesW)+padRight("SIZE", sizeW)+padRight("VIS", visW),
		)
	}

	panelTitle := "CONTAINER REGISTRY"
	if m.registryFilter != "" {
		panelTitle = fmt.Sprintf("CONTAINER REGISTRY  %d/%d", len(visible), len(m.registryNamespaces))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", totalW-4),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(panelTitle, totalW, height, borderColor, content)
}

// ─────────────────────────────────────────────
// Secrets Manager view
// ─────────────────────────────────────────────

func (m rootModel) renderSecrets(totalW, height int, borderColor lipgloss.Color) string {
	// scrollW=1 col reserved for the vertical scrollbar.
	// Row layout: prefix(2) + nameW + versionsW + statusW + scrollbar(1) = innerW = totalW-2
	const scrollW = 1
	const versionsW = 8
	const statusW = 12
	innerW := totalW - 2
	nameW := innerW - 2 - versionsW - statusW - scrollW
	if nameW < 8 {
		nameW = 8
	}

	visible := m.filteredSecrets()
	listH := max(1, height-listRowOverhead)
	scrollY := m.secretScrollY
	if m.secretCursor >= scrollY+listH {
		scrollY = m.secretCursor - listH + 1
	}
	if m.secretCursor < scrollY {
		scrollY = m.secretCursor
	}

	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	var rows []string
	if len(visible) == 0 {
		msg := "  No secrets found in this project."
		if m.secretFilter != "" {
			msg = "  No secrets match \"" + m.secretFilter + "\"."
		}
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(innerW-scrollW).Render(msg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", innerW-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		s := visible[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		statusColor := colGreen
		if s.status == "locked" {
			statusColor = colRed
		}

		versionsStr := fmt.Sprintf("%d", s.versionCount)
		var nameCol string
		if i == m.secretCursor {
			nameCol = padRight(s.name, nameW) // plain — keeps Background(colBg3) unbroken across the full row
		} else if m.secretFilter != "" {
			nameCol = padRight(highlightMatch(s.name, m.secretFilter), nameW)
		} else {
			nameCol = lipgloss.NewStyle().Foreground(statusColor).Render(padRight(s.name, nameW))
		}
		rowStr := nameCol + padRight(versionsStr, versionsW) + padRight(s.status, statusW) + sb

		if i == m.secretCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(innerW).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(innerW).Render("  "+rowStr))
		}
	}

	// Fill any remaining rows up to listH with blank lines + scrollbar chars.
	for si := len(rows); si < listH; si++ {
		sb := ""
		if si < len(vScrollBar) {
			sb = vScrollBar[si]
		}
		rows = append(rows, strings.Repeat(" ", innerW-scrollW)+sb)
	}

	var header string
	switch {
	case m.secretFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.secretFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.secretFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.secretFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("NAME", nameW)+padRight("VERS", versionsW)+padRight("STATUS", statusW),
		)
	}

	panelTitle := "SECRETS MANAGER"
	if m.secretFilter != "" {
		panelTitle = fmt.Sprintf("SECRETS MANAGER  %d/%d", len(visible), len(m.secrets))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", totalW-4),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(panelTitle, totalW, height, borderColor, content)
}

// ─────────────────────────────────────────────
// Billing preview (dashboard content area)
// ─────────────────────────────────────────────

// renderBillingPreview is shown briefly in the dashboard content area while
// billing data is loading (auto-triggered on nav selection).
func (m rootModel) renderBillingPreview(totalW, height int, borderColor lipgloss.Color) string {
	inner := lipgloss.Place(
		totalW-4, height-listRowOverhead,
		lipgloss.Center, lipgloss.Center,
		m.spin.View()+" Loading billing data…",
	)
	return panelBox("BILLING", totalW, height, borderColor, inner)
}
