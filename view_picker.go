package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Profile picker view
// ─────────────────────────────────────────────

func (m rootModel) drawProfilePicker() string {
	if m.loading {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Connecting to Scaleway...")
	}

	// ── Logo ──
	logoStr := lipgloss.NewStyle().Foreground(colRed).Render(strings.TrimPrefix(logo, "\n"))

	// ── Profile list ──
	const listW = 44
	title := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render("SELECT PROFILE")
	divider := lipgloss.NewStyle().Foreground(colBg3).Render(strings.Repeat("─", listW))

	var rows []string
	for i, name := range m.profileNames {
		region := "?"
		if prof, err := m.scwCfg.GetProfile(name); err == nil && prof.DefaultRegion != nil {
			region = string(*prof.DefaultRegion)
		}
		nameCol := lipgloss.NewStyle().Width(26).Render(name)
		regionCol := lipgloss.NewStyle().Width(10).Render(region)
		line := " " + nameCol + regionCol
		if i == m.profileCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBlue).Foreground(colBg).Bold(true).
				Width(listW).Render(line))
		} else {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colFg).Width(listW).Render(line))
		}
	}

	// ── Action buttons — all same width, always bordered, active = filled ──
	const btnW = 14
	type actionDef struct {
		label string
		color lipgloss.Color
	}
	actions := []actionDef{
		{"CONNECT", colGreen},
		{"QUIT", colRed},
	}
	var btns []string
	for i, a := range actions {
		label := lipgloss.NewStyle().Width(btnW).Align(lipgloss.Center).Render(a.label)
		if i == m.pickerAction {
			btns = append(btns, lipgloss.NewStyle().
				Background(a.color).Foreground(colBg).Bold(true).
				Border(lipgloss.RoundedBorder()).BorderForeground(a.color).
				Padding(0, 1).Render(label))
		} else {
			btns = append(btns, lipgloss.NewStyle().
				Foreground(a.color).
				Border(lipgloss.RoundedBorder()).BorderForeground(colBg3).
				Padding(0, 1).Render(label))
		}
	}
	buttonRow := lipgloss.JoinHorizontal(lipgloss.Top, btns[0], "  ", btns[1])

	hint := lipgloss.NewStyle().Foreground(colComment).Faint(true).
		Render("↑↓ profile · ←→ action · Enter confirm")

	errLine := ""
	if m.err != nil {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colRed).Render("✗ "+m.err.Error())
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoStr,
		"",
		title,
		divider,
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		"",
		buttonRow,
		"",
		hint,
		errLine,
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
