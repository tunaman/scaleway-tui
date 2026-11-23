package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — central dispatcher
//
// handleKey() intercepts the input overlay and per-service filter/overlay
// modes, then routes each key to the owning service. The per-service
// handlers live in keys_<service>.go (s3, k8s, instance, volumes, billing,
// registry, secrets, iam, picker, dashboard).
// ─────────────────────────────────────────────

func (m rootModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
				visible := m.filteredSecretVersions()
				if len(visible) > 0 && m.secBrowserCursor < len(visible) {
					m.loading = true
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
			case inputModeDownload:
				targets := m.downloadTargets
				m.downloadTargets = nil
				m.input.active = false
				m.download.active = true
				m.download.fileTotal = len(targets)
				return m, m.downloadFiles(m.browserBucket, m.browserPrefix, targets, val)
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
			// msg.Text contains the typed/pasted characters.
			if msg.Text != "" {
				insert := []rune(msg.Text)
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

	// ── Per-service filter modes and overlays (keys_<service>.go) ──
	if m.bucketFiltering {
		return m.handleFilterKey(msg, &m.bucketFiltering, &m.bucketFilter, m.resetBucketFilterCursors, false)
	}
	if m.billingProjectOverlay {
		return m.handleBillingProjectOverlayKey(msg)
	}
	if m.billingExportOverlay {
		return m.handleBillingExportOverlayKey(msg)
	}
	if m.volAttachOverlay {
		return m.handleVolAttachOverlayKey(msg)
	}
	if m.secShowContent {
		return m.handleSecShowContentKey(msg)
	}
	if m.regTagActionOverlay {
		return m.handleRegTagActionOverlayKey(msg)
	}
	if m.regConfirmDeleteTags {
		return m.handleRegConfirmDeleteKey(msg)
	}
	if m.regBrowserFiltering {
		return m.handleFilterKey(msg, &m.regBrowserFiltering, &m.regBrowserFilter, m.resetRegBrowserFilterCursors, false)
	}
	if m.regTagFiltering {
		return m.handleFilterKey(msg, &m.regTagFiltering, &m.regTagFilter, m.resetRegTagFilterCursors, true)
	}
	if m.registryFiltering {
		return m.handleFilterKey(msg, &m.registryFiltering, &m.registryFilter, m.resetRegistryFilterCursors, false)
	}
	if m.secretFiltering {
		return m.handleFilterKey(msg, &m.secretFiltering, &m.secretFilter, m.resetSecretFilterCursors, false)
	}
	if m.secBrowserFiltering {
		return m.handleFilterKey(msg, &m.secBrowserFiltering, &m.secBrowserFilter, m.resetSecBrowserFilterCursors, false)
	}
	if m.iamFiltering {
		return m.handleFilterKey(msg, &m.iamFiltering, &m.iamFilter, m.resetIAMFilterCursors, false)
	}
	if m.instFiltering {
		return m.handleFilterKey(msg, &m.instFiltering, &m.instFilter, m.resetInstFilterCursors, false)
	}
	if m.volFiltering {
		return m.handleFilterKey(msg, &m.volFiltering, &m.volFilter, m.resetVolFilterCursors, false)
	}
	if m.secConfirmDelete {
		return m.handleSecConfirmDeleteKey(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		// Only confirm/transfer overlays can be active here: the input overlay
		// swallows every key (incl. ctrl+c) while active, and the sec content
		// and volume attach overlays quit on ctrl+c in their own intercepts.
		if m.showConfirm || m.instConfirmAction != instActionNone ||
			m.upload.active || m.download.active || m.volConfirmAction != volActionNone {
			// Dismiss overlays rather than quitting.
			m.showConfirm = false
			m.instConfirmAction = instActionNone
			m.instConfirmServerID = ""
			m.instConfirmServerName = ""
			m.instConfirmServerZone = ""
			m.volConfirmAction = volActionNone
			m.volConfirmID = ""
			m.volConfirmName = ""
			m.volConfirmServerID = ""
			m.volConfirmServerName = ""
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.upload.active || m.download.active {
			// Transfers can't be interrupted — the overlay closes itself.
			return m, nil
		}
		if m.showConfirm {
			m.showConfirm = false
			m.confirmItems = nil
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
		// Clear IAM browser filter if active.
		if m.iamFilter != "" {
			m.iamFilter = ""
			m.iamCursor = 0
			m.iamScrollY = 0
			return m, nil
		}
		// Dismiss volume/snapshot action confirm.
		if m.volConfirmAction != volActionNone {
			m.volConfirmAction = volActionNone
			m.volConfirmID = ""
			m.volConfirmName = ""
			m.volConfirmServerID = ""
			m.volConfirmServerName = ""
			return m, nil
		}
		// Clear volumes/snapshots filter if active.
		if m.volFilter != "" {
			m.volFilter = ""
			m.volCursor = 0
			m.volScrollY = 0
			m.volSnapCursor = 0
			m.volSnapScrollY = 0
			return m, nil
		}
		// Dismiss server power-action confirm.
		if m.instConfirmAction != instActionNone {
			m.instConfirmAction = instActionNone
			m.instConfirmServerID = ""
			m.instConfirmServerName = ""
			m.instConfirmServerZone = ""
			return m, nil
		}
		// Clear instances filter if active.
		if m.instFilter != "" {
			m.instFilter = ""
			m.instCursor = 0
			m.instScrollY = 0
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
				return m, tea.Batch(m.spin.Tick, m.fetchNodes(m.k8sBrowserCluster, poolID))
			}
			if m.state == stateRegistryBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(m.regBrowserNamespace))
			}
			if m.state == stateSecretsBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchSecretVersions(m.secBrowserSecret))
			}
			if m.state == stateIAMBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchIAM())
			}
			if m.state == stateDashboard && m.activeService == serviceInstance {
				return m, tea.Batch(m.spin.Tick, m.fetchServers())
			}
			if m.state == stateDashboard && m.activeService == serviceVolumes {
				return m, tea.Batch(m.spin.Tick, m.fetchVolumes())
			}
			return m, tea.Batch(m.spin.Tick, m.fetchData())
		}
	case "e", "E":
		return m.billingExportKey()
	case "p", "P":
		return m.billingProjectKey()
	case "/":
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceInstance {
			return m.instanceStartFilterKey()
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			return m.bucketStartFilterKey()
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceRegistry {
			return m.registryStartFilterKey()
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceSecrets {
			return m.secretsStartFilterKey()
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceVolumes {
			return m.volumesStartFilterKey()
		}
		if m.state == stateSecretsBrowser && !m.loading {
			return m.secretsBrowserStartFilterKey()
		}
		if m.state == stateIAMBrowser && !m.loading {
			return m.iamStartFilterKey()
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.loading {
			return m.regTagStartFilterKey()
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 0 {
			return m.regBrowserStartFilterKey()
		}
	case "y", "Y":
		if m.showConfirm {
			return m.s3DeleteConfirmYes()
		}
		if m.regConfirmDeleteTags {
			return m.registryTagDeleteConfirmYes()
		}
		if m.state == stateK8sBrowser && m.k8sConfirmReboot {
			return m.k8sRebootConfirmYes()
		}
		if m.state == stateK8sBrowser && m.k8sConfirmReplace {
			return m.k8sReplaceConfirmYes()
		}
		if m.state == stateDashboard && m.activeService == serviceInstance && m.instConfirmAction != instActionNone {
			return m.instancePowerConfirmYes()
		}
		if m.volConfirmAction != volActionNone {
			return m.volumeConfirmYes()
		}
		// Object browser: yank (copy) the selected entry's full S3 key.
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm && !m.upload.active && !m.download.active {
			return m.s3YankKey()
		}
	case "n", "N":
		if m.instConfirmAction != instActionNone {
			return m.instancePowerConfirmNo()
		}
		if m.volConfirmAction != volActionNone {
			return m.volumeConfirmNo()
		}
		if m.showConfirm {
			return m.s3DeleteConfirmNo()
		}
		if m.regConfirmDeleteTags {
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
		}
		if m.state == stateSecretsBrowser && !m.loading {
			return m.secretsNewVersionKey()
		}
	case "c", "C":
		if m.loading || m.showConfirm || m.secConfirmDelete {
			return m, nil
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			return m.s3CreateBucketInput()
		} else if m.state == stateObjectBrowser {
			return m.s3CreateFolderInput()
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceSecrets {
			return m.secretsCreateInput()
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceInstance {
			return m.instanceCopyIPKey()
		} else if m.state == stateK8sBrowser && m.k8sBrowserFocus == 1 && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
			return m.k8sCopyNodeIPKey()
		} else if m.state == stateIAMBrowser && m.iamTab == iamTabAPIKeys && !m.iamFiltering {
			return m.iamCopyAccessKey()
		}
	case "u", "U":
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			return m.s3UploadInput()
		}
		if m.state == stateSecretsBrowser && !m.loading {
			return m.secretsUpdateDescInput()
		}
	case "d", "D":
		if m.state == stateDashboard && m.activeService == serviceSecrets && m.focus == focusContent && !m.loading && !m.secConfirmDelete {
			return m.secretsDeleteKey()
		}
		if m.state == stateDashboard && m.activeService == serviceVolumes && m.focus == focusContent && !m.loading {
			return m.volumesDeleteKey()
		}
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			return m.s3DeleteKey()
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
			return m.registryDeleteKey()
		}
	case "r", "R":
		if m.state == stateK8sBrowser && m.k8sBrowserFocus == 1 && !m.loading && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
			return m.k8sRebootKey()
		}
		if m.state == stateDashboard && m.activeService == serviceInstance && !m.loading && m.instConfirmAction == instActionNone {
			return m.instanceRebootKey()
		}
	case "o", "O":
		return m.instancePowerOnKey()
	case "s", "S":
		if m.state == stateDashboard && m.activeService == serviceInstance && !m.loading && m.instConfirmAction == instActionNone {
			return m.instancePowerOffKey()
		}
	case "x", "X":
		// Volumes: detach the selected (attached) volume from its server.
		if m.state == stateDashboard && m.activeService == serviceVolumes && m.volFocus == 0 &&
			m.focus == focusContent && !m.loading {
			return m.volumesDetachKey()
		}
		if m.state == stateK8sBrowser && m.k8sBrowserFocus == 1 && !m.loading && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
			return m.k8sReplaceKey()
		}
		// Object browser: download the selected file(s), or the file under
		// the cursor when nothing is selected.
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm && !m.upload.active && !m.download.active {
			return m.s3DownloadKey()
		}
	case "K":
		return m.k8sKubeConfigKey()
	case "tab":
		if m.state == stateIAMBrowser && !m.loading {
			return m.iamTabNextKey()
		}
		if m.state == stateDashboard {
			return m.dashboardTabKey()
		}
		if m.state == stateK8sBrowser && !m.loading && !m.k8sConfirmReboot {
			return m.k8sTabKey()
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
			return m.registryTabKey()
		}
	case "left", "h":
		if m.state == stateIAMBrowser && !m.loading {
			return m.iamTabPrevKey()
		}
		if m.state == stateProfilePicker && !m.loading {
			return m.pickerPrevAction()
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX = max(0, m.bucketScrollX-4)
		} else if m.state == stateObjectBrowser {
			m.browserScrollX = max(0, m.browserScrollX-4)
		} else if m.state == stateDashboard && m.activeService == serviceVolumes && m.focus == focusContent && !m.loading {
			// Volumes: switch to the Volumes tab.
			m.volFocus = 0
		} else if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
			return m.billingPrevMonthKey()
		}
	case "right", "l":
		if m.state == stateIAMBrowser && !m.loading {
			return m.iamTabNextKey()
		}
		if m.state == stateProfilePicker && !m.loading {
			return m.pickerNextAction()
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX += 4
		} else if m.state == stateObjectBrowser {
			m.browserScrollX += 4
		} else if m.state == stateDashboard && m.activeService == serviceVolumes && m.focus == focusContent && !m.loading {
			// Volumes: switch to the Snapshots tab.
			m.volFocus = 1
		} else if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
			return m.billingNextMonthKey()
		}
	case "shift+tab", "backtab":
		if m.state == stateIAMBrowser && !m.loading {
			return m.iamTabPrevKey()
		}
	case "up", "k":
		return m.handleUp()
	case "down", "j":
		return m.handleDown()
	case "enter":
		return m.handleEnter()
	case "space":
		if m.state == stateObjectBrowser {
			return m.s3SelectToggleKey()
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
			return m.registryTagSelectToggleKey()
		}
	case "a":
		// Volumes: attach the selected (unattached) volume to a server.
		if m.state == stateDashboard && m.activeService == serviceVolumes && m.volFocus == 0 &&
			m.focus == focusContent && !m.loading {
			return m.volumesAttachKey()
		}
		if m.state == stateObjectBrowser {
			return m.s3SelectAllKey()
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
			return m.registryTagSelectAllKey()
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Shared inline-filter handler
//
// All nine list filter modes (bucket, registry namespace/browser/tag, secret
// dashboard/browser, IAM, instances, volumes) share this handler: active and
// filter bind the mode's fields, reset zeroes the list's cursor(s)/scroll
// (pointer receiver, so it mutates the model handleKey returns), and
// quitOnCtrlC mirrors the historical per-mode behaviour — only the registry
// tag filter quits on ctrl+c.
// ─────────────────────────────────────────────

func (m rootModel) handleFilterKey(msg tea.KeyPressMsg, active *bool, filter *string, reset func(), quitOnCtrlC bool) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		*active = false
		*filter = ""
		reset()
	case "enter":
		// Lock in the first match and exit filter mode.
		*active = false
		reset()
	case "backspace", "ctrl+h":
		if len([]rune(*filter)) > 0 {
			runes := []rune(*filter)
			*filter = string(runes[:len(runes)-1])
		} else {
			// Empty filter + backspace → exit filter mode.
			*active = false
		}
		reset()
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	case "ctrl+c":
		if quitOnCtrlC {
			return m, tea.Quit
		}
	default:
		if msg.Text != "" {
			*filter += msg.Text
			reset()
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// State dispatchers (per-state bodies in keys_<service>.go)
// ─────────────────────────────────────────────

func (m rootModel) handleEsc() (rootModel, tea.Cmd) {
	switch m.state {
	case stateK8sBrowser:
		return m.k8sEsc()
	case stateSecretsBrowser:
		return m.secretsEsc()
	case stateIAMBrowser:
		return m.iamEsc()
	case stateRegistryBrowser:
		return m.registryEsc()
	case stateObjectBrowser:
		return m.s3Esc()
	case stateDashboard:
		return m.dashboardEsc()
	}
	return m, nil
}

func (m rootModel) handleUp() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		return m.pickerUp()
	case m.showDropdown && len(m.projects) > 0:
		return m.dropdownUp()
	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		return m.s3Up()
	case m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceBilling && len(m.billingDetail) > 0:
		return m.billingDetailUp()
	case m.state == stateRegistryBrowser:
		return m.registryUp()
	case m.state == stateDashboard && m.focus == focusNav:
		return m.dashboardNavUp()
	case m.state == stateK8sBrowser && !m.k8sConfirmReboot:
		return m.k8sUp()
	case m.state == stateSecretsBrowser:
		return m.secretsBrowserUp()
	case m.state == stateIAMBrowser:
		return m.iamUp()
	case m.state == stateDashboard && m.focus == focusContent:
		return m.dashboardContentUp()
	}
	return m, nil
}

