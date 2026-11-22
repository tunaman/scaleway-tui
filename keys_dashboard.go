package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — dashboard (nav + per-service dispatch)
// ─────────────────────────────────────────────

// dashboardEsc leaves the dashboard and returns to the profile picker.
func (m rootModel) dashboardEsc() (rootModel, tea.Cmd) {
	m.showDropdown = false
	m.state = stateProfilePicker
	m.err = nil
	return m, nil
}

// dashboardTabKey toggles focus between the nav and content panes.
func (m rootModel) dashboardTabKey() (rootModel, tea.Cmd) {
	m.focus = focusNav + (m.focus-focusNav+1)%2
	m.showDropdown = false
	return m, nil
}

// dashboardNavUp / dashboardNavDown move the service cursor; selecting
// billing refreshes its overview.
func (m rootModel) dashboardNavUp() (rootModel, tea.Cmd) {
	m.activeService = (m.activeService - 1 + serviceCount) % serviceCount
	if m.activeService == serviceBilling && !m.loading {
		m.loading = true
		m.billingExportMsg = ""
		return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
	}
	return m, nil
}

func (m rootModel) dashboardNavDown() (rootModel, tea.Cmd) {
	m.activeService = (m.activeService + 1) % serviceCount
	if m.activeService == serviceBilling && !m.loading {
		m.loading = true
		m.billingExportMsg = ""
		return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
	}
	return m, nil
}

// dropdownUp / dropdownDown move the project dropdown cursor.
func (m rootModel) dropdownUp() (rootModel, tea.Cmd) {
	m.dropdownIndex = (m.dropdownIndex - 1 + len(m.projects)) % len(m.projects)
	return m, nil
}

func (m rootModel) dropdownDown() (rootModel, tea.Cmd) {
	m.dropdownIndex = (m.dropdownIndex + 1) % len(m.projects)
	return m, nil
}

// dashboardContentUp / dashboardContentDown / dashboardContentEnter route
// cursor movement and Enter to the active service's list.
func (m rootModel) dashboardContentUp() (rootModel, tea.Cmd) {
	switch m.activeService {
	case serviceInstance:
		return m.instanceListUp()
	case serviceObjectStorage:
		return m.bucketListUp()
	case serviceK8s:
		return m.clusterListUp()
	case serviceRegistry:
		return m.registryNamespaceListUp()
	case serviceSecrets:
		return m.secretsListUp()
	case serviceVolumes:
		return m.volumesListUp()
	}
	return m, nil
}

func (m rootModel) dashboardContentDown() (rootModel, tea.Cmd) {
	switch m.activeService {
	case serviceInstance:
		return m.instanceListDown()
	case serviceObjectStorage:
		return m.bucketListDown()
	case serviceK8s:
		return m.clusterListDown()
	case serviceRegistry:
		return m.registryNamespaceListDown()
	case serviceSecrets:
		return m.secretsListDown()
	case serviceVolumes:
		return m.volumesListDown()
	}
	return m, nil
}

func (m rootModel) dashboardContentEnter() (rootModel, tea.Cmd) {
	switch m.activeService {
	case serviceObjectStorage:
		return m.bucketEnterKey()
	case serviceK8s:
		return m.clusterEnterKey()
	case serviceRegistry:
		return m.registryNamespaceEnterKey()
	case serviceBilling:
		return m.billingEnterKey()
	case serviceSecrets:
		return m.secretsEnterKey()
	case serviceIAM:
		return m.iamEnterKey()
	}
	return m, nil
}
