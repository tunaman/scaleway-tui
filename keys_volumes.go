package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Volumes & Snapshots (dashboard service)
// ─────────────────────────────────────────────

// resetVolFilterCursors zeroes both pane cursors/scroll for the shared
// filter handler (handleFilterKey).
func (m *rootModel) resetVolFilterCursors() {
	m.volCursor = 0
	m.volScrollY = 0
	m.volSnapCursor = 0
	m.volSnapScrollY = 0
}

// handleVolAttachOverlayKey handles the attach server picker overlay.
func (m rootModel) handleVolAttachOverlayKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	total := len(m.attachServersInZone())
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.volAttachOverlay = false
	case "up", "k":
		if m.volAttachCursor > 0 {
			m.volAttachCursor--
		}
	case "down", "j":
		if m.volAttachCursor < total-1 {
			m.volAttachCursor++
		}
	case "enter":
		servers := m.attachServersInZone()
		if len(servers) > 0 && m.volAttachCursor < len(servers) {
			vol := m.volAttachVolume
			m.volAttachOverlay = false
			// Confirm before the mutating call.
			m.volConfirmAction = volActionAttach
			m.volConfirmID = vol.id
			m.volConfirmName = vol.name
			m.volConfirmServerID = servers[m.volAttachCursor].id
			m.volConfirmServerName = servers[m.volAttachCursor].name
		}
	}
	return m, nil
}

// volumesStartFilterKey begins inline filtering of the volume lists.
func (m rootModel) volumesStartFilterKey() (rootModel, tea.Cmd) {
	m.volFiltering = true
	m.volFilter = ""
	m.volCursor = 0
	m.volScrollY = 0
	m.volSnapCursor = 0
	m.volSnapScrollY = 0
	return m, nil
}

// volumesDetachKey opens the detach confirmation for the selected (attached) volume.
func (m rootModel) volumesDetachKey() (rootModel, tea.Cmd) {
	if v := m.selectedVolume(); v != nil && v.serverID != "" {
		m.volConfirmAction = volActionDetach
		m.volConfirmID = v.id
		m.volConfirmName = v.name
		m.volConfirmServerID = v.serverID
		m.volConfirmServerName = m.volumeServerName(*v)
	}
	return m, nil
}

// volumesAttachKey opens the attach server picker for the selected (unattached) volume.
func (m rootModel) volumesAttachKey() (rootModel, tea.Cmd) {
	if v := m.selectedVolume(); v != nil && v.serverID == "" {
		m.volAttachVolume = *v
		m.volAttachCursor = 0
		m.volAttachOverlay = true
	}
	return m, nil
}

// volumesDeleteKey opens the delete confirmation for the selected item.
func (m rootModel) volumesDeleteKey() (rootModel, tea.Cmd) {
	if m.volFocus == 0 {
		// Only unattached volumes can be deleted.
		if v := m.selectedVolume(); v != nil && v.serverID == "" {
			m.volConfirmAction = volActionDeleteVolume
			m.volConfirmID = v.id
			m.volConfirmName = v.name
		}
	} else if s := m.selectedSnapshot(); s != nil {
		m.volConfirmAction = volActionDeleteSnapshot
		m.volConfirmID = s.id
		m.volConfirmName = s.name
	}
	return m, nil
}

// volumeConfirmYes / volumeConfirmNo handle the volume/snapshot action
// confirmation dialog.
func (m rootModel) volumeConfirmYes() (rootModel, tea.Cmd) {
	action := m.volConfirmAction
	id := m.volConfirmID
	serverID := m.volConfirmServerID
	// Resolve the target item (zone and type ride along in it; the
	// type decides whether the call goes to the Block or Instance API).
	var vol volumeItem
	var snap snapshotItem
	for _, v := range m.volumes {
		if v.id == id {
			vol = v
			break
		}
	}
	if vol.id == "" {
		for _, s := range m.snapshots {
			if s.id == id {
				snap = s
				break
			}
		}
	}
	m.volConfirmAction = volActionNone
	m.volConfirmID = ""
	m.volConfirmName = ""
	m.volConfirmServerID = ""
	m.volConfirmServerName = ""
	m.loading = true
	var cmd tea.Cmd
	switch action {
	case volActionDeleteVolume:
		cmd = m.deleteVolume(vol)
	case volActionDeleteSnapshot:
		cmd = m.deleteSnapshot(snap)
	case volActionAttach:
		cmd = m.attachVolume(vol, serverID)
	case volActionDetach:
		cmd = m.detachVolume(vol)
	}
	return m, tea.Batch(m.spin.Tick, cmd)
}

func (m rootModel) volumeConfirmNo() (rootModel, tea.Cmd) {
	m.volConfirmAction = volActionNone
	m.volConfirmID = ""
	m.volConfirmName = ""
	m.volConfirmServerID = ""
	m.volConfirmServerName = ""
	return m, nil
}

// volumesListUp / volumesListDown move the cursor in the focused pane.
func (m rootModel) volumesListUp() (rootModel, tea.Cmd) {
	if m.volFocus == 0 {
		fv := m.filteredVolumes()
		if len(fv) > 0 && m.volCursor > 0 {
			m.volCursor--
			if m.volCursor < m.volScrollY {
				m.volScrollY = m.volCursor
			}
		}
	} else {
		fs := m.filteredSnapshots()
		if len(fs) > 0 && m.volSnapCursor > 0 {
			m.volSnapCursor--
			if m.volSnapCursor < m.volSnapScrollY {
				m.volSnapScrollY = m.volSnapCursor
			}
		}
	}
	return m, nil
}

func (m rootModel) volumesListDown() (rootModel, tea.Cmd) {
	if m.volFocus == 0 {
		fv := m.filteredVolumes()
		if len(fv) > 0 && m.volCursor < len(fv)-1 {
			m.volCursor++
		}
	} else {
		fs := m.filteredSnapshots()
		if len(fs) > 0 && m.volSnapCursor < len(fs)-1 {
			m.volSnapCursor++
		}
	}
	return m, nil
}
