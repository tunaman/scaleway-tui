package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ─────────────────────────────────────────────
// Key handling
// ─────────────────────────────────────────────

func (m rootModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Input overlay: intercept all keys while active ──
	if m.input.active {
		runes := []rune(m.input.value)

		// Helper: clamp cursor into valid range
		clamp := func(c int) int {
			if c < 0 {
				return 0
			}
			if c > len(runes) {
				return len(runes)
			}
			return c
		}

		switch msg.String() {
		case "esc":
			m.input.active = false
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""

		case "enter":
			val := strings.TrimSpace(m.input.value)
			if val == "" && m.input.mode != inputModeSecretUpdateDesc {
				m.input.errStr = "Name cannot be empty."
				return m, nil
			}
			switch m.input.mode {
			case inputModeBucket:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createBucket(val))
			case inputModeFolder:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createFolder(m.browserBucket, m.browserPrefix, val))
			case inputModeSecret:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createSecret(val))
			case inputModeSecretNewVersion:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createSecretVersion(m.secBrowserSecret.id, val))
			case inputModeSecretUpdateDesc:
				m.input.active = false
				m.loading = true
				visible := m.filteredSecretVersions()
				if len(visible) > 0 && m.secBrowserCursor < len(visible) {
					rev := visible[m.secBrowserCursor].revision
					return m, tea.Batch(m.spin.Tick, m.updateSecretVersionDesc(m.secBrowserSecret.id, rev, val))
				}
				return m, nil
			case inputModeUpload:
				// Extract filename now so the overlay shows it immediately.
				base := val
				if idx := strings.LastIndexAny(val, "/\\"); idx >= 0 {
					base = val[idx+1:]
				}
				m.input.active = false
				m.upload.active = true
				m.upload.filename = base
				m.upload.bytesRead = 0
				m.upload.total = 0
				return m, m.uploadFile(m.browserBucket, m.browserPrefix, val)
			}

		case "backspace", "ctrl+h":
			if m.input.cursor > 0 {
				newRunes := append(runes[:m.input.cursor-1], runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				m.input.cursor = clamp(m.input.cursor - 1)
			}
			m.input.errStr = ""

		case "delete", "ctrl+d":
			if m.input.cursor < len(runes) {
				newRunes := append(runes[:m.input.cursor], runes[m.input.cursor+1:]...)
				m.input.value = string(newRunes)
			}
			m.input.errStr = ""

		case "left", "ctrl+b":
			m.input.cursor = clamp(m.input.cursor - 1)

		case "right", "ctrl+f":
			m.input.cursor = clamp(m.input.cursor + 1)

		case "home", "ctrl+a":
			m.input.cursor = 0

		case "end", "ctrl+e":
			m.input.cursor = len(runes)

		case "ctrl+u":
			// Kill to beginning of line (readline convention).
			m.input.value = string(runes[m.input.cursor:])
			m.input.cursor = 0
			m.input.errStr = ""

		case "ctrl+k":
			// Kill to end of line.
			m.input.value = string(runes[:m.input.cursor])
			m.input.errStr = ""

		case "ctrl+w":
			// Kill word backwards.
			if m.input.cursor > 0 {
				i := m.input.cursor - 1
				for i > 0 && runes[i-1] != '/' && runes[i-1] != ' ' {
					i--
				}
				newRunes := append(runes[:i], runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				m.input.cursor = i
			}
			m.input.errStr = ""

		default:
			// msg.Runes contains the typed/pasted characters.
			if len(msg.Runes) > 0 {
				insert := msg.Runes
				newRunes := make([]rune, 0, len(runes)+len(insert))
				newRunes = append(newRunes, runes[:m.input.cursor]...)
				newRunes = append(newRunes, insert...)
				newRunes = append(newRunes, runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				// Cursor advances by the number of inserted chars.
				// Don't use clamp() here — it closes over the pre-insert runes slice.
				m.input.cursor += len(insert)
				m.input.errStr = ""
			}
		}
		return m, nil
	}
	// ── Filter mode: intercept all printable keys ──
	if m.bucketFiltering {
		switch msg.String() {
		case "esc":
			m.bucketFiltering = false
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "enter":
			// Lock in the first match and exit filter mode.
			m.bucketFiltering = false
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "backspace", "ctrl+h":
			if len([]rune(m.bucketFilter)) > 0 {
				runes := []rune(m.bucketFilter)
				m.bucketFilter = string(runes[:len(runes)-1])
			} else {
				// Empty filter + backspace → exit filter mode.
				m.bucketFiltering = false
			}
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		default:
			// Only accept printable single chars.
			if len(msg.Runes) == 1 {
				m.bucketFilter += string(msg.Runes)
				m.bucketCursor = 0
				m.bucketScrollY = 0
			}
		}
		return m, nil
	}
	// ── Billing project picker overlay ──
	if m.billingProjectOverlay {
		total := len(m.projects) + 1 // 0=all, 1..n=project
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.billingProjectOverlay = false
		case "up", "k":
			if m.billingProjectCursor > 0 {
				m.billingProjectCursor--
			}
		case "down", "j":
			if m.billingProjectCursor < total-1 {
				m.billingProjectCursor++
			}
		case "enter":
			m.billingProjectOverlay = false
			if m.billingProjectCursor != m.billingProjectIdx {
				m.billingProjectIdx = m.billingProjectCursor
				m.loading = true
				m.billingExportMsg = ""
				return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
			}
		}
		return m, nil
	}

	// ── Billing export date picker overlay ──
	if m.billingExportOverlay {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.billingExportOverlay = false
		case "tab":
			m.billingExportField = 1 - m.billingExportField
		case "left", "h":
			if m.billingExportField == 0 {
				m.billingExportFrom = prevMonth(m.billingExportFrom)
			} else {
				prev := prevMonth(m.billingExportTo)
				if prev >= m.billingExportFrom {
					m.billingExportTo = prev
				}
			}
		case "right", "l":
			now := time.Now().Format("2006-01")
			if m.billingExportField == 0 {
				next := nextMonth(m.billingExportFrom)
				if next <= m.billingExportTo {
					m.billingExportFrom = next
				}
			} else {
				next := nextMonth(m.billingExportTo)
				if next <= now {
					m.billingExportTo = next
				}
			}
		case "enter":
			m.billingExportOverlay = false
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.exportBillingCSV(m.billingExportFrom, m.billingExportTo))
		}
		return m, nil
	}

	// ── Secret content overlay: close on any key ──
	if m.secShowContent {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		default:
			m.secShowContent = false
			m.secContent = ""
		}
		return m, nil
	}

	// ── Tag action overlay: pull instructions ──
	if m.regTagActionOverlay {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		default:
			m.regTagActionOverlay = false
		}
		return m, nil
	}

	// ── Tag delete confirm ──
	if m.regConfirmDeleteTags {
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

	// ── Registry browser filter mode ──
	if m.regBrowserFiltering {
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
			if len(msg.Runes) == 1 {
				m.regBrowserFilter += string(msg.Runes)
				m.regBrowserCursor = 0
				m.regBrowserScrollY = 0
			}
		}
		return m, nil
	}
	// ── Registry tag filter mode ──
	if m.regTagFiltering {
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
			if len(msg.Runes) == 1 {
				m.regTagFilter += string(msg.Runes)
				m.regBrowserTagCursor = 0
				m.regBrowserTagScrollY = 0
			}
		}
		return m, nil
	}
	// ── Registry filter mode ──
	if m.registryFiltering {
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
			if len(msg.Runes) == 1 {
				m.registryFilter += string(msg.Runes)
				m.registryCursor = 0
				m.registryScrollY = 0
			}
		}
		return m, nil
	}
	// ── Secrets dashboard filter mode ──
	if m.secretFiltering {
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
			if len(msg.Runes) == 1 {
				m.secretFilter += string(msg.Runes)
				m.secretCursor = 0
				m.secretScrollY = 0
			}
		}
		return m, nil
	}
	// ── Secrets browser filter mode ──
	if m.secBrowserFiltering {
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
			if len(msg.Runes) == 1 {
				m.secBrowserFilter += string(msg.Runes)
				m.secBrowserCursor = 0
				m.secBrowserScrollY = 0
			}
		}
		return m, nil
	}

	// ── Secret delete confirm ──
	if m.secConfirmDelete {
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

	switch msg.String() {
	case "ctrl+c", "q":
		if m.showConfirm || m.input.active || m.secShowContent {
			// Dismiss overlays rather than quitting.
			m.showConfirm = false
			m.input.active = false
			m.secShowContent = false
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.secShowContent {
			m.secShowContent = false
			m.secContent = ""
			return m, nil
		}
		if m.showConfirm {
			m.showConfirm = false
			m.confirmItems = nil
			return m, nil
		}
		// Dismiss tag action overlay.
		if m.regTagActionOverlay {
			m.regTagActionOverlay = false
			return m, nil
		}
		// Dismiss registry tag delete confirm.
		if m.regConfirmDeleteTags {
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
			return m, nil
		}
		// Clear tag filter if active.
		if m.regTagFiltering {
			m.regTagFiltering = false
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		if m.regTagFilter != "" {
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		// Return to images pane from versions pane.
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 {
			m.regBrowserFocus = 0
			m.regTagSelected = nil
			return m, nil
		}
		// Clear registry browser filter if active.
		if m.regBrowserFilter != "" {
			m.regBrowserFilter = ""
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
			return m, nil
		}
		// Clear registry filter if active.
		if m.registryFilter != "" {
			m.registryFilter = ""
			m.registryCursor = 0
			m.registryScrollY = 0
			return m, nil
		}
		// Clear bucket filter if active without entering filter mode.
		if m.bucketFilter != "" {
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
			return m, nil
		}
		// Clear secrets browser filter if active.
		if m.secBrowserFilter != "" {
			m.secBrowserFilter = ""
			m.secBrowserCursor = 0
			m.secBrowserScrollY = 0
			return m, nil
		}
		// Clear secrets dashboard filter if active.
		if m.secretFilter != "" {
			m.secretFilter = ""
			m.secretCursor = 0
			m.secretScrollY = 0
			return m, nil
		}
		return m.handleEsc()
	case "f5":
		if !m.loading {
			m.loading = true
			if m.state == stateObjectBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, m.browserPrefix))
			}
			if m.state == stateDashboard && m.activeService == serviceBilling {
				return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
			}
			if m.state == stateK8sBrowser {
				m.k8sNodesLoading = true
				poolID := ""
				if len(m.k8sBrowserNodePools) > 0 && m.k8sBrowserPoolCursor < len(m.k8sBrowserNodePools) {
					poolID = m.k8sBrowserNodePools[m.k8sBrowserPoolCursor].id
				}
				return m, m.fetchNodes(m.k8sBrowserCluster, poolID)
			}
			if m.state == stateRegistryBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(m.regBrowserNamespace))
			}
			if m.state == stateSecretsBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchSecretVersions(m.secBrowserSecret))
			}
			return m, tea.Batch(m.spin.Tick, m.fetchData())
		}
	case "e", "E":
		if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading && !m.billingExportOverlay {
			m.billingExportFrom = time.Now().AddDate(0, -11, 0).Format("2006-01")
			m.billingExportTo = time.Now().Format("2006-01")
			m.billingExportField = 0
			m.billingExportOverlay = true
		}
	case "p", "P":
		if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
			m.billingProjectOverlay = true
			m.billingProjectCursor = m.billingProjectIdx
		}
	case "/":
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketFiltering = true
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceRegistry {
			m.registryFiltering = true
			m.registryFilter = ""
			m.registryCursor = 0
			m.registryScrollY = 0
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceSecrets {
			m.secretFiltering = true
			m.secretFilter = ""
			m.secretCursor = 0
			m.secretScrollY = 0
		}
		if m.state == stateSecretsBrowser && !m.loading {
			m.secBrowserFiltering = true
			m.secBrowserFilter = ""
			m.secBrowserCursor = 0
			m.secBrowserScrollY = 0
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.loading {
			m.regTagFiltering = true
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 0 {
			m.regBrowserFiltering = true
			m.regBrowserFilter = ""
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		}
	case "y", "Y":
		if m.showConfirm {
			items := m.confirmItems
			bucket := m.browserBucket
			prefix := m.browserPrefix
			m.showConfirm = false
			m.confirmItems = nil
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.deleteEntries(bucket, prefix, items))
		}
		if m.regConfirmDeleteTags {
			imageID := m.regConfirmDeleteImgID
			tags := m.regConfirmTagsToDelete
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.deleteRegistryTags(imageID, tags))
		}
		if m.state == stateK8sBrowser && m.k8sConfirmReboot {
			m.k8sConfirmReboot = false
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.rebootNode(m.k8sRebootNodeID, m.k8sBrowserCluster.region))
		}
		if m.state == stateK8sBrowser && m.k8sConfirmReplace {
			m.k8sConfirmReplace = false
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.replaceNode(m.k8sReplaceNodeID, m.k8sBrowserCluster.region))
		}
	case "n", "N":
		if m.showConfirm {
			m.showConfirm = false
			m.confirmItems = nil
			return m, nil
		}
		if m.regConfirmDeleteTags {
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
		}
		if m.state == stateSecretsBrowser && !m.loading {
			m.input.active = true
			m.input.mode = inputModeSecretNewVersion
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		}
	case "c", "C":
		if m.loading || m.showConfirm || m.secConfirmDelete {
			return m, nil
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.input.active = true
			m.input.mode = inputModeBucket
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		} else if m.state == stateObjectBrowser {
			m.input.active = true
			m.input.mode = inputModeFolder
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceSecrets {
			m.input.active = true
			m.input.mode = inputModeSecret
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		}
	case "u", "U":
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			m.input.active = true
			m.input.mode = inputModeUpload
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		}
		if m.state == stateSecretsBrowser && !m.loading {
			visible := m.filteredSecretVersions()
			if len(visible) > 0 && m.secBrowserCursor < len(visible) {
				m.input.active = true
				m.input.mode = inputModeSecretUpdateDesc
				m.input.value = ""
				m.input.cursor = 0
				m.input.errStr = ""
			}
		}
	case "d", "D":
		if m.state == stateDashboard && m.activeService == serviceSecrets && m.focus == focusContent && !m.loading && !m.secConfirmDelete {
			visible := m.filteredSecrets()
			if len(visible) > 0 && m.secretCursor < len(visible) {
				s := visible[m.secretCursor]
				m.secConfirmDeleteID = s.id
				m.secConfirmDeleteName = s.name
				m.secConfirmDelete = true
			}
		}
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			var targets []bucketEntry
			if len(m.browserSelected) > 0 {
				for _, e := range m.browserEntries {
					if m.browserSelected[e.fullKey] {
						targets = append(targets, e)
					}
				}
			} else if len(m.browserEntries) > 0 {
				targets = []bucketEntry{m.browserEntries[m.browserCursor]}
			}
			if len(targets) > 0 {
				m.confirmItems = targets
				m.showConfirm = true
			}
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
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
		}
	case "r", "R":
		if m.state == stateK8sBrowser && m.k8sBrowserFocus == 1 && !m.loading && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
			if len(m.k8sBrowserNodes) > 0 && m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes) {
				n := m.k8sBrowserNodes[m.k8sBrowserNodeCursor]
				m.k8sRebootNodeID = n.id
				m.k8sRebootNodeName = n.name
				m.k8sConfirmReboot = true
			}
		}
	case "x", "X":
		if m.state == stateK8sBrowser && m.k8sBrowserFocus == 1 && !m.loading && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
			if len(m.k8sBrowserNodes) > 0 && m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes) {
				n := m.k8sBrowserNodes[m.k8sBrowserNodeCursor]
				m.k8sReplaceNodeID = n.id
				m.k8sReplaceNodeName = n.name
				m.k8sConfirmReplace = true
			}
		}
	case "tab":
		if m.state == stateDashboard {
			m.focus = focusNav + (m.focus-focusNav+1)%2
			m.showDropdown = false
		}
		if m.state == stateK8sBrowser && !m.loading && !m.k8sConfirmReboot {
			if m.k8sBrowserFocus == 0 && len(m.k8sBrowserNodePools) > 0 {
				m.k8sBrowserFocus = 1
				m.k8sBrowserNodeCursor = 0
				m.k8sBrowserNodeScrollY = 0
			} else {
				m.k8sBrowserFocus = 0
			}
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
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
		}
	case "left", "h":
		if m.state == stateProfilePicker && !m.loading {
			m.pickerAction = (m.pickerAction - 1 + pickerActionCount) % pickerActionCount
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX = max(0, m.bucketScrollX-4)
		} else if m.state == stateObjectBrowser {
			m.browserScrollX = max(0, m.browserScrollX-4)
		} else if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
			m.billingPeriod = prevMonth(m.billingPeriod)
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
		}
	case "right", "l":
		if m.state == stateProfilePicker && !m.loading {
			m.pickerAction = (m.pickerAction + 1) % pickerActionCount
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX += 4
		} else if m.state == stateObjectBrowser {
			m.browserScrollX += 4
		} else if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
			next := nextMonth(m.billingPeriod)
			// Don't navigate into the future beyond current month
			now := time.Now().Format("2006-01")
			if next <= now {
				m.billingPeriod = next
				m.loading = true
				m.billingExportMsg = ""
				return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
			}
		}
	case "up", "k":
		return m.handleUp()
	case "down", "j":
		return m.handleDown()
	case "enter":
		return m.handleEnter()
	case " ":
		if m.state == stateObjectBrowser && len(m.browserEntries) > 0 {
			key := m.browserEntries[m.browserCursor].fullKey
			if m.browserSelected == nil {
				m.browserSelected = make(map[string]bool)
			}
			m.browserSelected[key] = !m.browserSelected[key]
			if m.browserCursor < len(m.browserEntries)-1 {
				m.browserCursor++
			}
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
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
		}
	case "a":
		if m.state == stateObjectBrowser && len(m.browserEntries) > 0 {
			if m.browserSelected == nil {
				m.browserSelected = make(map[string]bool)
			}
			allSelected := len(m.browserSelected) == len(m.browserEntries)
			for _, e := range m.browserEntries {
				if allSelected {
					delete(m.browserSelected, e.fullKey)
				} else {
					m.browserSelected[e.fullKey] = true
				}
			}
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
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
		}
	}
	return m, nil
}

