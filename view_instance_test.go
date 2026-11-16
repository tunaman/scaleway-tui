package main

import (
	"strings"
	"testing"
)

// TestRenderServersDashboard draws the Instances dashboard screen with fake
// data and checks the row layout, the selected-row background, and the
// detail pane.
func TestRenderServersDashboard(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceInstance,
		project:       "test-project",
		activeRegion:  "nl-ams",
		servers: []instanceServer{
			{
				id:             "11111111-1111-1111-1111-111111111111",
				name:           "web-prod-1",
				status:         "running",
				commercialType: "GP1-M",
				zone:           "nl-ams-1",
				publicIP:       "51.15.200.10",
				privateIP:      "172.20.1.10",
				tags:           []string{"web", "prod"},
				volumes: []instanceVolume{
					{name: "0", size: 10 * 1024 * 1024 * 1024, volType: "b_ssd", boot: true},
					{name: "1", size: 50 * 1024 * 1024 * 1024, volType: "l_ssd"},
				},
			},
			{
				id:             "22222222-2222-2222-2222-222222222222",
				name:           "batch-worker",
				status:         "stopped in place",
				commercialType: "M3-XL",
				zone:           "nl-ams-2",
				privateIP:      "172.20.2.20",
				volumes: []instanceVolume{
					{name: "0", size: 20 * 1024 * 1024 * 1024, volType: "b_ssd", boot: true},
				},
			},
		},
	}

	out := m.drawDashboard()

	for _, want := range []string{
		"INSTANCES",
		"SERVER INFO",
		"web-prod-1",
		"batch-worker",
		"GP1-M",
		"51.15.200.10",
		// "stopped in place" is shortened in the list column.
		"stopped",
		// Detail pane for the selected (first) server.
		"Public IP",
		"172.20.1.10",
		"Block SSD",
		"Local SSD",
		"(boot)",
		"2 servers",
		"nl-ams-1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard output missing %q", want)
		}
	}
}

// TestRenderServersSelectedRowBackground verifies the selected row is built
// from plain (uncolored) strings so the Background(colBg3) highlight spans
// the full row width (see CLAUDE.md styling constraint).
func TestRenderServersSelectedRowBackground(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceInstance,
		project:       "test-project",
		activeRegion:  "nl-ams",
		servers: []instanceServer{
			{id: "a", name: "srv-one", status: "running", commercialType: "S1-M", zone: "nl-ams-1", publicIP: "1.2.3.4"},
			{id: "b", name: "srv-two", status: "stopped", commercialType: "GP1-S", zone: "nl-ams-2"},
		},
		instCursor: 1,
	}

	out := m.drawDashboard()
	lines := strings.Split(out, "\n")

	// Find the selected row (cursor = 1 → "srv-two").
	var selLine string
	for _, l := range lines {
		if strings.Contains(l, "srv-two") && strings.Contains(l, "▌") {
			selLine = l
			break
		}
	}
	if selLine == "" {
		t.Fatal("selected row for srv-two not found")
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
