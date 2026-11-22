package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — IAM browser
// ─────────────────────────────────────────────

// resetIAMFilterCursors zeroes the browser cursor/scroll for the shared
// filter handler (handleFilterKey).
func (m *rootModel) resetIAMFilterCursors() {
	m.iamCursor = 0
	m.iamScrollY = 0
}

// iamStartFilterKey begins inline filtering of the active IAM tab.
func (m rootModel) iamStartFilterKey() (rootModel, tea.Cmd) {
	m.iamFiltering = true
	m.iamFilter = ""
	m.iamCursor = 0
	m.iamScrollY = 0
	return m, nil
}

// iamTabNextKey / iamTabPrevKey switch IAM tab, resetting cursor and filter.
func (m rootModel) iamTabNextKey() (rootModel, tea.Cmd) {
	m.iamTab = (m.iamTab + 1) % iamTabCount
	m.iamCursor = 0
	m.iamScrollY = 0
	m.iamFilter = ""
	m.iamFiltering = false
	return m, nil
}

func (m rootModel) iamTabPrevKey() (rootModel, tea.Cmd) {
	m.iamTab = (m.iamTab - 1 + iamTabCount) % iamTabCount
	m.iamCursor = 0
	m.iamScrollY = 0
	m.iamFilter = ""
	m.iamFiltering = false
	return m, nil
}

// iamCopyAccessKey copies the selected API key's access key.
func (m rootModel) iamCopyAccessKey() (rootModel, tea.Cmd) {
	keys := m.filteredIAMAPIKeys()
	if len(keys) > 0 && m.iamCursor < len(keys) {
		return m.copyToClipboard(keys[m.iamCursor].accessKey)
	}
	return m, nil
}

// iamUp / iamDown move the cursor within the active tab.
func (m rootModel) iamUp() (rootModel, tea.Cmd) {
	if m.iamVisibleCount() > 0 && m.iamCursor > 0 {
		m.iamCursor--
		if m.iamCursor < m.iamScrollY {
			m.iamScrollY = m.iamCursor
		}
	}
	return m, nil
}

func (m rootModel) iamDown() (rootModel, tea.Cmd) {
	if n := m.iamVisibleCount(); n > 0 && m.iamCursor < n-1 {
		m.iamCursor++
	}
	return m, nil
}

// iamEnterKey opens the IAM browser from the dashboard (reusing cached data).
func (m rootModel) iamEnterKey() (rootModel, tea.Cmd) {
	if m.iamLoaded {
		// Data already cached — enter the browser immediately.
		m.iamFilter = ""
		m.iamFiltering = false
		m.state = stateIAMBrowser
		return m, nil
	}
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchIAM())
}

// iamEsc leaves the IAM browser for the dashboard.
func (m rootModel) iamEsc() (rootModel, tea.Cmd) {
	m.state = stateDashboard
	m.activeService = serviceIAM
	m.iamFilter = ""
	m.iamFiltering = false
	return m, nil
}
