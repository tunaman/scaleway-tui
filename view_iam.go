package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ─────────────────────────────────────────────
// IAM browser view (read-only, tabbed)
// ─────────────────────────────────────────────

var iamTabNames = []string{"Users", "Applications", "Groups", "Policies", "API keys", "Logs"}

// iamResolveName maps an IAM resource ID to a display name (user email / app /
// group name) when it is known, otherwise returns the raw ID.
func (m rootModel) iamResolveName(id string) string {
	if id == "" {
		return ""
	}
	if n, ok := m.iamNames[id]; ok && n != "" {
		return n
	}
	return id
}

// iamTabCounts returns the unfiltered row count for each tab (for the strip).
func (m rootModel) iamTabCounts() [iamTabCount]int {
	return [iamTabCount]int{
		len(m.iamUsers),
		len(m.iamApplications),
		len(m.iamGroups),
		len(m.iamPolicies),
		len(m.iamAPIKeys),
		len(m.iamLogs),
	}
}

// iamVisibleCount returns the number of rows in the active (filtered) tab.
func (m rootModel) iamVisibleCount() int {
	switch m.iamTab {
	case iamTabUsers:
		return len(m.filteredIAMUsers())
	case iamTabApplications:
		return len(m.filteredIAMApplications())
	case iamTabGroups:
		return len(m.filteredIAMGroups())
	case iamTabPolicies:
		return len(m.filteredIAMPolicies())
	case iamTabAPIKeys:
		return len(m.filteredIAMAPIKeys())
	case iamTabLogs:
		return len(m.filteredIAMLogs())
	}
	return 0
}

// renderIAMTabStrip renders the horizontal tab bar with per-tab counts.
func (m rootModel) renderIAMTabStrip() string {
	counts := m.iamTabCounts()
	var pills []string
	for i, name := range iamTabNames {
		label := fmt.Sprintf("%s %d", name, counts[i])
		if i == m.iamTab {
			pills = append(pills, lipgloss.NewStyle().
				Background(colPurple).Foreground(colBg).Bold(true).
				Render(" "+label+" "))
		} else {
			pills = append(pills, lipgloss.NewStyle().
				Foreground(colComment).
				Render(" "+label+" "))
		}
	}
	return strings.Join(pills, " ")
}

func (m rootModel) drawIAMBrowser() string {
	// ── Top bar ──
	topBar := m.renderTopBar()

	// ── Status bar ──
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("←→", "Tab"),
		hotkey("↑↓", "Navigate"),
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
			m.spin.View()+" Loading IAM…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	// ── Layout ──
	contentW := m.width - 8
	contentH := m.height - topBarHeight - statusBarHeight - 6
	listW := contentW
	const scrollW = 1
	const prefixW = 2
	rowW := listW - 2
	colsW := rowW - prefixW - scrollW // space available for all columns
	if colsW < 10 {
		colsW = 10
	}

	// One extra content line is used by the tab strip, so shrink the list body.
	listH := max(1, contentH-listRowOverhead-1)
	cursor := m.iamCursor
	scrollY := m.iamScrollY
	if cursor >= scrollY+listH {
		scrollY = cursor - listH + 1
	}
	if cursor < scrollY {
		scrollY = cursor
	}
	scrollY = max(0, scrollY)

	header, rowStrings := m.iamTabContent(colsW)

	n := len(rowStrings)
	vScrollBar := renderVScrollBar(n, scrollY, listH)

	// ── Filter bar overrides the column header while active ──
	var listHeader string
	switch {
	case m.iamFiltering:
		listHeader = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.iamFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.iamFilter != "":
		listHeader = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.iamFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		listHeader = "  " + header
	}

	// ── Rows ──
	var rows []string
	if n == 0 {
		noMsg := "  No " + strings.ToLower(iamTabNames[m.iamTab]) + " found."
		if m.iamFilter != "" {
			noMsg = "  No matches for \"" + m.iamFilter + "\"."
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

	end := min(scrollY+listH, n)
	for i := scrollY; i < end; i++ {
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}
		if i == cursor {
			// Plain text so the highlight background spans the whole row —
			// inline color resets would otherwise break it partway across.
			plain := stripANSI(rowStrings[i] + sb)
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+plain))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+rowStrings[i]+sb))
		}
	}
	// Pad to a full body so the panel height is stable.
	for si := len(rows); si < listH; si++ {
		sb := ""
		if si < len(vScrollBar) {
			sb = vScrollBar[si]
		}
		rows = append(rows, strings.Repeat(" ", rowW-scrollW)+sb)
	}

	panelTitle := "IAM · " + iamTabNames[m.iamTab]
	if m.iamFilter != "" {
		panelTitle = fmt.Sprintf("IAM · %s  %d/%d", iamTabNames[m.iamTab], n, m.iamTabCounts()[m.iamTab])
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		m.renderIAMTabStrip(),
		listHeader,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	listPane := panelBox(panelTitle, listW, contentH, colPurple, listContent)

	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, listPane, statusBar),
	)
}

