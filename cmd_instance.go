package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/ipam/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Instances (VMs) — fetch + power actions
// ─────────────────────────────────────────────

// regionZones derives the zone list for a region ("fr-par" → fr-par-1..3).
func regionZones(region string) []string {
	zones := make([]string, 0, 3)
	for i := 1; i <= 3; i++ {
		zones = append(zones, fmt.Sprintf("%s-%d", region, i))
	}
	return zones
}

// allServerStates are the states accepted by the ListServers API. The API has
// no "all" value, so every state is queried to get the full fleet.
var allServerStates = []instance.ServerState{
	instance.ServerStateRunning,
	instance.ServerStateStopped,
	instance.ServerStateStoppedInPlace,
	instance.ServerStateStarting,
	instance.ServerStateStopping,
	instance.ServerStateLocked,
}

// listServersAllZones fetches every server in the active region (all states).
// A failing zone/state is skipped (a zone may be unavailable for this
// account); the first error is only surfaced when every request failed, so a
// single dead zone never hides the rest of the fleet.
func (m rootModel) listServersAllZones() ([]instanceServer, error) {
	instanceAPI := instance.NewAPI(m.scwClient)
	var servers []instanceServer
	var firstErr error
	reqsOK := 0
	for _, zone := range regionZones(m.activeRegion) {
		for _, state := range allServerStates {
			var zoneServers []*instance.Server
			var page int32 = 1
			req := &instance.ListServersRequest{
				Zone:  scw.Zone(zone),
				State: &state,
			}
			if m.projectID != "" {
				req.Project = &m.projectID
			}
			ok := true
			for {
				resp, err := instanceAPI.ListServers(req)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					ok = false
					break // try the remaining zone/state combos
				}
				zoneServers = append(zoneServers, resp.Servers...)
				if uint32(len(zoneServers)) >= resp.TotalCount {
					break
				}
				page++
				req.Page = scw.Int32Ptr(page)
			}
			if ok {
				reqsOK++
			}
			for _, s := range zoneServers {
				servers = append(servers, serverFromSDK(s))
			}
		}
	}
	if reqsOK == 0 && firstErr != nil {
		return nil, firstErr
	}
	return servers, nil
}

// serverFromSDK converts an SDK Server into the TUI row type.
func serverFromSDK(s *instance.Server) instanceServer {
	pubIP := ""
	if len(s.PublicIPs) > 0 && s.PublicIPs[0] != nil {
		pubIP = s.PublicIPs[0].Address.String()
	}
	if pubIP == "" && s.PublicIP != nil { //nolint:staticcheck // fallback for pre-routed-IP servers
		pubIP = s.PublicIP.Address.String() //nolint:staticcheck
	}
	privIP := ""
	if s.PrivateIP != nil { //nolint:staticcheck // only non-nil on pre-routed-IP servers
		privIP = *s.PrivateIP //nolint:staticcheck
	}
	zone := ""
	if s.Location != nil {
		zone = s.Location.ZoneID
	}
	var created time.Time
	if s.CreationDate != nil {
		created = *s.CreationDate
	}

	// Routed-IP servers carry no private address on the NIC itself — the IPs
	// are referenced by IPAM ID and resolved lazily (fetchServerPrivateIP).
	var ipamIPIDs []string
	for _, nic := range s.PrivateNics {
		if nic != nil {
			ipamIPIDs = append(ipamIPIDs, nic.IpamIPIDs...)
		}
	}

	// Volumes arrive as a map keyed by slot index ("0", "1", ...); sort by
	// index so the boot volume (slot 0) comes first.
	type volSlot struct {
		idx int
		v   *instance.VolumeServer
	}
	var slots []volSlot
	for k, v := range s.Volumes {
		idx, _ := strconv.Atoi(k)
		slots = append(slots, volSlot{idx: idx, v: v})
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].idx < slots[j].idx })
	vols := make([]instanceVolume, 0, len(slots))
	for _, slot := range slots {
		if slot.v == nil {
			continue
		}
		vs := instanceVolume{
			name:    strconv.Itoa(slot.idx),
			volType: string(slot.v.VolumeType),
			boot:    slot.v.Boot,
		}
		if slot.v.Size != nil {
			vs.size = uint64(*slot.v.Size)
		}
		vols = append(vols, vs)
	}

	return instanceServer{
		id:             s.ID,
		name:           s.Name,
		status:         string(s.State),
		commercialType: s.CommercialType,
		zone:           zone,
		publicIP:       pubIP,
		privateIP:      privIP,
		ipamIPIDs:      ipamIPIDs,
		protected:      s.Protected,
		tags:           s.Tags,
		created:        created,
		volumes:        vols,
	}
}

// fetchServers lists all servers in the active region.
func (m rootModel) fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.listServersAllZones()
		if err != nil {
			return errMsg{fmt.Errorf("list servers: %w", err)}
		}
		return serversMsg{servers: servers}
	}
}

// pollServersAfterDelay waits 3 seconds then fires a serverPollTickMsg,
// used to refresh server status while any server is starting/stopping.
func pollServersAfterDelay() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(3 * time.Second)
		return serverPollTickMsg{}
	}
}

// fetchServerPrivateIP resolves a routed-IP server's private address by
// looking up its IPAM NIC IPs. The first IPv4 found wins. Best-effort: a
// lookup failure or no IPv4 yields an empty IP (the detail pane keeps "—").
func (m rootModel) fetchServerPrivateIP(server instanceServer) tea.Cmd {
	return func() tea.Msg {
		ipamAPI := ipam.NewAPI(m.scwClient)
		for _, id := range server.ipamIPIDs {
			ip, err := ipamAPI.GetIP(&ipam.GetIPRequest{IPID: id})
			if err != nil || ip == nil {
				continue
			}
			if ip.IsIPv6 {
				continue
			}
			if addr := ip.Address.IP.String(); addr != "" {
				return serverPrivateIPMsg{serverID: server.id, ip: addr}
			}
		}
		return serverPrivateIPMsg{serverID: server.id}
	}
}

// selectedServerPrivateIPCmd returns a cmd that resolves the private IP of
// the currently selected server (via IPAM), or nil when there is nothing to
// fetch (no NIC IPs, already cached, or already in flight). It marks the
// server as loading on the model it is called on.
func (m *rootModel) selectedServerPrivateIPCmd() tea.Cmd {
	s := m.selectedServer()
	if s == nil || len(s.ipamIPIDs) == 0 {
		return nil
	}
	if _, ok := m.privateIPs[s.id]; ok {
		return nil
	}
	if m.privateIPLoadingID == s.id {
		return nil
	}
	m.privateIPLoadingID = s.id
	return m.fetchServerPrivateIP(*s)
}

// instanceActionAPI maps the TUI confirm-action constants to SDK actions.
var instanceActionAPI = map[int]instance.ServerAction{
	instActionPowerOn:  instance.ServerActionPoweron,
	instActionPowerOff: instance.ServerActionPoweroff,
	instActionReboot:   instance.ServerActionReboot,
}

// powerServer runs a power action (poweron / poweroff / reboot) on a server.
func (m rootModel) powerServer(serverID, zone string, action instance.ServerAction) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		_, err := instanceAPI.ServerAction(&instance.ServerActionRequest{
			Zone:     scw.Zone(zone),
			ServerID: serverID,
			Action:   action,
		})
		if err != nil {
			return errMsg{fmt.Errorf("server action %s: %w", action, err)}
		}
		return serverActionDoneMsg{}
	}
}
