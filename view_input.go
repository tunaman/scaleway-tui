package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderInputOverlay draws a centred input dialog on top of the base view.
// It handles all inputMode variants across every screen.
func (m rootModel) renderInputOverlay(base string) string {
	dialogW := 58

	var title, subtitle, placeholder string
	switch m.input.mode {
	case inputModeBucket:
		title = "  NEW BUCKET"
		subtitle = "  Enter a name for the new bucket."
		placeholder = "my-bucket-name"
	case inputModeFolder:
		path := m.browserBucket
		if m.browserPrefix != "" {
			path += "/" + strings.TrimSuffix(m.browserPrefix, "/")
		}
		title = "  NEW FOLDER"
		subtitle = "  Creating in: " + path
		placeholder = "folder-name"
	case inputModeUpload:
		title = "  UPLOAD FILE"
		subtitle = "  Enter the full local path of the file to upload."
		placeholder = "/path/to/local/file.csv"
	case inputModeSecretNewVersion:
		title = "  NEW SECRET VERSION"
		subtitle = "  Enter the secret value for " + m.secBrowserSecret.name + "."
		placeholder = "secret-value"
	case inputModeSecretUpdateDesc:
		title = "  UPDATE VERSION DESCRIPTION"
		subtitle = "  Enter a new description (leave blank to clear)."
		placeholder = "description..."
	}

	titleRendered := lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(title)
	subtitleRendered := lipgloss.NewStyle().Foreground(colComment).Render(subtitle)

	// Text field: split value at cursor position, insert cursor glyph between.
	fieldW := dialogW - 6 // inner width after border + padding
	var fieldContent string
	if m.input.value == "" {
		// Show placeholder with cursor at start.
		ph := lipgloss.NewStyle().Foreground(colBg3).Render(placeholder)
		cur := lipgloss.NewStyle().Foreground(colGreen).Render("▌")
		fieldContent = cur + ph
	} else {
		runes := []rune(m.input.value)
		cur := m.input.cursor
		if cur > len(runes) {
			cur = len(runes)
		}
		before := string(runes[:cur])
		after := string(runes[cur:])

		// Scroll the visible window so the cursor is always visible.
		// fieldW accounts for 1 cell taken by the cursor glyph.
		visW := fieldW - 1
		beforeRunes := []rune(before)
		if len(beforeRunes) > visW {
			// Trim the leftmost characters so cursor stays in view.
			before = string(beforeRunes[len(beforeRunes)-visW:])
		}

		curGlyph := lipgloss.NewStyle().Foreground(colGreen).Render("▌")
		afterStyled := lipgloss.NewStyle().Foreground(colFg).Render(after)
		fieldContent = lipgloss.NewStyle().Foreground(colFg).Render(before) + curGlyph + afterStyled
	}

	field := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Width(fieldW).
		Render(fieldContent)

	errLine := ""
	if m.input.errStr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colRed).Render("  ✗ "+m.input.errStr)
	}

	hint := lipgloss.NewStyle().Foreground(colComment).Faint(true).
		Render("  Enter · Esc · ←→ · Home/End · Ctrl+W · Ctrl+U/K")

	body := lipgloss.JoinVertical(lipgloss.Left,
		titleRendered,
		subtitleRendered,
		"",
		"  "+field,
		errLine,
		"",
		hint,
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