func (m rootModel) handleEsc() (rootModel, tea.Cmd) {
	switch m.state {
	case stateK8sBrowser:
		if m.k8sConfirmReboot {
			m.k8sConfirmReboot = false
			return m, nil
		}
		if m.k8sConfirmReplace {
			m.k8sConfirmReplace = false
			return m, nil
		}
		if m.k8sBrowserFocus == 1 {
			m.k8sBrowserFocus = 0
			return m, nil
		}
		m.state = stateDashboard
		m.activeService = serviceK8s
		m.k8sBrowserNodes = nil
		m.k8sBrowserNodePools = nil
		m.k8sConfirmReboot = false
		m.k8sConfirmReplace = false
	case stateSecretsBrowser:
		m.state = stateDashboard
		m.activeService = serviceSecrets
		m.secBrowserFilter = ""
		m.secBrowserFiltering = false
		m.secShowContent = false
		m.secContent = ""
	case stateRegistryBrowser:
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
	case stateObjectBrowser:
		if m.browserPrefix == "" {
			// At root of bucket — go back to the bucket list.
			m.state = stateDashboard
		} else {
			// Go up one level.
			parent := parentPrefix(m.browserPrefix)
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, parent))
		}
	case stateDashboard:
		m.showDropdown = false
		m.state = stateProfilePicker
		m.err = nil
	}
	return m, nil
}

