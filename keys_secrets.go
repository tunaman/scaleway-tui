package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Secrets Manager (dashboard + secrets browser)
// ─────────────────────────────────────────────

// handleSecretFilterKey handles keys while the dashboard secrets filter is active.
func (m rootModel) handleSecretFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.secretFiltering = false
		m.secretFilter = ""
		m.secretCursor = 0
		m.secretScrollY = 0
	case "enter":
		m.secretFiltering = false
		m.secretCursor = 0
		m.secretScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.secretFilter)) > 0 {
			runes := []rune(m.secretFilter)
			m.secretFilter = string(runes[:len(runes)-1])
		} else {
			m.secretFiltering = false
		}
		m.secretCursor = 0
		m.secretScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.secretFilter += msg.Text
			m.secretCursor = 0
			m.secretScrollY = 0
		}
	}
	return m, nil
}

// handleSecBrowserFilterKey handles keys while the secrets browser filter is active.
func (m rootModel) handleSecBrowserFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.secBrowserFiltering = false
		m.secBrowserFilter = ""
		m.secBrowserCursor = 0
		m.secBrowserScrollY = 0
	case "enter":
		m.secBrowserFiltering = false
		m.secBrowserCursor = 0
		m.secBrowserScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.secBrowserFilter)) > 0 {
			runes := []rune(m.secBrowserFilter)
			m.secBrowserFilter = string(runes[:len(runes)-1])
		} else {
			m.secBrowserFiltering = false
		}
		m.secBrowserCursor = 0
		m.secBrowserScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.secBrowserFilter += msg.Text
			m.secBrowserCursor = 0
			m.secBrowserScrollY = 0
		}
	}
	return m, nil
}

// handleSecShowContentKey handles the secret content overlay (C copies,
// any other key closes).
func (m rootModel) handleSecShowContentKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "c", "C":
		return m.copyToClipboard(m.secContent)
	default:
		m.secShowContent = false
		m.secContent = ""
	}
	return m, nil
}

// handleSecConfirmDeleteKey handles the secret delete confirmation.
func (m rootModel) handleSecConfirmDeleteKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		id := m.secConfirmDeleteID
		m.secConfirmDelete = false
		m.secConfirmDeleteID = ""
		m.secConfirmDeleteName = ""
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.deleteSecret(id))
	default:
		m.secConfirmDelete = false
		m.secConfirmDeleteID = ""
		m.secConfirmDeleteName = ""
	}
	return m, nil
}

// secretsStartFilterKey begins inline filtering of the dashboard secrets list.
func (m rootModel) secretsStartFilterKey() (rootModel, tea.Cmd) {
	m.secretFiltering = true
	m.secretFilter = ""
	m.secretCursor = 0
	m.secretScrollY = 0
	return m, nil
}

// secretsBrowserStartFilterKey begins inline filtering of the version list.
func (m rootModel) secretsBrowserStartFilterKey() (rootModel, tea.Cmd) {
	m.secBrowserFiltering = true
	m.secBrowserFilter = ""
	m.secBrowserCursor = 0
	m.secBrowserScrollY = 0
	return m, nil
}

// secretsListUp / secretsListDown move the dashboard secrets cursor.
func (m rootModel) secretsListUp() (rootModel, tea.Cmd) {
	fs := m.filteredSecrets()
	if len(fs) > 0 && m.secretCursor > 0 {
		m.secretCursor--
		if m.secretCursor < m.secretScrollY {
			m.secretScrollY = m.secretCursor
		}
	}
	return m, nil
}

func (m rootModel) secretsListDown() (rootModel, tea.Cmd) {
	fs := m.filteredSecrets()
	if len(fs) > 0 && m.secretCursor < len(fs)-1 {
		m.secretCursor++
	}
	return m, nil
}

// secretsEnterKey opens the selected secret (fetches its versions).
func (m rootModel) secretsEnterKey() (rootModel, tea.Cmd) {
	fs := m.filteredSecrets()
	if len(fs) == 0 {
		return m, nil
	}
	s := fs[m.secretCursor]
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchSecretVersions(s))
}

// secretsBrowserUp / secretsBrowserDown move the version cursor.
func (m rootModel) secretsBrowserUp() (rootModel, tea.Cmd) {
	visible := m.filteredSecretVersions()
	if len(visible) > 0 && m.secBrowserCursor > 0 {
		m.secBrowserCursor--
		if m.secBrowserCursor < m.secBrowserScrollY {
			m.secBrowserScrollY = m.secBrowserCursor
		}
	}
	return m, nil
}

func (m rootModel) secretsBrowserDown() (rootModel, tea.Cmd) {
	visible := m.filteredSecretVersions()
	if len(visible) > 0 && m.secBrowserCursor < len(visible)-1 {
		m.secBrowserCursor++
	}
	return m, nil
}

// secretsBrowserEnterKey shows the content of the selected version.
func (m rootModel) secretsBrowserEnterKey() (rootModel, tea.Cmd) {
	visible := m.filteredSecretVersions()
	if len(visible) > 0 && m.secBrowserCursor < len(visible) {
		rev := visible[m.secBrowserCursor].revision
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.accessSecretVersion(m.secBrowserSecret.id, rev))
	}
	return m, nil
}

// secretsEsc leaves the secrets browser for the dashboard.
func (m rootModel) secretsEsc() (rootModel, tea.Cmd) {
	m.state = stateDashboard
	m.activeService = serviceSecrets
	m.secBrowserFilter = ""
	m.secBrowserFiltering = false
	m.secShowContent = false
	m.secContent = ""
	return m, nil
}

// secretsCreateInput / secretsNewVersionKey / secretsUpdateDescInput open
// the matching input dialog.
func (m rootModel) secretsCreateInput() (rootModel, tea.Cmd) {
	m.input.active = true
	m.input.mode = inputModeSecret
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	return m, nil
}

func (m rootModel) secretsNewVersionKey() (rootModel, tea.Cmd) {
	m.input.active = true
	m.input.mode = inputModeSecretNewVersion
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	return m, nil
}

func (m rootModel) secretsUpdateDescInput() (rootModel, tea.Cmd) {
	visible := m.filteredSecretVersions()
	if len(visible) > 0 && m.secBrowserCursor < len(visible) {
		m.input.active = true
		m.input.mode = inputModeSecretUpdateDesc
		m.input.value = ""
		m.input.cursor = 0
		m.input.errStr = ""
	}
	return m, nil
}

// secretsDeleteKey opens the delete confirmation for the selected secret.
func (m rootModel) secretsDeleteKey() (rootModel, tea.Cmd) {
	visible := m.filteredSecrets()
	if len(visible) > 0 && m.secretCursor < len(visible) {
		s := visible[m.secretCursor]
		m.secConfirmDeleteID = s.id
		m.secConfirmDeleteName = s.name
		m.secConfirmDelete = true
	}
	return m, nil
}
