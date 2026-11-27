package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Key handling — region switcher
//
// The region picker overlay lists every region known to the active SDK and
// switches the active region on Enter. Region-scoped services (Instances,
// Object Storage, K8s, Registry, Secrets, Volumes) are reset and refetched;
// org-scoped services (Billing, IAM) are left untouched.
// ─────────────────────────────────────────────

// regionKey opens the region picker overlay on the dashboard.
func (m rootModel) regionKey() (rootModel, tea.Cmd) {
	if m.state != stateDashboard || m.loading || m.regionPickerOverlay {
		return m, nil
	}
	m.regionPickerOverlay = true
	m.regionPickerCursor = currentRegionIndex(m.activeRegion)
	return m, nil
}

// currentRegionIndex returns the index of region within scw.AllRegions,
// falling back to 0 when unknown.
func currentRegionIndex(region string) int {
	for i, r := range scw.AllRegions {
		if string(r) == region {
			return i
		}
	}
	return 0
}

// handleRegionPickerKey handles keys while the region picker is open.
func (m rootModel) handleRegionPickerKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.regionPickerOverlay = false
	case "up", "k":
		if m.regionPickerCursor > 0 {
			m.regionPickerCursor--
		}
	case "down", "j":
		if m.regionPickerCursor < len(scw.AllRegions)-1 {
			m.regionPickerCursor++
		}
	case "enter":
		m.regionPickerOverlay = false
		newRegion := string(scw.AllRegions[m.regionPickerCursor])
		if newRegion == m.activeRegion {
			return m, nil
		}
		return m.switchRegion(newRegion)
	}
	return m, nil
}

// switchRegion switches to newRegion: rebuilds the region-scoped S3 client,
// resets all region-scoped state, persists the choice and refetches.
func (m rootModel) switchRegion(newRegion string) (rootModel, tea.Cmd) {
	prof, err := m.scwCfg.GetProfile(m.activeProfileName)
	if err != nil {
		m.err = err
		return m, nil
	}
	mc, err := buildS3Client(prof, newRegion)
	if err != nil {
		m.err = err
		return m, nil
	}
	m.activeRegion = newRegion
	m.minioClient = mc
	m.resetRegionScopedState()
	m.err = nil
	// Persist the choice for the next launch.
	saveTUIConfig(tuiConfig{ActiveProfile: m.activeProfileName, ActiveRegion: newRegion})
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchData())
}

// resetRegionScopedState clears every region-dependent field and returns the
// TUI to the dashboard (any browser is left behind — its contents are region-scoped).
func (m *rootModel) resetRegionScopedState() {
	// Dashboard data.
	m.buckets = nil
	m.clusters = nil
	m.registryNamespaces = nil
	m.secrets = nil
	m.servers = nil
	m.volumes = nil
	m.snapshots = nil
	m.volFetchErr = nil
	m.privateIPs = nil
	m.privateIPLoadingID = ""

	// Dashboard cursors / filters.
	m.bucketCursor = 0
	m.bucketScrollY = 0
	m.bucketScrollX = 0
	m.bucketFilter = ""
	m.bucketFiltering = false
	m.clusterCursor = 0
	m.prevBucketSel = -1
	m.registryCursor = 0
	m.registryScrollY = 0
	m.registryFilter = ""
	m.registryFiltering = false
	m.secretCursor = 0
	m.secretScrollY = 0
	m.secretFilter = ""
	m.secretFiltering = false
	m.instCursor = 0
	m.instScrollY = 0
	m.instFilter = ""
	m.instFiltering = false
	m.volFocus = 0
	m.volCursor = 0
	m.volScrollY = 0
	m.volSnapCursor = 0
	m.volSnapScrollY = 0
	m.volFilter = ""
	m.volFiltering = false
	m.volConfirmAction = volActionNone
	m.volConfirmID = ""
	m.volConfirmName = ""
	m.volConfirmServerID = ""
	m.volConfirmServerName = ""
	m.volAttachOverlay = false
	m.volAttachCursor = 0
	m.volAttachVolume = volumeItem{}
	m.instConfirmAction = instActionNone
	m.instConfirmServerID = ""
	m.instConfirmServerName = ""
	m.instConfirmServerZone = ""

	// Browser states — any of these holds region-specific data.
	m.state = stateDashboard
	m.browserBucket = ""
	m.browserPrefix = ""
	m.browserEntries = nil
	m.browserCursor = 0
	m.browserScrollY = 0
	m.browserScrollX = 0
	m.browserSelected = nil
	m.showConfirm = false
	m.confirmItems = nil
	m.input.active = false
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	m.upload.active = false
	m.download.active = false
	m.downloadTargets = nil
	m.downloadMsg = ""
	m.k8sBrowserCluster = cluster{}
	m.k8sBrowserNodePools = nil
	m.k8sBrowserNodes = nil
	m.k8sBrowserFocus = 0
	m.k8sBrowserPoolCursor = 0
	m.k8sBrowserPoolScrollY = 0
	m.k8sBrowserNodeCursor = 0
	m.k8sBrowserNodeScrollY = 0
	m.k8sNodesLoading = false
	m.k8sConfirmReboot = false
	m.k8sRebootNodeID = ""
	m.k8sRebootNodeName = ""
	m.k8sConfirmReplace = false
	m.k8sReplaceNodeID = ""
	m.k8sReplaceNodeName = ""
	m.regBrowserNamespace = registryNamespace{}
	m.regBrowserImages = nil
	m.regBrowserCursor = 0
	m.regBrowserScrollY = 0
	m.regBrowserFilter = ""
	m.regBrowserFiltering = false
	m.regBrowserFocus = 0
	m.regBrowserTagCursor = 0
	m.regBrowserTagScrollY = 0
	m.regTagActionOverlay = false
	m.regTagFilter = ""
	m.regTagFiltering = false
	m.regTagSelected = nil
	m.regConfirmDeleteTags = false
	m.regConfirmTagsToDelete = nil
	m.regConfirmDeleteImgID = ""
	m.regConfirmDeleteImgName = ""
	m.regTagsLoading = false
	m.secBrowserSecret = secretItem{}
	m.secBrowserVersions = nil
	m.secBrowserCursor = 0
	m.secBrowserScrollY = 0
	m.secBrowserFilter = ""
	m.secBrowserFiltering = false
	m.secShowContent = false
	m.secContent = ""
	m.secContentRevision = 0
	m.secConfirmDelete = false
	m.secConfirmDeleteID = ""
	m.secConfirmDeleteName = ""
}
