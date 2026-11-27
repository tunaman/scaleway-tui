package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/minio/minio-go/v7"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// testRegionCfg builds a minimal in-memory scw.Config with one profile that
// has dummy credentials, so region-switch code can be exercised offline.
func testRegionCfg() *scw.Config {
	return &scw.Config{
		Profiles: map[string]*scw.Profile{
			"test": {
				AccessKey:     scw.StringPtr("test-access-key"),
				SecretKey:     scw.StringPtr("test-secret-key"),
				DefaultRegion: scw.StringPtr("nl-ams"),
			},
		},
	}
}

func TestResolveRegion(t *testing.T) {
	prof := &scw.Profile{DefaultRegion: scw.StringPtr("nl-ams")}

	if got := resolveRegion("fr-par", prof); got != "fr-par" {
		t.Errorf("resolveRegion saved=fr-par → %q, want fr-par", got)
	}
	if got := resolveRegion("not-a-region", prof); got != "nl-ams" {
		t.Errorf("resolveRegion unknown saved → %q, want profile default nl-ams", got)
	}
	if got := resolveRegion("", prof); got != "nl-ams" {
		t.Errorf("resolveRegion empty saved → %q, want profile default nl-ams", got)
	}
	profNoRegion := &scw.Profile{}
	if got := resolveRegion("", profNoRegion); got != "nl-ams" {
		t.Errorf("resolveRegion empty saved + no profile default → %q, want nl-ams", got)
	}
}

func TestCurrentRegionIndex(t *testing.T) {
	found := currentRegionIndex("nl-ams")
	for i, r := range scw.AllRegions {
		if string(r) == "nl-ams" && found != i {
			t.Errorf("currentRegionIndex(nl-ams) = %d, want %d", found, i)
		}
	}
	if got := currentRegionIndex("bogus"); got != 0 {
		t.Errorf("currentRegionIndex(bogus) = %d, want 0", got)
	}
}

func TestRegionKeyOpensOverlay(t *testing.T) {
	m := rootModel{state: stateDashboard, activeRegion: "nl-ams", width: 120, height: 40}
	m, cmd := m.regionKey()
	if cmd != nil {
		t.Error("regionKey returned a non-nil cmd")
	}
	if !m.regionPickerOverlay {
		t.Error("regionKey did not open the overlay")
	}
	if m.regionPickerCursor != currentRegionIndex("nl-ams") {
		t.Errorf("cursor = %d, want %d", m.regionPickerCursor, currentRegionIndex("nl-ams"))
	}

	// Not available outside the dashboard / while loading.
	for _, tc := range []rootModel{
		{state: stateObjectBrowser, activeRegion: "nl-ams"},
		{state: stateDashboard, activeRegion: "nl-ams", loading: true},
	} {
		if m2, _ := tc.regionKey(); m2.regionPickerOverlay {
			t.Errorf("regionKey opened overlay for state=%d loading=%v", tc.state, tc.loading)
		}
	}
	// Already open: no-op (keeps the existing overlay and cursor).
	target := rootModel{state: stateDashboard, activeRegion: "nl-ams", regionPickerOverlay: true, regionPickerCursor: 2}
	m2, _ := target.regionKey()
	if !m2.regionPickerOverlay || m2.regionPickerCursor != 2 {
		t.Errorf("regionKey changed an already-open overlay: overlay=%v cursor=%d", m2.regionPickerOverlay, m2.regionPickerCursor)
	}
}

