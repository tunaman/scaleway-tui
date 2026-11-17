package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Volumes & snapshots — fetch + actions
// ─────────────────────────────────────────────

// volumeFromSDK converts an SDK Volume into the TUI row type.
func volumeFromSDK(v *instance.Volume) volumeItem {
	it := volumeItem{
		id:      v.ID,
		name:    v.Name,
		size:    uint64(v.Size),
		volType: string(v.VolumeType),
		state:   string(v.State),
		zone:    string(v.Zone),
	}
	if v.Server != nil {
		it.serverName = v.Server.Name
		it.serverID = v.Server.ID
	}
	if v.CreationDate != nil {
		it.createdAt = *v.CreationDate
	}
	return it
}

// snapshotFromSDK converts an SDK Snapshot into the TUI row type.
func snapshotFromSDK(s *instance.Snapshot) snapshotItem {
	it := snapshotItem{
		id:      s.ID,
		name:    s.Name,
		size:    uint64(s.Size),
		volType: string(s.VolumeType),
		state:   string(s.State),
		zone:    string(s.Zone),
	}
	if s.BaseVolume != nil {
		it.baseVolume = s.BaseVolume.Name
	}
	if s.CreationDate != nil {
		it.createdAt = *s.CreationDate
	}
	return it
}

// listZoneVolumes fetches the volumes and snapshots of one zone. Both lists
// are project-filtered like the server list.
func (m rootModel) listZoneVolumes(api *instance.API, zone string) ([]volumeItem, []snapshotItem, error) {
	var vols []volumeItem
	var snaps []snapshotItem
	var zerr error

	var volPage int32 = 1
	for {
		req := &instance.ListVolumesRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(volPage)}
		if m.projectID != "" {
			req.Project = &m.projectID
		}
		resp, err := api.ListVolumes(req)
		if err != nil {
			zerr = err
			break
		}
		for _, v := range resp.Volumes {
			vols = append(vols, volumeFromSDK(v))
		}
		if len(vols) >= int(resp.TotalCount) {
			break
		}
		volPage++
	}

	var snapPage int32 = 1
	for {
		req := &instance.ListSnapshotsRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(snapPage)}
		if m.projectID != "" {
			req.Project = &m.projectID
		}
		resp, err := api.ListSnapshots(req)
		if err != nil {
			zerr = err
			break
		}
		for _, s := range resp.Snapshots {
			snaps = append(snaps, snapshotFromSDK(s))
		}
		if len(snaps) >= int(resp.TotalCount) {
			break
		}
		snapPage++
	}

	if zerr != nil {
		return nil, nil, zerr
	}
	return vols, snaps, nil
}

// listVolumesAllZones fetches every volume and snapshot in the active region.
// A failing zone is skipped (a zone may be unavailable for this account); the
// first error is only surfaced when every request failed, so a single dead
// zone never hides the rest — same pattern as listServersAllZones.
func (m rootModel) listVolumesAllZones() ([]volumeItem, []snapshotItem, error) {
	instanceAPI := instance.NewAPI(m.scwClient)
	var volumes []volumeItem
	var snapshots []snapshotItem
	var firstErr error
	zonesOK := 0
	for _, zone := range regionZones(m.activeRegion) {
		zv, zs, err := m.listZoneVolumes(instanceAPI, zone)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		zonesOK++
		volumes = append(volumes, zv...)
		snapshots = append(snapshots, zs...)
	}
	if zonesOK == 0 && firstErr != nil {
		return nil, nil, firstErr
	}
	return volumes, snapshots, nil
}

// hasTransientVolume reports whether any volume or snapshot is in a state
// that resolves itself (snapshotting, attaching, resizing, ...). The list
// polls every few seconds while this is true.
func hasTransientVolume(volumes []volumeItem, snapshots []snapshotItem) bool {
	for _, v := range volumes {
		switch v.state {
		case "snapshotting", "fetching", "saving", "attaching", "resizing", "hotsyncing":
			return true
		}
	}
	for _, s := range snapshots {
		switch s.state {
		case "snapshotting", "importing", "exporting":
			return true
		}
	}
	return false
}

// fetchVolumes lists all volumes and snapshots in the active region.
func (m rootModel) fetchVolumes() tea.Cmd {
	return func() tea.Msg {
		volumes, snapshots, err := m.listVolumesAllZones()
		if err != nil {
			return errMsg{fmt.Errorf("list volumes: %w", err)}
		}
		return volumesMsg{volumes: volumes, snapshots: snapshots}
	}
}

// pollVolumesAfterDelay waits 3 seconds then fires a volumePollTickMsg,
// used to refresh while a snapshot is being taken or a volume is
// transitioning.
func pollVolumesAfterDelay() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(3 * time.Second)
		return volumePollTickMsg{}
	}
}

// createSnapshot takes a snapshot of a volume.
func (m rootModel) createSnapshot(vol volumeItem, name string) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		req := &instance.CreateSnapshotRequest{
			Zone:     scw.Zone(vol.zone),
			Name:     name,
			VolumeID: &vol.id,
		}
		if m.projectID != "" {
			req.Project = &m.projectID
		}
		if _, err := instanceAPI.CreateSnapshot(req); err != nil {
			return errMsg{fmt.Errorf("create snapshot: %w", err)}
		}
		return snapshotCreatedMsg{}
	}
}

// deleteSnapshot removes a snapshot.
func (m rootModel) deleteSnapshot(id, zone string) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		if err := instanceAPI.DeleteSnapshot(&instance.DeleteSnapshotRequest{
			Zone:       scw.Zone(zone),
			SnapshotID: id,
		}); err != nil {
			return errMsg{fmt.Errorf("delete snapshot: %w", err)}
		}
		return snapshotDeletedMsg{}
	}
}

// deleteVolume removes an unattached volume.
func (m rootModel) deleteVolume(id, zone string) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		if err := instanceAPI.DeleteVolume(&instance.DeleteVolumeRequest{
			Zone:     scw.Zone(zone),
			VolumeID: id,
		}); err != nil {
			return errMsg{fmt.Errorf("delete volume: %w", err)}
		}
		return volumeDeletedMsg{}
	}
}

// attachVolume attaches a volume to a server in the same zone.
func (m rootModel) attachVolume(vol volumeItem, serverID string) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		if _, err := instanceAPI.AttachServerVolume(&instance.AttachServerVolumeRequest{
			Zone:     scw.Zone(vol.zone),
			ServerID: serverID,
			VolumeID: vol.id,
		}); err != nil {
			return errMsg{fmt.Errorf("attach volume: %w", err)}
		}
		return volumeAttachedMsg{}
	}
}

// detachVolume detaches a volume from its server.
func (m rootModel) detachVolume(vol volumeItem) tea.Cmd {
	return func() tea.Msg {
		instanceAPI := instance.NewAPI(m.scwClient)
		if _, err := instanceAPI.DetachServerVolume(&instance.DetachServerVolumeRequest{
			Zone:     scw.Zone(vol.zone),
			ServerID: vol.serverID,
			VolumeID: vol.id,
		}); err != nil {
			return errMsg{fmt.Errorf("detach volume: %w", err)}
		}
		return volumeDetachedMsg{}
	}
}
