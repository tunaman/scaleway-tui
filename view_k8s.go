package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// K8s browser view
// ─────────────────────────────────────────────

func (m rootModel) drawK8sBrowser() string {
	cl := m.k8sBrowserCluster

	// ── Top bar ──
	pill := func(text string) string {
		return lipgloss.NewStyle().
			Foreground(colBlue).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBlue).
			Padding(0, 1).
			Render(text)
	}
	crumb := lipgloss.NewStyle().Foreground(colComment).Render("K8S ")
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center,
		crumb,
		pill(cl.name),
		lipgloss.NewStyle().Foreground(colComment).Render(" "),
		pill("v"+cl.version),
		lipgloss.NewStyle().Foreground(colComment).Render(" "),
		pill(cl.region),
	)
	topBar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).Padding(0, 1).
		Render(leftPart)

	// ── Status bar ──
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("↑↓", "Navigate"),
		hotkey("Tab", "Switch pane"),
		hotkey("R", "Reboot node"),
		hotkey("X", "Replace node"),
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

	// ── Layout ──
	contentW := m.width - 8
	contentH := m.height - topBarHeight - statusBarHeight - 6

	// Pool pane takes 68% of content, node pane takes the rest.
	poolPaneW := contentW * 68 / 100
	if poolPaneW < 60 {
		poolPaneW = 60
	}
	nodePaneW := contentW - poolPaneW - 1

	poolFocusColor := colBorder
	nodeFocusColor := colBorder
	if m.k8sBrowserFocus == 0 {
		poolFocusColor = colBlue
	} else {
		nodeFocusColor = colBlue
	}

	poolPane := m.renderK8sPoolPane(poolPaneW, contentH, poolFocusColor)
	nodePane := m.renderK8sNodePane(nodePaneW, contentH, nodeFocusColor)

	content := lipgloss.JoinHorizontal(lipgloss.Top, poolPane, nodePane)
	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.k8sConfirmReboot {
		return m.renderK8sRebootConfirm(base)
	}
	if m.k8sConfirmReplace {
		return m.renderK8sReplaceConfirm(base)
	}
	return base
}

// poolVolumeStr formats volume type and size into a short human-readable string.
func poolVolumeStr(volType string, volBytes uint64) string {
	typePart := ""
	switch volType {
	case "l_ssd":
		typePart = "Local SSD"
	case "b_ssd":
		typePart = "Block SSD"
	case "sbs_5k", "sbs-5k":
		typePart = "SBS-5K"
	case "sbs_15k", "sbs-15k":
		typePart = "SBS-15K"
	case "default_volume_type", "":
		typePart = "Block SSD"
	default:
		typePart = volType
	}
	if volBytes == 0 {
		return typePart
	}
	gb := volBytes / (1024 * 1024 * 1024)
	return fmt.Sprintf("%s %d GB", typePart, gb)
}

// poolZoneShort turns "fr-par-1" → "PAR-1", "nl-ams-1" → "AMS-1", etc.
func poolZoneShort(zone string) string {
	parts := strings.Split(zone, "-")
	if len(parts) < 3 {
		return zone
	}
	return strings.ToUpper(parts[1]) + "-" + parts[2]
}

func (m rootModel) renderK8sPoolPane(paneW, paneH int, borderColor lipgloss.Color) string {
	pools := m.k8sBrowserNodePools
	listH := max(1, paneH-listRowOverhead)
	rowW := paneW - 2
	const scrollW = 1
	const prefixW = 2
	const nodeTypeW = 13 // "POP2_16C_64G" = 12
	const nodesW = 7     // "4" + padding
	const rangeW = 8     // "1-6" + padding
	const autoW = 11     // "AUTO SCALE" = 10
	const healW = 5      // "On" / "Off"
	const zoneW = 6      // "AMS-1" = 5
	const volumeW = 17   // "Block SSD 186 GB" = 16
	const fixedW = nodeTypeW + nodesW + rangeW + autoW + healW + zoneW + volumeW + scrollW + prefixW
	nameW := rowW - fixedW
	if nameW < 10 {
		nameW = 10
	}

	scrollY := m.k8sBrowserPoolScrollY
	if m.k8sBrowserPoolCursor >= scrollY+listH {
		scrollY = m.k8sBrowserPoolCursor - listH + 1
	}
	if m.k8sBrowserPoolCursor < scrollY {
		scrollY = m.k8sBrowserPoolCursor
	}

	vScrollBar := renderVScrollBar(len(pools), scrollY, listH)

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		"  " +
			padRight("NAME", nameW) +
			padRight("NODE TYPE", nodeTypeW) +
			padRight("NODES", nodesW) +
			padRight("RANGE", rangeW) +
			padRight("AUTO SCALE", autoW) +
			padRight("HEAL", healW) +
			padRight("ZONE", zoneW) +
			padRight("SYSTEM VOLUME", volumeW),
	)

	// onOffPlain returns a plain padded string (no ANSI) — used for the
	// selected row so that the selection background isn't cut by inner resets.
	onOffPlain := func(b bool, w int) string {
		if b {
			return padRight("On", w)
		}
		return padRight("Off", w)
	}
	// onOffColored returns a colored string — used for unselected rows.
	onOffColored := func(b bool, w int) string {
		if b {
			return lipgloss.NewStyle().Foreground(colGreen).Render(padRight("On", w))
		}
		return lipgloss.NewStyle().Foreground(colComment).Render(padRight("Off", w))
	}

	var rows []string
	if len(pools) == 0 {
		noMsg := "  No pools found."
		for si := range listH {
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

	end := min(scrollY+listH, len(pools))
	for i := scrollY; i < end; i++ {
		p := pools[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		nodesStr := fmt.Sprintf("%d", p.size)
		rangeStr := fmt.Sprintf("%d-%d", p.minSize, p.maxSize)
		volStr := poolVolumeStr(p.rootVolumeType, p.rootVolumeSize)
		zoneStr := poolZoneShort(p.zone)

		cols := func(autoFn func(bool, int) string) string {
			return padRight(p.name, nameW) +
				padRight(p.nodeType, nodeTypeW) +
				padRight(nodesStr, nodesW) +
				padRight(rangeStr, rangeW) +
				autoFn(p.autoscaling, autoW) +
				autoFn(p.autohealing, healW) +
				padRight(zoneStr, zoneW) +
				padRight(volStr, volumeW) +
				sb
		}

		if i == m.k8sBrowserPoolCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+cols(onOffPlain)))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+cols(onOffColored)))
		}
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox("NODE POOLS", paneW, paneH, borderColor, listContent)
}

