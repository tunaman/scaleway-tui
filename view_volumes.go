package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ─────────────────────────────────────────────
// Volumes & Snapshots view
// ─────────────────────────────────────────────

// volumeStateColor maps a volume/snapshot state to a palette color.
// "available"/"in_use" are the healthy resting states; everything transient
// (creating, deleting, snapshotting, resizing, ...) is yellow; failures are
// red.
func volumeStateColor(state string) color.Color {
	switch state {
	case "available", "in_use":
		return colGreen
	case "error", "invalid_data":
		return colRed
	}
	return colYellow
}

// volumeServerName resolves the display name of the server a volume is
// attached to. The Block API only carries the server ID (in the volume's
// references), so the name is looked up in the fetched server list; the
// stored name (set for legacy Instance-API volumes) wins when present.
func (m rootModel) volumeServerName(v volumeItem) string {
	if v.serverName != "" {
		return v.serverName
	}
	for _, s := range m.servers {
		if s.id == v.serverID {
			return s.name
		}
	}
	return ""
}

// volumeDisplayName shortens Kubernetes CSI volume/snapshot names for
// display: everything before the "pvc-" marker (the cluster-ID prefix, e.g.
// "<cluster-uuid>_pvc-…") is dropped. Names without the marker are returned
// unchanged. Filtering still matches the full name.
func volumeDisplayName(name string) string {
	if i := strings.Index(name, "pvc-"); i > 0 {
		return name[i:]
	}
	return name
}

// renderVolumeTabStrip renders the Volumes / Snapshots tab bar (same pill
// style as the IAM browser) with per-tab counts.
func (m rootModel) renderVolumeTabStrip() string {
	labels := []string{
		fmt.Sprintf("Volumes %d", len(m.filteredVolumes())),
		fmt.Sprintf("Snapshots %d", len(m.filteredSnapshots())),
	}
	pills := make([]string, len(labels))
	for i, label := range labels {
		if i == m.volFocus {
			pills[i] = lipgloss.NewStyle().
				Background(colPurple).Foreground(colBg).Bold(true).
				Render(" " + label + " ")
		} else {
			pills[i] = lipgloss.NewStyle().
				Foreground(colComment).
				Render(" " + label + " ")
		}
	}
	return strings.Join(pills, " ")
}