func (m rootModel) handleUp() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		m.profileCursor = (m.profileCursor - 1 + len(m.profileNames)) % len(m.profileNames)

	case m.showDropdown && len(m.projects) > 0:
		m.dropdownIndex = (m.dropdownIndex - 1 + len(m.projects)) % len(m.projects)

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		if m.browserCursor > 0 {
			m.browserCursor--
			m.browserScrollX = 0
			if m.browserCursor < m.browserScrollY {
				m.browserScrollY = m.browserCursor
			}
		}

	case m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceBilling && len(m.billingDetail) > 0:
		if m.billingCursor > 0 {
			m.billingCursor--
			if m.billingCursor < m.billingScrollY {
				m.billingScrollY = m.billingCursor
			}
		}

	case m.state == stateRegistryBrowser:
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

	case m.state == stateDashboard && m.focus == focusNav:
		m.activeService = (m.activeService - 1 + serviceCount) % serviceCount
		if m.activeService == serviceBilling && !m.loading {
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
		}

	case m.state == stateK8sBrowser && !m.k8sConfirmReboot:
		if m.k8sBrowserFocus == 0 {
			if m.k8sBrowserPoolCursor > 0 {
				m.k8sBrowserPoolCursor--
				if m.k8sBrowserPoolCursor < m.k8sBrowserPoolScrollY {
					m.k8sBrowserPoolScrollY = m.k8sBrowserPoolCursor
				}
				m.k8sNodesLoading = true
				return m, m.fetchNodes(m.k8sBrowserCluster, m.k8sBrowserNodePools[m.k8sBrowserPoolCursor].id)
			}
		} else {
			if m.k8sBrowserNodeCursor > 0 {
				m.k8sBrowserNodeCursor--
				if m.k8sBrowserNodeCursor < m.k8sBrowserNodeScrollY {
					m.k8sBrowserNodeScrollY = m.k8sBrowserNodeCursor
				}
			}
		}

	case m.state == stateSecretsBrowser:
		visible := m.filteredSecretVersions()
		if len(visible) > 0 && m.secBrowserCursor > 0 {
			m.secBrowserCursor--
			if m.secBrowserCursor < m.secBrowserScrollY {
				m.secBrowserScrollY = m.secBrowserCursor
			}
		}

	case m.state == stateDashboard && m.focus == focusContent:
		fb := m.filteredBuckets()
		if m.activeService == serviceObjectStorage && len(fb) > 0 {
			if m.bucketCursor > 0 {
				m.bucketCursor--
				m.bucketScrollX = 0
				if m.bucketCursor < m.bucketScrollY {
					m.bucketScrollY = m.bucketCursor
				}
				return m, m.maybeCalculateSize()
			}
			return m, nil
		}
		if m.activeService == serviceK8s && len(m.clusters) > 0 {
			m.clusterCursor = (m.clusterCursor - 1 + len(m.clusters)) % len(m.clusters)
		}
		if m.activeService == serviceRegistry && len(m.registryNamespaces) > 0 {
			if m.registryCursor > 0 {
				m.registryCursor--
				if m.registryCursor < m.registryScrollY {
					m.registryScrollY = m.registryCursor
				}
			}
		}
		if m.activeService == serviceSecrets {
			fs := m.filteredSecrets()
			if len(fs) > 0 && m.secretCursor > 0 {
				m.secretCursor--
				if m.secretCursor < m.secretScrollY {
					m.secretScrollY = m.secretCursor
				}
			}
		}
	}
	return m, nil
}

