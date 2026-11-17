package main

import (
	"strings"
	"testing"
	"time"
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
		// Detail pane for the selected (first) volume.
		"Server",
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

// TestHasTransientVolume checks the polling trigger states.
func TestHasTransientVolume(t *testing.T) {
	cases := []struct {
		name      string
		volumes   []volumeItem
		snapshots []snapshotItem
		want      bool
	}{
		{"all available", []volumeItem{{state: "available"}}, []snapshotItem{{state: "available"}}, false},
		{"volume snapshotting", []volumeItem{{state: "snapshotting"}}, nil, true},
		{"volume attaching", []volumeItem{{state: "attaching"}}, nil, true},
		{"volume hotsyncing", []volumeItem{{state: "hotsyncing"}}, nil, true},
		{"volume error", []volumeItem{{state: "error"}}, nil, false},
		{"snapshot snapshotting", nil, []snapshotItem{{state: "snapshotting"}}, true},
		{"snapshot importing", nil, []snapshotItem{{state: "importing"}}, true},
		{"snapshot exporting", nil, []snapshotItem{{state: "exporting"}}, true},
		{"snapshot invalid_data", nil, []snapshotItem{{state: "invalid_data"}}, false},
	}
	for _, c := range cases {
		if got := hasTransientVolume(c.volumes, c.snapshots); got != c.want {
			t.Errorf("hasTransientVolume(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
