package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Instances (dashboard service)
// ─────────────────────────────────────────────

// handleInstFilterKey handles keys while the server list filter is active.
func (m rootModel) handleInstFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.instFiltering = false
		m.instFilter = ""
		m.instCursor = 0
		m.instScrollY = 0
	case "enter":
		m.instFiltering = false
		m.instCursor = 0
		m.instScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.instFilter)) > 0 {
			runes := []rune(m.instFilter)
			m.instFilter = string(runes[:len(runes)-1])
		} else {
			// Empty filter + backspace → exit filter mode.
			m.instFiltering = false
		}
		m.instCursor = 0
		m.instScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.instFilter += msg.Text
			m.instCursor = 0
			m.instScrollY = 0
		}
	}
	return m, nil
}

// instanceStartFilterKey begins inline filtering of the server list.
func (m rootModel) instanceStartFilterKey() (rootModel, tea.Cmd) {
	m.instFiltering = true
	m.instFilter = ""
	m.instCursor = 0
	m.instScrollY = 0
	return m, nil
}

// instancePowerOnKey / instancePowerOffKey / instanceRebootKey open the
// matching confirmation dialog for the selected server.
func (m rootModel) instancePowerOnKey() (rootModel, tea.Cmd) {
	if m.state == stateDashboard && m.activeService == serviceInstance && !m.loading && m.instConfirmAction == instActionNone {
		if s := m.selectedServer(); s != nil {
			m.instConfirmAction = instActionPowerOn
			m.instConfirmServerID = s.id
			m.instConfirmServerName = s.name
			m.instConfirmServerZone = s.zone
		}
	}
	return m, nil
}

func (m rootModel) instancePowerOffKey() (rootModel, tea.Cmd) {
	if m.state == stateDashboard && m.activeService == serviceInstance && !m.loading && m.instConfirmAction == instActionNone {
		if s := m.selectedServer(); s != nil {
			m.instConfirmAction = instActionPowerOff
			m.instConfirmServerID = s.id
			m.instConfirmServerName = s.name
			m.instConfirmServerZone = s.zone
		}
	}
	return m, nil
}

func (m rootModel) instanceRebootKey() (rootModel, tea.Cmd) {
	if s := m.selectedServer(); s != nil {
		m.instConfirmAction = instActionReboot
		m.instConfirmServerID = s.id
		m.instConfirmServerName = s.name
		m.instConfirmServerZone = s.zone
	}
	return m, nil
}

// instancePowerConfirmYes / instancePowerConfirmNo handle the power-action
// confirmation dialog.
func (m rootModel) instancePowerConfirmYes() (rootModel, tea.Cmd) {
	action := instanceActionAPI[m.instConfirmAction]
	id := m.instConfirmServerID
	zone := m.instConfirmServerZone
	m.instConfirmAction = instActionNone
	m.instConfirmServerID = ""
	m.instConfirmServerName = ""
	m.instConfirmServerZone = ""
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.powerServer(id, zone, action))
}

func (m rootModel) instancePowerConfirmNo() (rootModel, tea.Cmd) {
	m.instConfirmAction = instActionNone
	m.instConfirmServerID = ""
	m.instConfirmServerName = ""
	m.instConfirmServerZone = ""
	return m, nil
}

// instanceCopyIPKey copies the selected server's IP (public, falling back to private).
func (m rootModel) instanceCopyIPKey() (rootModel, tea.Cmd) {
	if s := m.selectedServer(); s != nil {
		ip := s.publicIP
		if ip == "" {
			ip = s.privateIP
		}
		if ip != "" {
			return m.copyToClipboard(ip)
		}
	}
	return m, nil
}

// instanceListUp / instanceListDown move the server cursor and kick off
// lazy IPAM private-IP resolution for the newly selected server.
func (m rootModel) instanceListUp() (rootModel, tea.Cmd) {
	fs := m.filteredServers()
	if len(fs) > 0 && m.instCursor > 0 {
		m.instCursor--
		if m.instCursor < m.instScrollY {
			m.instScrollY = m.instCursor
		}
		return m, m.selectedServerPrivateIPCmd()
	}
	return m, nil
}

func (m rootModel) instanceListDown() (rootModel, tea.Cmd) {
	fs := m.filteredServers()
	if len(fs) > 0 && m.instCursor < len(fs)-1 {
		m.instCursor++
		return m, m.selectedServerPrivateIPCmd()
	}
	return m, nil
}