func (m rootModel) handleDown() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		m.profileCursor = (m.profileCursor + 1) % len(m.profileNames)

	case m.showDropdown && len(m.projects) > 0:
		m.dropdownIndex = (m.dropdownIndex + 1) % len(m.projects)

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		if m.browserCursor < len(m.browserEntries)-1 {
			m.browserCursor++
			m.browserScrollX = 0
		}

	case m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceBilling && len(m.billingDetail) > 0:
		if m.billingCursor < len(m.billingDetail)-1 {
			m.billingCursor++
		}

	case m.state == stateRegistryBrowser:
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

	case m.state == stateDashboard && m.focus == focusNav:
		m.activeService = (m.activeService + 1) % serviceCount
		if m.activeService == serviceBilling && !m.loading {
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
		}

	case m.state == stateK8sBrowser && !m.k8sConfirmReboot:
		if m.k8sBrowserFocus == 0 {
			if m.k8sBrowserPoolCursor < len(m.k8sBrowserNodePools)-1 {
				m.k8sBrowserPoolCursor++
				if m.k8sBrowserPoolCursor >= m.k8sBrowserPoolScrollY+20 {
					m.k8sBrowserPoolScrollY++
				}
				m.k8sNodesLoading = true
				return m, m.fetchNodes(m.k8sBrowserCluster, m.k8sBrowserNodePools[m.k8sBrowserPoolCursor].id)
			}
		} else {
			if m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes)-1 {
				m.k8sBrowserNodeCursor++
			}
		}

	case m.state == stateSecretsBrowser:
		visible := m.filteredSecretVersions()
		if len(visible) > 0 && m.secBrowserCursor < len(visible)-1 {
			m.secBrowserCursor++
		}

	case m.state == stateDashboard && m.focus == focusContent:
		fb := m.filteredBuckets()
		if m.activeService == serviceObjectStorage && len(fb) > 0 {
			if m.bucketCursor < len(fb)-1 {
				m.bucketCursor++
				m.bucketScrollX = 0
				return m, m.maybeCalculateSize()
			}
			return m, nil
		}
		if m.activeService == serviceK8s && len(m.clusters) > 0 {
			m.clusterCursor = (m.clusterCursor + 1) % len(m.clusters)
		}
		if m.activeService == serviceRegistry && len(m.registryNamespaces) > 0 {
			if m.registryCursor < len(m.registryNamespaces)-1 {
				m.registryCursor++
			}
		}
		if m.activeService == serviceSecrets {
			fs := m.filteredSecrets()
			if len(fs) > 0 && m.secretCursor < len(fs)-1 {
				m.secretCursor++
			}
		}
	}
	return m, nil
}