func (m rootModel) handleDown() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		return m.pickerDown()
	case m.showDropdown && len(m.projects) > 0:
		return m.dropdownDown()
	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		return m.s3Down()
	case m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceBilling && len(m.billingDetail) > 0:
		return m.billingDetailDown()
	case m.state == stateRegistryBrowser:
		return m.registryDown()
	case m.state == stateDashboard && m.focus == focusNav:
		return m.dashboardNavDown()
	case m.state == stateK8sBrowser && !m.k8sConfirmReboot:
		return m.k8sDown()
	case m.state == stateSecretsBrowser:
		return m.secretsBrowserDown()
	case m.state == stateIAMBrowser:
		return m.iamDown()
	case m.state == stateDashboard && m.focus == focusContent:
		return m.dashboardContentDown()
	}
	return m, nil
}

func (m rootModel) handleEnter() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		return m.pickerEnter()
	case m.state == stateDashboard && m.focus == focusContent:
		return m.dashboardContentEnter()
	case m.state == stateSecretsBrowser && !m.loading:
		return m.secretsBrowserEnterKey()
	case m.state == stateRegistryBrowser && m.regBrowserFocus == 1:
		return m.registryBrowserEnterKey()
	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		return m.s3EnterKey()
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Clipboard (OSC 52)
// ─────────────────────────────────────────────

// copyToClipboard puts text on the system clipboard via the terminal's OSC 52
// support (tea.SetClipboard) and flashes a short note in the top bar.
// Terminals without OSC 52 support (e.g. stock xterm) simply ignore the
// sequence — the UI degrades to no copy, no error.
func (m rootModel) copyToClipboard(text string) (rootModel, tea.Cmd) {
	m, clear := m.flashNote("Copied → clipboard", clipboardNoteDuration)
	return m, tea.Batch(tea.SetClipboard(text), clear)
}

// flashNote shows a green "✓ <note>" in the top bar for d.
func (m rootModel) flashNote(note string, d time.Duration) (rootModel, tea.Cmd) {
	m.clipboardNote = note
	return m, tea.Tick(d, func(time.Time) tea.Msg { return clipboardNoteClearedMsg{} })
}

const (
	clipboardNoteDuration  = 2 * time.Second
	kubeConfigNoteDuration = 5 * time.Second // long enough to read the path
)