func (m rootModel) renderK8sNodePane(paneW, paneH int, borderColor lipgloss.Color) string {
	nodes := m.k8sBrowserNodes
	listH := max(1, paneH-listRowOverhead)
	rowW := paneW - 2
	const scrollW = 1
	const statusW = 10 // fits "ready", "not_ready"; truncates rare "creation_error"
	const prefixW = 2
	nameW := rowW - statusW - scrollW - prefixW
	if nameW < 8 {
		nameW = 8
	}

	scrollY := m.k8sBrowserNodeScrollY
	if m.k8sBrowserNodeCursor >= scrollY+listH {
		scrollY = m.k8sBrowserNodeCursor - listH + 1
	}
	if m.k8sBrowserNodeCursor < scrollY {
		scrollY = m.k8sBrowserNodeCursor
	}

	vScrollBar := renderVScrollBar(len(nodes), scrollY, listH)

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		"  " + padRight("NODE", nameW) + padRight("STATUS", statusW),
	)

	var rows []string

	if m.k8sNodesLoading {
		loadingRow := lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  " + m.spin.View() + " Loading nodes…")
		for si := 0; si < listH; si++ {
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Width(rowW).Render(loadingRow))
			} else {
				rows = append(rows, strings.Repeat(" ", rowW))
			}
		}
	} else if len(nodes) == 0 {
		noMsg := "  No nodes found."
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
	} else {
		end := min(scrollY+listH, len(nodes))
		for i := scrollY; i < end; i++ {
			n := nodes[i]
			sb := ""
			if i-scrollY < len(vScrollBar) {
				sb = vScrollBar[i-scrollY]
			}

			statusColor := colGreen
			switch strings.ToLower(n.status) {
			case "not_ready", "rebooting", "deleting", "creating", "starting", "registering", "upgrading":
				statusColor = colYellow
			case "error", "unknown", "locked", "creation_error":
				statusColor = colRed
			}
			statusStr := lipgloss.NewStyle().Foreground(statusColor).Render(padRight(n.status, statusW))
			rowStr := padRight(n.name, nameW) + statusStr + sb

			if m.k8sBrowserFocus == 1 && i == m.k8sBrowserNodeCursor {
				rows = append(rows, lipgloss.NewStyle().
					Background(colBg3).Foreground(colFg).Bold(true).
					Width(rowW).Render("▌ "+rowStr))
			} else {
				rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+rowStr))
			}
		}
	}

	poolTitle := "NODES"
	if len(m.k8sBrowserNodePools) > 0 && m.k8sBrowserPoolCursor < len(m.k8sBrowserNodePools) {
		pool := m.k8sBrowserNodePools[m.k8sBrowserPoolCursor]
		poolTitle = fmt.Sprintf("NODES  %s", pool.name)
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(poolTitle, paneW, paneH, borderColor, listContent)
}

// renderK8sReplaceConfirm shows a centered confirmation dialog for node replacement.
func (m rootModel) renderK8sReplaceConfirm(_ string) string {
	dialogW := min(m.width-8, 60)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	heading := bg.Foreground(colPurple).Bold(true).Width(innerW).Render("Replace Node?")
	empty := bg.Width(innerW).Render("")
	nodeLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Node:  ") +
			bg.Foreground(colFg).Render(m.k8sReplaceNodeName),
	)
	warn := bg.Foreground(colYellow).Width(innerW).Render("The node will be drained and replaced with a new one.")

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))
	actions := bg.Width(innerW).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Background(colPurple).Foreground(colBg).Bold(true).Padding(0, 2).Render("Y  Confirm"),
			lipgloss.NewStyle().Background(colBg3).Foreground(colFg).Padding(0, 2).Render("Esc  Cancel"),
		),
	)

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, empty, nodeLine, empty, warn, empty, divider, empty, actions,
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

// renderK8sRebootConfirm shows a centered confirmation dialog over the K8s browser.
func (m rootModel) renderK8sRebootConfirm(base string) string {
	dialogW := min(m.width-8, 60)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	heading := bg.Foreground(colRed).Bold(true).Width(innerW).Render("Reboot Node?")
	empty := bg.Width(innerW).Render("")
	nodeLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Node:  ") +
			bg.Foreground(colFg).Render(m.k8sRebootNodeName),
	)
	warn := bg.Foreground(colYellow).Width(innerW).Render("The node will be drained and restarted.")

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))
	actions := bg.Width(innerW).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Background(colRed).Foreground(colFg).Bold(true).Padding(0, 2).Render("Y  Confirm"),
			lipgloss.NewStyle().Background(colBg3).Foreground(colFg).Padding(0, 2).Render("Esc  Cancel"),
		),
	)

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, empty, nodeLine, empty, warn, empty, divider, empty, actions,
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
