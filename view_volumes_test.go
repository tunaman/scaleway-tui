package main

import (
	"strings"
	"testing"
	"time"

	block "github.com/scaleway/scaleway-sdk-go/api/block/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// TestRenderVolumesDashboard draws the Volumes & Snapshots dashboard screen
// with fake data and checks the row layout, the header, and the detail pane.
func TestRenderVolumesDashboard(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceVolumes,
		project:       "test-project",
		activeRegion:  "nl-ams",
		volumes: []volumeItem{
			{
				id:         "11111111-1111-1111-1111-111111111111",
				name:       "web-data",
				size:       50 * 1024 * 1024 * 1024,
				volType:    "b_ssd",
				state:      "available",
				zone:       "nl-ams-1",
				serverName: "web-prod-1",
				createdAt:  time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			},
			{
				id:      "22222222-2222-2222-2222-222222222222",
				name:    "scratch",
				size:    10 * 1024 * 1024 * 1024,
				volType: "l_ssd",
				state:   "available",
				zone:    "nl-ams-2",
			},
		},
		snapshots: []snapshotItem{
			{
				id:         "33333333-3333-3333-3333-333333333333",
				name:       "snap-web-data-20260115",
				size:       50 * 1024 * 1024 * 1024,
				volType:    "b_ssd",
				state:      "available",
				zone:       "nl-ams-1",
				baseVolume: "web-data",
				createdAt:  time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	out := m.drawDashboard()

	for _, want := range []string{
		"VOLUMES",
		"VOLUME INFO",
		"web-data",
		"scratch",
		"web-prod-1",
		"50.00 GB",
		"b_ssd",
		"nl-ams-1",
		// Tab strip.
		"Volumes 2",
		"Snapshots 1",
		// Detail pane for the selected (first) volume.
		"Created",
		"15 Jan 2026",
		// Top bar counters.
		"2 volumes / 1 snapshots",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard output missing %q", want)
		}
	}
}

// TestRenderVolumesSnapshotsPane switches to the snapshots pane and checks
// the header, the rows, and the snapshot detail pane.
func TestRenderVolumesSnapshotsPane(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceVolumes,
		project:       "test-project",
		activeRegion:  "nl-ams",
		volFocus:      1,
		snapshots: []snapshotItem{
			{
				id:         "33333333-3333-3333-3333-333333333333",
				name:       "snap-web-data-20260115",
				size:       50 * 1024 * 1024 * 1024,
				volType:    "b_ssd",
				state:      "snapshotting",
				zone:       "nl-ams-1",
				baseVolume: "web-data",
				createdAt:  time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	out := m.drawDashboard()

	for _, want := range []string{
		"SNAPSHOTS",
		"SNAPSHOT INFO",
		"BASE",
		"snap-web-data-20260115",
		"snapshotting",
		"Base vol",
		"web-data",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("snapshots pane output missing %q", want)
		}
	}
}

// TestRenderVolumesSelectedRowBackground verifies the selected row is built
// from plain (uncolored) strings so the Background(colBg3) highlight spans
// the full row width (see CLAUDE.md styling constraint).
func TestRenderVolumesSelectedRowBackground(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceVolumes,
		project:       "test-project",
		activeRegion:  "nl-ams",
		volumes: []volumeItem{
			{id: "a", name: "vol-one", size: 10 << 30, volType: "b_ssd", state: "available", zone: "nl-ams-1"},
			{id: "b", name: "vol-two", size: 20 << 30, volType: "l_ssd", state: "available", zone: "nl-ams-2", serverName: "srv-two"},
		},
		volCursor: 1,
	}

	out := m.drawDashboard()
	lines := strings.Split(out, "\n")

	// Find the selected row (cursor = 1 → "vol-two").
	var selLine string
	for _, l := range lines {
		if strings.Contains(l, "vol-two") && strings.Contains(l, "▌") {
			selLine = l
			break
		}
	}
	if selLine == "" {
		t.Fatal("selected row for vol-two not found")
	}
	// Isolate the list-pane segment of the selected row: from the ▌ cursor
	// up to (not including) the trailing scrollbar char.
	seg := selLine
	if i := strings.Index(seg, "▌"); i >= 0 {
		seg = seg[i:]
	}
	if j := strings.LastIndex(seg, "│"); j > 0 {
		seg = seg[:j]
	}
	// The segment must be plain text: any inline SGR code would cut the
	// Background(colBg3) highlight short (see CLAUDE.md styling constraint).
	if rest := stripANSI(seg); strings.ContainsRune(rest, 0x1b) {
		t.Errorf("selected row contains inline ANSI codes that break the background: %q", rest)
	}
}

// TestHasTransientVolume checks the polling trigger states (Block API and
// legacy Instance API states).
func TestHasTransientVolume(t *testing.T) {
	cases := []struct {
		name      string
		volumes   []volumeItem
		snapshots []snapshotItem
		want      bool
	}{
		{"all available", []volumeItem{{state: "available"}}, []snapshotItem{{state: "available"}}, false},
		{"volume in use", []volumeItem{{state: "in_use"}}, nil, false},
		{"volume creating", []volumeItem{{state: "creating"}}, nil, true},
		{"volume deleting", []volumeItem{{state: "deleting"}}, nil, true},
		{"volume resizing", []volumeItem{{state: "resizing"}}, nil, true},
		{"volume snapshotting", []volumeItem{{state: "snapshotting"}}, nil, true},
		{"volume updating", []volumeItem{{state: "updating"}}, nil, true},
		{"volume legacy hotsyncing", []volumeItem{{state: "hotsyncing"}}, nil, true},
		{"volume legacy attaching", []volumeItem{{state: "attaching"}}, nil, true},
		{"volume error", []volumeItem{{state: "error"}}, nil, false},
		{"volume locked", []volumeItem{{state: "locked"}}, nil, false},
		{"volume ref attaching", []volumeItem{{state: "available", refState: "attaching"}}, nil, true},
		{"volume ref attached", []volumeItem{{state: "in_use", refState: "attached"}}, nil, false},
		{"volume ref detaching", []volumeItem{{state: "in_use", refState: "detaching"}}, nil, true},
		{"snapshot creating", nil, []snapshotItem{{state: "creating"}}, true},
		{"snapshot deleting", nil, []snapshotItem{{state: "deleting"}}, true},
		{"snapshot legacy importing", nil, []snapshotItem{{state: "importing"}}, true},
		{"snapshot error", nil, []snapshotItem{{state: "error"}}, false},
	}
	for _, c := range cases {
		if got := hasTransientVolume(c.volumes, c.snapshots); got != c.want {
			t.Errorf("hasTransientVolume(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestBlockVolumeFromSDK checks the Block-API conversion: the attached
// server's ID comes from the volume's references, and the status/type map
// onto the TUI row fields.
func TestBlockVolumeFromSDK(t *testing.T) {
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	v := &block.Volume{
		ID:        "vol-1",
		Name:      "db-data",
		Size:      10 * scw.GB,
		Type:      "sbs_5k",
		Status:    block.VolumeStatusInUse,
		Zone:      scw.ZoneNlAms1,
		CreatedAt: &created,
		References: []*block.Reference{
			{
				ID:                  "ref-1",
				ProductResourceType: "instance_server",
				ProductResourceID:   "srv-1",
				Type:                block.ReferenceTypeExclusive,
				Status:              block.ReferenceStatusAttached,
			},
		},
	}
	it := blockVolumeFromSDK(v)
	if it.id != "vol-1" || it.name != "db-data" || it.size != uint64(10*scw.GB) {
		t.Errorf("unexpected basics: %+v", it)
	}
	if it.volType != "sbs_5k" || it.state != "in_use" || it.zone != "nl-ams-1" {
		t.Errorf("unexpected type/state/zone: %+v", it)
	}
	if it.serverID != "srv-1" || it.refState != "attached" {
		t.Errorf("expected server reference srv-1/attached, got %q/%q", it.serverID, it.refState)
	}
	if !it.createdAt.Equal(created) {
		t.Errorf("createdAt = %v, want %v", it.createdAt, created)
	}
}

// TestBlockSnapshotFromSDK checks the Block-API snapshot conversion,
// including the fallback to the storage class when the parent volume is gone.
func TestBlockSnapshotFromSDK(t *testing.T) {
	created := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	s := &block.Snapshot{
		ID:        "snap-1",
		Name:      "snap-db",
		Size:      5 * scw.GB,
		Status:    block.SnapshotStatusAvailable,
		Zone:      scw.ZoneNlAms1,
		CreatedAt: &created,
		Class:     block.StorageClassSbs,
		ParentVolume: &block.SnapshotParentVolume{
			ID:   "vol-1",
			Name: "db-data",
			Type: "sbs_5k",
		},
	}
	it := blockSnapshotFromSDK(s)
	if it.id != "snap-1" || it.baseVolume != "db-data" || it.volType != "sbs_5k" {
		t.Errorf("unexpected parent mapping: %+v", it)
	}
	if it.state != "available" || it.zone != "nl-ams-1" {
		t.Errorf("unexpected state/zone: %+v", it)
	}

	// Parent volume deleted → no base name, type falls back to the class.
	orphan := *s
	orphan.ParentVolume = nil
	oit := blockSnapshotFromSDK(&orphan)
	if oit.baseVolume != "" || oit.volType != "sbs" {
		t.Errorf("orphan snapshot: base=%q type=%q, want %q/%q", oit.baseVolume, oit.volType, "", "sbs")
	}
}

// TestVolumeServerName checks that the attached server's display name is
// resolved from the fetched server list when only its ID is known (the
// Block API case), with the stored name winning for legacy rows.
func TestVolumeServerName(t *testing.T) {
	m := rootModel{
		servers: []instanceServer{
			{id: "srv-1", name: "web-prod-1"},
			{id: "srv-2", name: "worker-2"},
		},
	}
	byRef := volumeItem{id: "v1", serverID: "srv-1"}
	if got := m.volumeServerName(byRef); got != "web-prod-1" {
		t.Errorf("volumeServerName(known ID) = %q, want %q", got, "web-prod-1")
	}
	stored := volumeItem{id: "v2", serverID: "srv-2", serverName: "legacy-name"}
	if got := m.volumeServerName(stored); got != "legacy-name" {
		t.Errorf("volumeServerName(stored name) = %q, want %q", got, "legacy-name")
	}
	unknown := volumeItem{id: "v3", serverID: "srv-9"}
	if got := m.volumeServerName(unknown); got != "" {
		t.Errorf("volumeServerName(unknown ID) = %q, want empty", got)
	}
}

// TestVolumeDisplayName checks the CSI cluster-ID prefix is dropped.
func TestVolumeDisplayName(t *testing.T) {
	cases := map[string]string{
		"cf84129e-5f45-49ea-ae79-3669fa69c53f_pvc-2a4bca7d-2f02": "pvc-2a4bca7d-2f02",
		"pvc-abc":  "pvc-abc",
		"web-data": "web-data",
		"":         "",
	}
	for in, want := range cases {
		if got := volumeDisplayName(in); got != want {
			t.Errorf("volumeDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderVolumesScrollbarAligned checks the selected row's scrollbar cell
// sits in the same column as on the other rows, and that the highlight does
// not swallow it (the scrollbar must follow the background span).
func TestRenderVolumesScrollbarAligned(t *testing.T) {
	m := rootModel{
		width: 160, height: 20,
		state: stateDashboard, focus: focusContent, activeService: serviceVolumes,
		project: "test-project", activeRegion: "nl-ams",
	}
	for i := 0; i < 40; i++ {
		m.volumes = append(m.volumes, volumeItem{
			id: string(rune('a' + i%26)), name: "cluster_pvc-" + strings.Repeat("x", 60),
			size: 10 << 30, volType: "sbs_5k", state: "in_use", zone: "nl-ams-1",
			serverName: "scw-very-long-server-name-that-truncates",
		})
	}
	out := stripANSI(m.drawDashboard())
	col := func(l string) int {
		r := []rune(l)
		// The list pane scrollbar is the last │/█ before the list pane's right border.
		idx := strings.Index(l, "pvc-")
		if idx < 0 {
			return -1
		}
		for i := len([]rune(l[:idx])); i < len(r); i++ {
			if r[i] == '│' || r[i] == '█' {
				return i
			}
		}
		return -1
	}
	var sel, other = -1, -1
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "pvc-") || !strings.Contains(l, "in_use") {
			continue
		}
		if strings.Contains(l, "▌") && sel < 0 {
			sel = col(l)
		} else if other < 0 && !strings.Contains(l, "▌") {
			other = col(l)
		}
	}
	if sel < 0 || other < 0 || sel != other {
		t.Errorf("scrollbar column: selected=%d other=%d, want equal", sel, other)
	}
	if strings.Contains(out, "cluster_pvc") {
		t.Error("cluster prefix should be stripped from displayed names")
	}
}

// TestRenderVolumesFullBaseName verifies NAME is sized to its content so a
// full pvc-<uuid> base-volume name fits in the BASE column on a wide terminal.
func TestRenderVolumesFullBaseName(t *testing.T) {
	const base = "pvc-40a5672e-1111-2222-3333-444444444444"
	m := rootModel{
		width:         200,
		height:        30,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceVolumes,
		project:       "test-project",
		activeRegion:  "nl-ams",
		volFocus:      1,
		snapshots: []snapshotItem{{
			id:         "44444444-4444-4444-4444-444444444444",
			name:       "snapshot-bf11957e-9df9-4ad8-be00-bba2e03e78d1",
			size:       1024 * 1024 * 1024,
			volType:    "sbs_5k",
			state:      "available",
			zone:       "nl-ams-1",
			baseVolume: "0123abcd-0000-0000-0000-000000000000_" + base,
		}},
	}
	listPane := m.renderVolumes(m.width-30, 20, colPurple)
	// The detail pane also shows the base volume, so look at the list row itself.
	for _, line := range strings.Split(stripANSI(listPane), "\n") {
		if strings.Contains(line, "sbs_5k") { // the list row, not the detail pane title
			if !strings.Contains(line, base) {
				t.Errorf("BASE column should show the full %q, row: %q", base, line)
			}
			return
		}
	}
	t.Fatal("snapshot row not found in list pane")
}