// iamTabContent returns the column header and the fully-formatted (colored,
// padded) row strings for the active tab. Each returned row is exactly colsW
// visible columns wide.
func (m rootModel) iamTabContent(colsW int) (string, []string) {
	comment := func(key string, w int) string {
		return lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight(key, w))
	}

	switch m.iamTab {
	case iamTabUsers:
		typeW, statusW, mfaW, loginW := 9, 12, 6, 14
		nameW := colsW - typeW - statusW - mfaW - loginW
		if nameW < 10 {
			nameW = 10
		}
		header := comment("USER", nameW) + comment("TYPE", typeW) + comment("STATUS", statusW) + comment("MFA", mfaW) + comment("LAST LOGIN", loginW)
		var rows []string
		for _, u := range m.filteredIAMUsers() {
			typeStr := u.userType
			typeColor := colComment
			if u.userType == "owner" {
				typeColor = colYellow
			}
			statusColor := colGreen
			if u.status != "activated" {
				statusColor = colYellow
			}
			mfa := lipgloss.NewStyle().Foreground(colGreen).Render(padRight("✓", mfaW))
			if !u.mfa {
				mfa = lipgloss.NewStyle().Foreground(colRed).Render(padRight("✗", mfaW))
			}
			row := padRight(u.email, nameW) +
				lipgloss.NewStyle().Foreground(typeColor).Render(padRight(typeStr, typeW)) +
				lipgloss.NewStyle().Foreground(statusColor).Render(padRight(iamNonEmpty(u.status), statusW)) +
				mfa +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(fmtDate(u.lastLoginAt), loginW))
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows

	case iamTabApplications:
		keysW := 10
		nameW := (colsW - keysW) * 2 / 5
		if nameW < 12 {
			nameW = 12
		}
		descW := colsW - nameW - keysW
		if descW < 8 {
			descW = 8
		}
		header := comment("NAME", nameW) + comment("DESCRIPTION", descW) + comment("API KEYS", keysW)
		var rows []string
		for _, a := range m.filteredIAMApplications() {
			row := lipgloss.NewStyle().Foreground(colBlue).Render(padRight(a.name, nameW)) +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(a.description, descW)) +
				padRight(fmt.Sprintf("%d", a.nbAPIKeys), keysW)
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows

	case iamTabGroups:
		usersW, appsW := 8, 8
		nameW := (colsW - usersW - appsW) * 2 / 5
		if nameW < 12 {
			nameW = 12
		}
		descW := colsW - nameW - usersW - appsW
		if descW < 8 {
			descW = 8
		}
		header := comment("NAME", nameW) + comment("DESCRIPTION", descW) + comment("USERS", usersW) + comment("APPS", appsW)
		var rows []string
		for _, g := range m.filteredIAMGroups() {
			row := lipgloss.NewStyle().Foreground(colBlue).Render(padRight(g.name, nameW)) +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(g.description, descW)) +
				padRight(fmt.Sprintf("%d", g.nbUsers), usersW) +
				padRight(fmt.Sprintf("%d", g.nbApps), appsW)
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows

	case iamTabPolicies:
		rulesW := 8
		nameW := (colsW - rulesW) * 2 / 5
		if nameW < 12 {
			nameW = 12
		}
		princW := colsW - nameW - rulesW
		if princW < 10 {
			princW = 10
		}
		header := comment("NAME", nameW) + comment("RULES", rulesW) + comment("PRINCIPAL", princW)
		var rows []string
		for _, p := range m.filteredIAMPolicies() {
			princ := p.principalKind
			if p.principalID != "" {
				princ = p.principalKind + ": " + m.iamResolveName(p.principalID)
			}
			row := lipgloss.NewStyle().Foreground(colBlue).Render(padRight(p.name, nameW)) +
				padRight(fmt.Sprintf("%d", p.nbRules), rulesW) +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(princ, princW))
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows

	case iamTabAPIKeys:
		createdW, expiresW := 13, 13
		keyW := 22
		bearerW := (colsW - keyW - createdW - expiresW) * 2 / 5
		if bearerW < 12 {
			bearerW = 12
		}
		descW := colsW - keyW - bearerW - createdW - expiresW
		if descW < 6 {
			descW = 6
		}
		header := comment("ACCESS KEY", keyW) + comment("DESCRIPTION", descW) + comment("BEARER", bearerW) + comment("CREATED", createdW) + comment("EXPIRES", expiresW)
		var rows []string
		for _, k := range m.filteredIAMAPIKeys() {
			bearer := k.bearerKind
			if k.bearerID != "" {
				bearer = m.iamResolveName(k.bearerID)
			}
			expires := "Never"
			expColor := colComment
			if !k.expiresAt.IsZero() {
				expires = fmtDate(k.expiresAt)
				expColor = colYellow
			}
			row := lipgloss.NewStyle().Foreground(colBlue).Render(padRight(k.accessKey, keyW)) +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(k.description, descW)) +
				padRight(bearer, bearerW) +
				lipgloss.NewStyle().Foreground(colComment).Render(padRight(fmtDate(k.createdAt), createdW)) +
				lipgloss.NewStyle().Foreground(expColor).Render(padRight(expires, expiresW))
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows

	case iamTabLogs:
		dateW, actionW := 18, 10
		resW := (colsW - dateW - actionW) * 2 / 5
		if resW < 12 {
			resW = 12
		}
		byW := colsW - dateW - resW - actionW
		if byW < 10 {
			byW = 10
		}
		header := comment("DATE", dateW) + comment("RESOURCE", resW) + comment("ACTION", actionW) + comment("PERFORMED BY", byW)
		var rows []string
		for _, l := range m.filteredIAMLogs() {
			actionColor := colComment
			switch l.action {
			case "created":
				actionColor = colGreen
			case "updated":
				actionColor = colYellow
			case "deleted":
				actionColor = colRed
			}
			res := l.resourceType
			if l.resourceID != "" {
				res = l.resourceType + " " + l.resourceID
			}
			row := lipgloss.NewStyle().Foreground(colComment).Render(padRight(fmtDateTime(l.createdAt), dateW)) +
				padRight(res, resW) +
				lipgloss.NewStyle().Foreground(actionColor).Render(padRight(l.action, actionW)) +
				padRight(m.iamResolveName(l.bearerID), byW)
			rows = append(rows, padRight(row, colsW))
		}
		return header, rows
	}
	return "", nil
}

// ─────────────────────────────────────────────
// IAM dashboard preview pane (serviceIAM, not yet entered)
// ─────────────────────────────────────────────

func (m rootModel) renderIAMPreview(totalW, height int, borderColor color.Color) string {
	lines := []string{
		lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(" Identity & Access Management"),
		"",
		lipgloss.NewStyle().Foreground(colComment).Render(" Organization-level identities and access."),
		"",
	}
	items := []string{"Users", "Applications", "Groups", "Policies", "API keys", "Logs"}
	for _, it := range items {
		lines = append(lines, lipgloss.NewStyle().Foreground(colPurple).Render("  • ")+
			lipgloss.NewStyle().Foreground(colFg).Render(it))
	}
	lines = append(lines,
		"",
		lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(" Press Enter to browse."),
	)
	return panelBox("IAM", totalW, height, borderColor, lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// ─────────────────────────────────────────────
// Small formatting helpers
// ─────────────────────────────────────────────

func iamNonEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