// renderVolumes draws the Volumes & Snapshots dashboard service: a list pane
// (←/→ switch between the Volumes and Snapshots tabs) plus a detail pane for the
// selected item.
func (m rootModel) renderVolumes(totalW, height int, borderColor color.Color) string {
	listW := totalW - detailPaneWidth - 1
	const scrollW = 1
	const prefixW = 2
	const colGap = 2
	const stateW = 12 + colGap // "snapshotting" + gap (not the last column)
	const sizeW = 10 + colGap
	const typeW = 10 + colGap
	const minLastW = 14 + colGap // SERVER / BASE

	isVols := m.volFocus == 0

	// Size NAME to the widest display name of the active tab (over the full
	// list, so the layout doesn't jump while filtering) and hand the rest of
	// the row to the last column (SERVER / BASE), which then fits a full
	// pvc-<uuid> base-volume name on wide terminals.
	nameMax := lipgloss.Width("NAME")
	if isVols {
		for _, v := range m.volumes {
			nameMax = max(nameMax, lipgloss.Width(volumeDisplayName(v.name)))
		}
	} else {
		for _, s := range m.snapshots {
			nameMax = max(nameMax, lipgloss.Width(volumeDisplayName(s.name)))
		}
	}
	avail := listW - 2 - prefixW - sizeW - typeW - stateW - scrollW
	nameW := max(10, min(nameMax+colGap, avail-minLastW))
	lastW := max(minLastW, avail-nameW)
	// Column text widths keep a trailing gap so a truncated value ("…") never
	// runs into the next column.
	nameTxt, sizeTxt, typeTxt, lastTxt := nameW-colGap, sizeW-colGap, typeW-colGap, lastW-colGap
	col := func(v string, txt, w int) string { return padRight(padRight(v, txt), w) }

	vols := m.filteredVolumes()
	snaps := m.filteredSnapshots()
	visibleLen := len(vols)
	if !isVols {
		visibleLen = len(snaps)
	}
	cursor, scrollY := m.volCursor, m.volScrollY
	if !isVols {
		cursor, scrollY = m.volSnapCursor, m.volSnapScrollY
	}

	listH := max(1, height-listRowOverhead-1) // -1 for the tab strip

	// ── Scroll viewport ──
	if cursor >= scrollY+listH {
		scrollY = cursor - listH + 1
	}
	if cursor < scrollY {
		scrollY = cursor
	}
	scrollY = max(0, scrollY)

	// ── Scrollbar column ──
	vScrollBar := renderVScrollBar(visibleLen, scrollY, listH)

	// ── Build visible rows ──
	var rows []string
	if visibleLen == 0 {
		msg := "  No volumes found in this project."
		if !isVols {
			msg = "  No snapshots found in this project."
		}
		if m.volFilter != "" {
			if isVols {
				msg = "  No volumes match \"" + m.volFilter + "\"."
			} else {
				msg = "  No snapshots match \"" + m.volFilter + "\"."
			}
		} else if m.volFetchErr != nil {
			// The fetch (partially) failed: say so instead of claiming the
			// project is empty.
			if isVols {
				msg = "  ✗ Volume fetch failed: " + m.volFetchErr.Error()
			} else {
				msg = "  ✗ Snapshot fetch failed: " + m.volFetchErr.Error()
			}
		}
		msgStyle := lipgloss.NewStyle().Faint(true)
		if m.volFetchErr != nil && m.volFilter == "" {
			msgStyle = lipgloss.NewStyle().Foreground(colRed)
		}
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, msgStyle.Width(listW-2-scrollW).Render(msg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", listW-2-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, visibleLen)
	for i := scrollY; i < end; i++ {
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		var name, sizeStr, typeStr, stateStr, lastStr string
		if isVols {
			v := vols[i]
			name, sizeStr, typeStr, stateStr = volumeDisplayName(v.name), formatBytes(int64(v.size)), v.volType, v.state // #nosec G115 -- volume sizes are far below MaxInt64
			lastStr = m.volumeServerName(v)
			if lastStr == "" {
				lastStr = "—"
			}
		} else {
			s := snaps[i]
			name, sizeStr, typeStr, stateStr = volumeDisplayName(s.name), formatBytes(int64(s.size)), s.volType, s.state // #nosec G115 -- snapshot sizes are far below MaxInt64
			lastStr = volumeDisplayName(s.baseVolume)
			if lastStr == "" {
				lastStr = "—"
			}
		}

		cols := col(name, nameTxt, nameW) +
			col(sizeStr, sizeTxt, sizeW) +
			col(typeStr, typeTxt, typeW)
		lastCol := col(lastStr, lastTxt, lastW)
		stateColored := lipgloss.NewStyle().Foreground(volumeStateColor(stateStr)).Render(padRight(stateStr, stateW))

		if i == cursor {
			// The scrollbar cell is appended outside the highlight: it carries
			// its own color codes, which would reset the background mid-row,
			// and the plain row must keep the full last-column width so the
			// scrollbar lines up with the other rows.
			plainCols := cols + padRight(stateStr, stateW) + lastCol
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(listW-2-scrollW).Render("▌ "+plainCols)+sb)
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(listW-2).Render(
				"  "+cols+stateColored+lastCol+sb))
		}
	}

	// ── Header / filter bar ──
	lastHeader := "SERVER"
	if !isVols {
		lastHeader = "BASE"
	}
	var header string
	switch {
	case m.volFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.volFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.volFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.volFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("NAME", nameW)+padRight("SIZE", sizeW)+padRight("TYPE", typeW)+
				padRight("STATE", stateW)+padRight(lastHeader, lastW),
		)
	}

	panelTitle := "VOLUMES"
	if !isVols {
		panelTitle = "SNAPSHOTS"
	}
	if m.volFilter != "" {
		total := len(m.volumes)
		if !isVols {
			total = len(m.snapshots)
		}
		panelTitle = fmt.Sprintf("%s  %d/%d", panelTitle, visibleLen, total)
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		m.renderVolumeTabStrip(),
		header,
		strings.Repeat("─", listW-2),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	listPane := panelBox(panelTitle, listW, height, borderColor, listContent)

	detailTitle := "VOLUME INFO"
	detail := m.renderVolumeDetail()
	if !isVols {
		detailTitle = "SNAPSHOT INFO"
		detail = m.renderSnapshotDetail()
	}
	detailPane := panelBox(detailTitle, detailPaneWidth, height, colPurple, detail)
	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
}