func TestHandleRegionPickerKeyNavigation(t *testing.T) {
	// Esc closes the overlay.
	m := rootModel{state: stateDashboard, activeRegion: "nl-ams", regionPickerOverlay: true, width: 120, height: 40}
	m, _ = m.handleRegionPickerKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.regionPickerOverlay {
		t.Error("esc did not close the overlay")
	}

	// Up/down move the cursor, clamped at both ends.
	m = rootModel{state: stateDashboard, activeRegion: "nl-ams", regionPickerOverlay: true, regionPickerCursor: 0, width: 120, height: 40}
	m, _ = m.handleRegionPickerKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.regionPickerCursor != 0 {
		t.Errorf("up at top: cursor = %d, want 0", m.regionPickerCursor)
	}
	for i := 0; i < len(scw.AllRegions)+1; i++ {
		m, _ = m.handleRegionPickerKey(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.regionPickerCursor != len(scw.AllRegions)-1 {
		t.Errorf("down past bottom: cursor = %d, want %d", m.regionPickerCursor, len(scw.AllRegions)-1)
	}

	// Enter on the current region is a no-op (overlay closes, nothing else).
	m = rootModel{state: stateDashboard, activeRegion: "nl-ams", regionPickerOverlay: true, regionPickerCursor: currentRegionIndex("nl-ams"), width: 120, height: 40}
	m, cmd := m.handleRegionPickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.regionPickerOverlay {
		t.Error("enter did not close the overlay")
	}
	if m.activeRegion != "nl-ams" {
		t.Errorf("activeRegion = %q, want unchanged nl-ams", m.activeRegion)
	}
	if m.loading {
		t.Error("loading = true after a no-op enter")
	}
	if cmd != nil {
		t.Error("no-op enter returned a non-nil cmd")
	}
}

func TestSwitchRegion(t *testing.T) {
	// Redirect HOME so the config write doesn't touch the real home dir.
	t.Setenv("HOME", t.TempDir())

	m := rootModel{
		state:             stateDashboard,
		scwCfg:            testRegionCfg(),
		activeProfileName: "test",
		activeRegion:      "nl-ams",
		width:             120,
		height:            40,
		// Stale region-scoped state to be cleared:
		servers:            []instanceServer{{id: "s1", name: "stale"}},
		volumes:            []volumeItem{{id: "v1", name: "stale"}},
		snapshots:          []snapshotItem{{id: "snap1", name: "stale"}},
		volFetchErr:        os.ErrNotExist,
		buckets:            []bucket{{name: "stale"}},
		clusters:           []cluster{{id: "c1", name: "stale"}},
		registryNamespaces: []registryNamespace{{id: "ns1", name: "stale"}},
		secrets:            []secretItem{{id: "sec1", name: "stale"}},
		browserBucket:      "stale-bucket",
		browserEntries:     []bucketEntry{{name: "stale-obj"}},
		instFilter:         "stale",
		browserSelected:    map[string]bool{"stale-obj": true},
	}

	m, cmd := m.switchRegion("fr-par")
	if cmd == nil {
		t.Error("switchRegion returned a nil cmd")
	}
	if m.activeRegion != "fr-par" {
		t.Errorf("activeRegion = %q, want fr-par", m.activeRegion)
	}
	if m.minioClient == nil {
		t.Error("minioClient not rebuilt")
	}
	if _, ok := interface{}(m.minioClient).(*minio.Client); !ok {
		t.Error("minioClient is not a *minio.Client")
	}
	if m.servers != nil || m.volumes != nil || m.snapshots != nil || m.buckets != nil || m.clusters != nil || m.registryNamespaces != nil || m.secrets != nil {
		t.Error("stale region-scoped data was not cleared")
	}
	if m.volFetchErr != nil {
		t.Error("volFetchErr not cleared")
	}
	if m.browserBucket != "" || m.browserEntries != nil || m.browserSelected != nil || m.instFilter != "" {
		t.Error("browser state not cleared")
	}
	if m.state != stateDashboard {
		t.Errorf("state = %d, want stateDashboard", m.state)
	}
	if !m.loading {
		t.Error("loading = false, want true (fetch in flight)")
	}
	if m.err != nil {
		t.Errorf("err = %v, want nil", m.err)
	}

	// The choice is persisted.
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "scw-tui", "config.json"))
	if err != nil {
		t.Fatalf("reading persisted config: %v", err)
	}
	var saved tuiConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parsing persisted config: %v", err)
	}
	if saved.ActiveRegion != "fr-par" {
		t.Errorf("persisted active_region = %q, want fr-par", saved.ActiveRegion)
	}
	if saved.ActiveProfile != "test" {
		t.Errorf("persisted active_profile = %q, want test", saved.ActiveProfile)
	}
}

func TestSwitchRegionUnknownProfile(t *testing.T) {
	m := rootModel{
		state:             stateDashboard,
		scwCfg:            testRegionCfg(),
		activeProfileName: "does-not-exist",
		activeRegion:      "nl-ams",
		minioClient:       nil,
	}
	m, cmd := m.switchRegion("fr-par")
	if cmd != nil {
		t.Error("switchRegion returned a cmd for a bad profile")
	}
	if m.err == nil {
		t.Error("expected an error for a bad profile")
	}
	if m.activeRegion != "nl-ams" {
		t.Errorf("activeRegion = %q, want unchanged nl-ams", m.activeRegion)
	}
}

func TestRenderRegionPicker(t *testing.T) {
	m := rootModel{
		state:               stateDashboard,
		width:               100,
		height:              40,
		activeRegion:        "nl-ams",
		regionPickerOverlay: true,
		regionPickerCursor:  currentRegionIndex("fr-par"),
	}
	out := m.renderRegionPicker()
	for _, r := range scw.AllRegions {
		if !strings.Contains(out, string(r)) {
			t.Errorf("overlay missing region %q", r)
		}
	}
	if !strings.Contains(out, "✓") {
		t.Error("overlay missing checkmark for the active region")
	}
	if !strings.Contains(out, "▌") {
		t.Error("overlay missing cursor marker")
	}
	if !strings.Contains(out, "switch") {
		t.Error("overlay missing hint line")
	}
}

func TestDrawDashboardWithRegionPicker(t *testing.T) {
	m := rootModel{
		state:               stateDashboard,
		focus:               focusContent,
		activeService:       serviceInstance,
		width:               120,
		height:              45,
		project:             "test-project",
		activeRegion:        "nl-ams",
		regionPickerOverlay: true,
		regionPickerCursor:  currentRegionIndex("fr-par"),
	}
	// Must not panic and must render the overlay, not the base dashboard.
	out := m.drawDashboard()
	if !strings.Contains(out, "▌") {
		t.Error("dashboard with region picker did not render the overlay")
	}
}
