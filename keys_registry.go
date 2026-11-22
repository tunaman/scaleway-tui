package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Container Registry (dashboard + registry browser)
// ─────────────────────────────────────────────

// handleRegistryFilterKey handles keys while the namespace list filter is active.
func (m rootModel) handleRegistryFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.registryFiltering = false
		m.registryFilter = ""
		m.registryCursor = 0
		m.registryScrollY = 0
	case "enter":
		m.registryFiltering = false
		m.registryCursor = 0
		m.registryScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.registryFilter)) > 0 {
			runes := []rune(m.registryFilter)
			m.registryFilter = string(runes[:len(runes)-1])
		} else {
			m.registryFiltering = false
		}
		m.registryCursor = 0
		m.registryScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.registryFilter += msg.Text
			m.registryCursor = 0
			m.registryScrollY = 0
		}
	}
	return m, nil
}

// handleRegBrowserFilterKey handles keys while the images list filter is active.
func (m rootModel) handleRegBrowserFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.regBrowserFiltering = false
		m.regBrowserFilter = ""
		m.regBrowserCursor = 0
		m.regBrowserScrollY = 0
	case "enter":
		m.regBrowserFiltering = false
		m.regBrowserCursor = 0
		m.regBrowserScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.regBrowserFilter)) > 0 {
			runes := []rune(m.regBrowserFilter)
			m.regBrowserFilter = string(runes[:len(runes)-1])
		} else {
			m.regBrowserFiltering = false
		}
		m.regBrowserCursor = 0
		m.regBrowserScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.regBrowserFilter += msg.Text
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		}
	}
	return m, nil
}

// handleRegTagFilterKey handles keys while the tags list filter is active.
func (m rootModel) handleRegTagFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.regTagFiltering = false
		m.regTagFilter = ""
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
	case "enter":
		m.regTagFiltering = false
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.regTagFilter)) > 0 {
			runes := []rune(m.regTagFilter)
			m.regTagFilter = string(runes[:len(runes)-1])
		} else {
			m.regTagFiltering = false
		}
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.regTagFilter += msg.Text
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
		}
	}
	return m, nil
}

// handleRegTagActionOverlayKey handles the pull-instructions overlay
// (C copies the pull command, any other key closes).
func (m rootModel) handleRegTagActionOverlayKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "c", "C":
		if cmd := m.registryPullCommand(); cmd != "" {
			return m.copyToClipboard(cmd)
		}
	default:
		m.regTagActionOverlay = false
	}
	return m, nil
}

// handleRegConfirmDeleteKey handles the tag delete confirmation.
func (m rootModel) handleRegConfirmDeleteKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		imageID := m.regConfirmDeleteImgID
		tags := m.regConfirmTagsToDelete
		m.regConfirmDeleteTags = false
		m.regConfirmTagsToDelete = nil
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.deleteRegistryTags(imageID, tags))
	default:
		m.regConfirmDeleteTags = false
		m.regConfirmTagsToDelete = nil
	}
	return m, nil
}

// registryStartFilterKey begins inline filtering of the namespace list.
func (m rootModel) registryStartFilterKey() (rootModel, tea.Cmd) {
	m.registryFiltering = true
	m.registryFilter = ""
	m.registryCursor = 0
	m.registryScrollY = 0
	return m, nil
}

// regBrowserStartFilterKey / regTagStartFilterKey begin inline filtering in
// the registry browser's images / tags pane.
func (m rootModel) regBrowserStartFilterKey() (rootModel, tea.Cmd) {
	m.regBrowserFiltering = true
	m.regBrowserFilter = ""
	m.regBrowserCursor = 0
	m.regBrowserScrollY = 0
	return m, nil
}

func (m rootModel) regTagStartFilterKey() (rootModel, tea.Cmd) {
	m.regTagFiltering = true
	m.regTagFilter = ""
	m.regBrowserTagCursor = 0
	m.regBrowserTagScrollY = 0
	return m, nil
}

// registryNamespaceListUp / registryNamespaceListDown move the namespace
// cursor on the dashboard.
func (m rootModel) registryNamespaceListUp() (rootModel, tea.Cmd) {
	if len(m.registryNamespaces) > 0 && m.registryCursor > 0 {
		m.registryCursor--
		if m.registryCursor < m.registryScrollY {
			m.registryScrollY = m.registryCursor
		}
	}
	return m, nil
}

func (m rootModel) registryNamespaceListDown() (rootModel, tea.Cmd) {
	if len(m.registryNamespaces) > 0 && m.registryCursor < len(m.registryNamespaces)-1 {
		m.registryCursor++
	}
	return m, nil
}

// registryNamespaceEnterKey opens the selected namespace (fetches its images).
func (m rootModel) registryNamespaceEnterKey() (rootModel, tea.Cmd) {
	fn := m.filteredRegistryNamespaces()
	if len(fn) == 0 {
		return m, nil
	}
	ns := fn[m.registryCursor]
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(ns))
}

// registryTabKey switches between the images and tags panes.
func (m rootModel) registryTabKey() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if m.regBrowserFocus == 0 {
		if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
			if !m.regTagsLoading {
				m.regBrowserFocus = 1
				m.regBrowserTagCursor = 0
				m.regBrowserTagScrollY = 0
			}
		}
	} else {
		m.regBrowserFocus = 0
		m.regTagSelected = nil
	}
	return m, nil
}