// volumeDetailKey renders one "Key  value" line of a detail pane.
func volumeDetailKey(key, val string, valColor color.Color) string {
	k := lipgloss.NewStyle().Foreground(colComment).Render(" " + padRight(key, 9))
	v := lipgloss.NewStyle().Foreground(valColor).Render(" " + val)
	return k + v
}

func (m rootModel) renderVolumeDetail() string {
	v := m.selectedVolume()
	if v == nil {
		return lipgloss.NewStyle().Faint(true).Render("Select a volume")
	}
	innerW := detailPaneWidth - 4
	nameDisplay := volumeDisplayName(v.name)
	if lipgloss.Width(nameDisplay) > innerW {
		nameDisplay = string([]rune(nameDisplay)[:innerW-1]) + "…"
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(" " + nameDisplay + " "),
		"",
		volumeDetailKey("Zone", v.zone, colBlue),
		volumeDetailKey("Type", v.volType, colFg),
		volumeDetailKey("Size", formatBytes(int64(v.size)), colFg), // #nosec G115 -- volume sizes are far below MaxInt64
		volumeDetailKey("State", v.state, volumeStateColor(v.state)),
		volumeDetailKey("Created", fmtDate(v.createdAt), colFg),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m rootModel) renderSnapshotDetail() string {
	s := m.selectedSnapshot()
	if s == nil {
		return lipgloss.NewStyle().Faint(true).Render("Select a snapshot")
	}
	innerW := detailPaneWidth - 4
	nameDisplay := volumeDisplayName(s.name)
	if lipgloss.Width(nameDisplay) > innerW {
		nameDisplay = string([]rune(nameDisplay)[:innerW-1]) + "…"
	}

	base := volumeDisplayName(s.baseVolume)
	if base == "" {
		base = "—"
	}
	lines := []string{
		lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(" " + nameDisplay + " "),
		"",
		volumeDetailKey("Zone", s.zone, colBlue),
		volumeDetailKey("Type", s.volType, colFg),
		volumeDetailKey("Size", formatBytes(int64(s.size)), colFg), // #nosec G115 -- snapshot sizes are far below MaxInt64
		volumeDetailKey("State", s.state, volumeStateColor(s.state)),
		volumeDetailKey("Base vol", base, colFg),
		volumeDetailKey("Created", fmtDate(s.createdAt), colFg),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ─────────────────────────────────────────────
// Volume/snapshot action confirm dialog
// ─────────────────────────────────────────────

func (m rootModel) renderVolumeConfirm(base string) string {
	var title, warn string
	var accent color.Color
	switch m.volConfirmAction {
	case volActionDeleteVolume:
		title = "Delete volume?"
		warn = "This action cannot be undone. All data on the volume will be lost."
		accent = colRed
	case volActionDeleteSnapshot:
		title = "Delete snapshot?"
		warn = "This action cannot be undone."
		accent = colRed
	case volActionAttach:
		title = "Attach volume?"
		warn = "The volume will be attached to the selected server."
		accent = colGreen
	default: // volActionDetach
		title = "Detach volume?"
		warn = "The volume will be detached from its server."
		accent = colYellow
	}

	dialogW := min(m.width-8, 60)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	name := m.volConfirmName
	if lipgloss.Width(name) > innerW-10 {
		name = string([]rune(name)[:innerW-11]) + "…"
	}
	heading := bg.Foreground(accent).Bold(true).Width(innerW).Render(title)
	empty := bg.Width(innerW).Render("")
	itemLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Volume:  ") +
			bg.Foreground(colFg).Render(name),
	)
	var serverLine string
	if m.volConfirmAction == volActionAttach || m.volConfirmAction == volActionDetach {
		serverLine = bg.Width(innerW).Render(
			bg.Foreground(colComment).Render("Server:  ") +
				bg.Foreground(colFg).Render(m.volConfirmServerName),
		)
	}
	warnLine := bg.Foreground(colYellow).Width(innerW).Render(warn)

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))
	actions := bg.Width(innerW).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Background(accent).Foreground(colBg).Bold(true).Padding(0, 2).Render("Y  Confirm"),
			lipgloss.NewStyle().Background(colBg3).Foreground(colFg).Padding(0, 2).Render("N  Cancel"),
		),
	)

	var bodyLines []string
	bodyLines = append(bodyLines, heading, empty, itemLine)
	if serverLine != "" {
		bodyLines = append(bodyLines, serverLine)
	}
	bodyLines = append(bodyLines, empty, warnLine, empty, divider, empty, actions)
	body := bg.Width(innerW).Render(lipgloss.JoinVertical(lipgloss.Left, bodyLines...))

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(colBg)),
	)
}

