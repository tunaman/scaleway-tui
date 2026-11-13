package main

import (
	"strings"
	"testing"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
)

// TestServerFromSDKPrivateNics verifies routed-IP servers collect their IPAM
// NIC IDs (across all NICs) so the private address can be resolved later.
func TestServerFromSDKPrivateNics(t *testing.T) {
	s := &instance.Server{
		ID:   "srv-1",
		Name: "web-1",
		PrivateNics: []*instance.PrivateNIC{
			{IpamIPIDs: []string{"ipam-a", "ipam-b"}},
			{IpamIPIDs: []string{"ipam-c"}},
			{}, // a NIC with no IPs contributes nothing
		},
	}
	got := serverFromSDK(s)
	want := []string{"ipam-a", "ipam-b", "ipam-c"}
	if len(got.ipamIPIDs) != len(want) {
		t.Fatalf("ipamIPIDs = %v, want %v", got.ipamIPIDs, want)
	}
	for i := range want {
		if got.ipamIPIDs[i] != want[i] {
			t.Fatalf("ipamIPIDs = %v, want %v", got.ipamIPIDs, want)
		}
	}
}

// TestSelectedServerPrivateIPCmd verifies the fetch is deduplicated: it fires
// once for an unresolved server, then not again (cached or in flight).
func TestSelectedServerPrivateIPCmdDedup(t *testing.T) {
	m := rootModel{
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceInstance,
		servers:       []instanceServer{{id: "a", name: "srv", ipamIPIDs: []string{"ipam-a"}}},
	}
	if cmd := m.selectedServerPrivateIPCmd(); cmd == nil {
		t.Fatal("expected a fetch cmd for an unresolved server")
	}
	if m.privateIPLoadingID != "a" {
		t.Fatalf("privateIPLoadingID = %q, want %q", m.privateIPLoadingID, "a")
	}
	// In flight → no second cmd.
	if cmd := m.selectedServerPrivateIPCmd(); cmd != nil {
		t.Fatal("expected nil while a fetch is in flight")
	}
	// Cached → no cmd.
	m.privateIPLoadingID = ""
	m.privateIPs = map[string]string{"a": "10.0.0.5"}
	if cmd := m.selectedServerPrivateIPCmd(); cmd != nil {
		t.Fatal("expected nil for a cached server")
	}
	// No NIC IPs → no cmd.
	m.servers[0].ipamIPIDs = nil
	m.privateIPs = nil
	if cmd := m.selectedServerPrivateIPCmd(); cmd != nil {
		t.Fatal("expected nil for a server with no IPAM NIC IDs")
	}
}

// TestRenderServerDetailPrivateIP verifies the detail pane shows the
// IPAM-resolved private IP and a "Resolving…" placeholder while loading.
func TestRenderServerDetailPrivateIP(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceInstance,
		project:       "test-project",
		activeRegion:  "nl-ams",
		servers: []instanceServer{
			{id: "a", name: "srv-one", status: "running", commercialType: "S1-M", zone: "nl-ams-1", ipamIPIDs: []string{"ipam-a"}},
		},
	}

	m.privateIPLoadingID = "a"
	if out := m.drawDashboard(); !strings.Contains(out, "Resolving…") {
		t.Errorf("expected 'Resolving…' while loading, got:\n%s", out)
	}

	m.privateIPLoadingID = ""
	m.privateIPs = map[string]string{"a": "10.1.2.3"}
	if out := m.drawDashboard(); !strings.Contains(out, "10.1.2.3") {
		t.Errorf("expected resolved private IP 10.1.2.3, got:\n%s", out)
	}
}