func (m rootModel) handleEnter() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
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

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceObjectStorage && len(m.filteredBuckets()) > 0:
		fb := m.filteredBuckets()
		b := fb[m.bucketCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(b.name, ""))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceK8s && len(m.clusters) > 0:
		cl := m.clusters[m.clusterCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchNodePools(cl))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceRegistry && len(m.filteredRegistryNamespaces()) > 0:
		fn := m.filteredRegistryNamespaces()
		ns := fn[m.registryCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(ns))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceBilling:
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceSecrets && len(m.filteredSecrets()) > 0:
		fs := m.filteredSecrets()
		s := fs[m.secretCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchSecretVersions(s))

	case m.state == stateSecretsBrowser && !m.loading:
		visible := m.filteredSecretVersions()
		if len(visible) > 0 && m.secBrowserCursor < len(visible) {
			rev := visible[m.secBrowserCursor].revision
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.accessSecretVersion(m.secBrowserSecret.id, rev))
		}

	case m.state == stateRegistryBrowser && m.regBrowserFocus == 1:
		visible := m.filteredRegistryImages()
		if len(visible) > 0 && m.regBrowserCursor < len(visible) {
			img := visible[m.regBrowserCursor]
			if len(img.tags) > 0 && m.regBrowserTagCursor < len(img.tags) {
				m.regTagActionOverlay = true
			}
		}
		return m, nil

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		entry := m.browserEntries[m.browserCursor]
		if entry.isDir {
			// Descend into the folder.
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, entry.fullKey))
		}
		// File selected — no action for now (could open detail overlay later).
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