// ─────────────────────────────────────────────
// Attach server picker overlay
// ─────────────────────────────────────────────

// attachServersInZone returns the servers in the zone of the volume being
// attached, in the order they appear in the server list.
func (m rootModel) attachServersInZone() []instanceServer {
	var out []instanceServer
	for _, s := range m.servers {
		if s.zone == m.volAttachVolume.zone {
			out = append(out, s)
		}
	}
	return out
}

func (m rootModel) renderVolumeAttachOverlay(base string) string {
	dialogW := min(m.width-8, 62)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	servers := m.attachServersInZone()
	maxRows := 8

	heading := bg.Foreground(colGreen).Bold(true).Width(innerW).Render("Attach to which server?")
	sub := bg.Width(innerW).Render(bg.Foreground(colComment).Render(
		"Volume: " + m.volAttachVolume.name + "  ·  Zone: " + m.volAttachVolume.zone))

	var listLines []string
	if len(servers) == 0 {
		listLines = append(listLines, bg.Width(innerW).Render(
			bg.Foreground(colComment).Render("No servers in "+m.volAttachVolume.zone)))
	}
	end := min(len(servers), m.volAttachCursor+maxRows)
	start := max(0, end-maxRows)
	for i := start; i < end; i++ {
		s := servers[i]
		row := padRight(s.name, innerW-2-8-12) + padRight(s.commercialType, 8) + s.status
		if i == m.volAttachCursor {
			row = "▌ " + row
			listLines = append(listLines, bg.Foreground(colFg).Bold(true).
				Background(colBg3).Width(innerW).Render(row))
		} else {
			listLines = append(listLines, bg.Width(innerW).Render("  "+row))
		}
	}

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))
	hint := bg.Width(innerW).Render(bg.Foreground(colComment).Faint(true).Render(
		"↑↓  Navigate · Enter  Attach · Esc  Cancel"))

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, sub, "",
			lipgloss.JoinVertical(lipgloss.Left, listLines...),
			"", divider, "", hint,
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
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(colBg)),
	)
}