// registryBrowserEnterKey shows the pull command for the selected tag.
func (m rootModel) registryBrowserEnterKey() (rootModel, tea.Cmd) {
	visible := m.filteredRegistryImages()
	if len(visible) > 0 && m.regBrowserCursor < len(visible) {
		img := visible[m.regBrowserCursor]
		if len(img.tags) > 0 && m.regBrowserTagCursor < len(img.tags) {
			m.regTagActionOverlay = true
		}
	}
	return m, nil
}

// registryDeleteKey opens the tag delete confirmation for the selected tag(s).
func (m rootModel) registryDeleteKey() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
		img := imgs[m.regBrowserCursor]
		filteredTags := m.filteredRegistryTags(img)
		var toDelete []registryTag
		if len(m.regTagSelected) > 0 {
			for _, t := range filteredTags {
				if m.regTagSelected[t.name] {
					toDelete = append(toDelete, t)
				}
			}
		} else if len(filteredTags) > 0 && m.regBrowserTagCursor < len(filteredTags) {
			toDelete = []registryTag{filteredTags[m.regBrowserTagCursor]}
		}
		if len(toDelete) > 0 {
			m.regConfirmTagsToDelete = toDelete
			m.regConfirmDeleteImgID = img.id
			m.regConfirmDeleteImgName = img.name
			m.regConfirmDeleteTags = true
		}
	}
	// Image-level deletion removed — only tag deletion is supported.
	return m, nil
}

// registryTagDeleteConfirmYes deletes the confirmed tags.
func (m rootModel) registryTagDeleteConfirmYes() (rootModel, tea.Cmd) {
	imageID := m.regConfirmDeleteImgID
	tags := m.regConfirmTagsToDelete
	m.regConfirmDeleteTags = false
	m.regConfirmTagsToDelete = nil
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.deleteRegistryTags(imageID, tags))
}

// registryTagSelectToggleKey toggles selection of the tag under the cursor.
func (m rootModel) registryTagSelectToggleKey() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
		filteredTags := m.filteredRegistryTags(imgs[m.regBrowserCursor])
		if len(filteredTags) > 0 && m.regBrowserTagCursor < len(filteredTags) {
			tag := filteredTags[m.regBrowserTagCursor]
			if m.regTagSelected == nil {
				m.regTagSelected = make(map[string]bool)
			}
			m.regTagSelected[tag.name] = !m.regTagSelected[tag.name]
			if !m.regTagSelected[tag.name] {
				delete(m.regTagSelected, tag.name)
			}
			if m.regBrowserTagCursor < len(filteredTags)-1 {
				m.regBrowserTagCursor++
			}
		}
	}
	return m, nil
}

// registryTagSelectAllKey toggles select-all for the visible tags.
func (m rootModel) registryTagSelectAllKey() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
		filteredTags := m.filteredRegistryTags(imgs[m.regBrowserCursor])
		if len(filteredTags) > 0 {
			if m.regTagSelected == nil {
				m.regTagSelected = make(map[string]bool)
			}
			allSelected := len(m.regTagSelected) == len(filteredTags)
			for _, t := range filteredTags {
				if allSelected {
					delete(m.regTagSelected, t.name)
				} else {
					m.regTagSelected[t.name] = true
				}
			}
		}
	}
	return m, nil
}

// registryUp / registryDown move the cursor in the active pane; moving
// between images refetches that image's tags.
func (m rootModel) registryUp() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if m.regBrowserFocus == 0 && len(imgs) > 0 {
		if m.regBrowserCursor > 0 {
			m.regBrowserCursor--
			if m.regBrowserCursor < m.regBrowserScrollY {
				m.regBrowserScrollY = m.regBrowserCursor
			}
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			m.regTagsLoading = true
			return m, m.fetchRegistryTags(imgs[m.regBrowserCursor])
		}
	} else if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
		if m.regBrowserTagCursor > 0 {
			m.regBrowserTagCursor--
			if m.regBrowserTagCursor < m.regBrowserTagScrollY {
				m.regBrowserTagScrollY = m.regBrowserTagCursor
			}
		}
	}
	return m, nil
}

func (m rootModel) registryDown() (rootModel, tea.Cmd) {
	imgs := m.filteredRegistryImages()
	if m.regBrowserFocus == 0 && len(imgs) > 0 {
		if m.regBrowserCursor < len(imgs)-1 {
			m.regBrowserCursor++
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			m.regTagsLoading = true
			return m, m.fetchRegistryTags(imgs[m.regBrowserCursor])
		}
	} else if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
		tags := imgs[m.regBrowserCursor].tags
		if m.regBrowserTagCursor < len(tags)-1 {
			m.regBrowserTagCursor++
		}
	}
	return m, nil
}

// registryEsc leaves the registry browser for the dashboard.
func (m rootModel) registryEsc() (rootModel, tea.Cmd) {
	m.state = stateDashboard
	m.activeService = serviceRegistry
	m.regBrowserFilter = ""
	m.regBrowserFiltering = false
	m.regBrowserFocus = 0
	m.regBrowserTagCursor = 0
	m.regBrowserTagScrollY = 0
	m.regTagActionOverlay = false
	m.regConfirmDeleteTags = false
	m.regTagSelected = nil
	m.regTagFilter = ""
	m.regTagFiltering = false
	return m, nil
}
