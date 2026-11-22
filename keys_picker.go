package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — profile picker
// ─────────────────────────────────────────────

// pickerUp moves the profile cursor up.
func (m rootModel) pickerUp() (rootModel, tea.Cmd) {
	m.profileCursor = (m.profileCursor - 1 + len(m.profileNames)) % len(m.profileNames)
	return m, nil
}

// pickerDown moves the profile cursor down.
func (m rootModel) pickerDown() (rootModel, tea.Cmd) {
	m.profileCursor = (m.profileCursor + 1) % len(m.profileNames)
	return m, nil
}

// pickerPrevAction / pickerNextAction switch the picker action row.
func (m rootModel) pickerPrevAction() (rootModel, tea.Cmd) {
	m.pickerAction = (m.pickerAction - 1 + pickerActionCount) % pickerActionCount
	return m, nil
}

func (m rootModel) pickerNextAction() (rootModel, tea.Cmd) {
	m.pickerAction = (m.pickerAction + 1) % pickerActionCount
	return m, nil
}

// pickerEnter connects to the selected profile (or quits).
func (m rootModel) pickerEnter() (rootModel, tea.Cmd) {
	if len(m.profileNames) == 0 {
		return m, nil
	}
	switch m.pickerAction {
	case pickerActionConnect:
		chosen := m.profileNames[m.profileCursor]
		m.loading = true
		m.err = nil
		return m, tea.Batch(m.spin.Tick, m.activateProfile(chosen))
	case pickerActionQuit:
		return m, tea.Quit
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Profile activation (Cmd)
// ─────────────────────────────────────────────

func (m rootModel) activateProfile(name string) tea.Cmd {
	return func() tea.Msg {
		scwClient, mc, region, err := buildClients(m.scwCfg, name)
		if err != nil {
			return errMsg{err}
		}
		saveTUIConfig(tuiConfig{ActiveProfile: name})

		// Read default_project_id and default_organization_id directly from
		// the profile — no API call needed.
		projectID := ""
		orgID := ""
		if prof, err := m.scwCfg.GetProfile(name); err == nil {
			if prof.DefaultProjectID != nil {
				projectID = *prof.DefaultProjectID
			}
			if prof.DefaultOrganizationID != nil {
				orgID = *prof.DefaultOrganizationID
			}
		}

		return clientsReadyMsg{
			scwClient:             scwClient,
			minioClient:           mc,
			profileName:           name,
			region:                region,
			defaultProjectID:      projectID,
			defaultOrganizationID: orgID,
		}
	}
}
