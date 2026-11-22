package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	block "github.com/scaleway/scaleway-sdk-go/api/block/v1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Volumes & snapshots — fetch + actions
// ─────────────────────────────────────────────
//
// Scaleway moved block storage out of the Instance API into the Block
// Storage API: the Instance ListVolumes endpoint no longer returns sbs_*
// volumes (it only serves local l_ssd / scratch volumes). The TUI therefore
// fetches from both APIs and merges the results:
//
//	Instance API: local (l_ssd) and scratch volumes + their snapshots
//	Block API:    sbs volumes + their snapshots
//
// Attaching/detaching still goes through the Instance API — per Scaleway's
// docs the Block API has no attach/detach endpoint.

// isBlockVolume reports whether a volume lives in the Block Storage API
// (sbs_* types) rather than the legacy Instance API.
func isBlockVolume(volType string) bool {
	return strings.HasPrefix(volType, "sbs")
}

// volumeFromSDK converts an Instance-API Volume into the TUI row type.
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

// blockVolumeFromSDK converts a Block-API Volume into the TUI row type. The
// Block API does not embed the attached server — only a reference carrying
// its ID — so the display name is resolved from the server list at render
// time (rootModel.volumeServerName).
func blockVolumeFromSDK(v *block.Volume) volumeItem {
	it := volumeItem{
		id:      v.ID,
		name:    v.Name,
		size:    uint64(v.Size),
		volType: v.Type,
		state:   string(v.Status),
		zone:    string(v.Zone),
	}
	for _, ref := range v.References {
		if ref.ProductResourceType == "instance_server" {
			it.serverID = ref.ProductResourceID
			it.refState = string(ref.Status)
			break
		}
	}
	if v.CreatedAt != nil {
		it.createdAt = *v.CreatedAt
	}
	return it
}

// snapshotFromSDK converts an Instance-API Snapshot into the TUI row type.
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

// blockSnapshotFromSDK converts a Block-API Snapshot into the TUI row type.
func blockSnapshotFromSDK(s *block.Snapshot) snapshotItem {
	it := snapshotItem{
		id:      s.ID,
		name:    s.Name,
		size:    uint64(s.Size),
		state:   string(s.Status),
		zone:    string(s.Zone),
	}
	if s.ParentVolume != nil {
		it.baseVolume = s.ParentVolume.Name
		it.volType = s.ParentVolume.Type
	}
	if it.volType == "" {
		// Parent volume already deleted: fall back to the storage class.
		it.volType = string(s.Class)
	}
	if s.CreatedAt != nil {
		it.createdAt = *s.CreatedAt
	}
	return it
}

// listInstanceZoneVolumes fetches one zone's local/scratch volumes from the
// Instance API (project-filtered, paginated).
func (m rootModel) listInstanceZoneVolumes(api *instance.API, zone string) ([]volumeItem, error) {
	var vols []volumeItem
	page := int32(1)
	for {
		req := &instance.ListVolumesRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(page)}
		if m.projectID != "" {
			req.Project = &m.projectID
		}
		resp, err := api.ListVolumes(req)
		if err != nil {
			return nil, err
		}
		if len(resp.Volumes) == 0 {
			break
		}
		for _, v := range resp.Volumes {
			vols = append(vols, volumeFromSDK(v))
		}
		if uint64(len(vols)) >= uint64(resp.TotalCount) {
			break
		}
		page++
	}
	return vols, nil
}

// listInstanceZoneSnapshots fetches one zone's local/scratch snapshots from
// the Instance API (project-filtered, paginated).
func (m rootModel) listInstanceZoneSnapshots(api *instance.API, zone string) ([]snapshotItem, error) {
	var snaps []snapshotItem
	page := int32(1)
	for {
		req := &instance.ListSnapshotsRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(page)}
		if m.projectID != "" {
			req.Project = &m.projectID
		}
		resp, err := api.ListSnapshots(req)
		if err != nil {
			return nil, err
		}
		if len(resp.Snapshots) == 0 {
			break
		}
		for _, s := range resp.Snapshots {
			snaps = append(snaps, snapshotFromSDK(s))
		}
		if len(snaps) >= int(resp.TotalCount) {
			break
		}
		page++
	}
	return snaps, nil
}

// listBlockZoneVolumes fetches one zone's sbs volumes from the Block Storage
// API (project-filtered, paginated).
func (m rootModel) listBlockZoneVolumes(api *block.API, zone string) ([]volumeItem, error) {
	var vols []volumeItem
	page := int32(1)
	for {
		req := &block.ListVolumesRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(page)}
		if m.projectID != "" {
			req.ProjectID = &m.projectID
		}
		resp, err := api.ListVolumes(req)
		if err != nil {
			return nil, err
		}
		if len(resp.Volumes) == 0 {
			break
		}
		for _, v := range resp.Volumes {
			vols = append(vols, blockVolumeFromSDK(v))
		}
		if uint64(len(vols)) >= resp.TotalCount {
			break
		}
		page++
	}
	return vols, nil
}

// listBlockZoneSnapshots fetches one zone's sbs snapshots from the Block
// Storage API (project-filtered, paginated).
func (m rootModel) listBlockZoneSnapshots(api *block.API, zone string) ([]snapshotItem, error) {
	var snaps []snapshotItem
	page := int32(1)
	for {
		req := &block.ListSnapshotsRequest{Zone: scw.Zone(zone), Page: scw.Int32Ptr(page)}
		if m.projectID != "" {
			req.ProjectID = &m.projectID
		}
		resp, err := api.ListSnapshots(req)
		if err != nil {
			return nil, err
		}
		if len(resp.Snapshots) == 0 {
			break
		}
		for _, s := range resp.Snapshots {
			snaps = append(snaps, blockSnapshotFromSDK(s))
		}
		if uint64(len(snaps)) >= resp.TotalCount {
			break
		}
		page++
	}
	return snaps, nil
}

// listZoneVolumes fetches one zone's volumes and snapshots from both the
// Instance API (local/scratch) and the Block API (sbs) and merges the two.
// A failing sub-fetch is collected but never discards the zone's other
// results; the caller decides what to do with the collected errors.
func (m rootModel) listZoneVolumes(zone string) ([]volumeItem, []snapshotItem, []error) {
	instanceAPI := instance.NewAPI(m.scwClient)
	blockAPI := block.NewAPI(m.scwClient)

	var vols []volumeItem
	var snaps []snapshotItem
	var errs []error

	if v, err := m.listInstanceZoneVolumes(instanceAPI, zone); err != nil {
		errs = append(errs, fmt.Errorf("instance volumes %s: %w", zone, err))
	} else {
		vols = append(vols, v...)
	}
	if s, err := m.listInstanceZoneSnapshots(instanceAPI, zone); err != nil {
		errs = append(errs, fmt.Errorf("instance snapshots %s: %w", zone, err))
	} else {
		snaps = append(snaps, s...)
	}
	if v, err := m.listBlockZoneVolumes(blockAPI, zone); err != nil {
		errs = append(errs, fmt.Errorf("block volumes %s: %w", zone, err))
	} else {
		vols = append(vols, v...)
	}
	if s, err := m.listBlockZoneSnapshots(blockAPI, zone); err != nil {
		errs = append(errs, fmt.Errorf("block snapshots %s: %w", zone, err))
	} else {
		snaps = append(snaps, s...)
	}
	return vols, snaps, errs
}

// listVolumesAllZones fetches every volume and snapshot in the active region
// (all zones × both APIs). Successful sub-fetches are always kept; the
// returned error (nil when everything succeeded) carries the failures of the
// sub-fetches that didn't, so a dead zone or a missing permission never
// silently hides the rest. On total failure it returns (nil, nil, err); a
// non-nil list is always authoritative data (possibly empty).
func (m rootModel) listVolumesAllZones() ([]volumeItem, []snapshotItem, error) {
	volumes := make([]volumeItem, 0, 64)
	snapshots := make([]snapshotItem, 0, 64)
	var errs []error
	for _, zone := range regionZones(string(m.region())) {
		zv, zs, zerrs := m.listZoneVolumes(zone)
		volumes = append(volumes, zv...)
		snapshots = append(snapshots, zs...)
		errs = append(errs, zerrs...)
	}
	if len(errs) == 0 {
		return volumes, snapshots, nil
	}
	// Nothing could be fetched at all: hand back nil so the caller keeps
	// the last known good lists.
	if len(volumes) == 0 && len(snapshots) == 0 {
		return nil, nil, errors.Join(errs...)
	}
	return volumes, snapshots, errors.Join(errs...)
}

// hasTransientVolume reports whether any volume or snapshot is in a state
// that resolves itself (creating, deleting, snapshotting, attaching, ...).
// The list polls every few seconds while this is true.
func hasTransientVolume(volumes []volumeItem, snapshots []snapshotItem) bool {
	for _, v := range volumes {
		switch v.state {
		// Block API
		case "creating", "deleting", "resizing", "snapshotting", "updating":
			return true
		// Instance API (local/scratch volumes)
		case "fetching", "saving", "attaching", "hotsyncing":
			return true
		}
		// Block volumes expose attach/detach on the server reference, not
		// as a volume state.
		if v.refState == "attaching" || v.refState == "detaching" {
			return true
		}
	}
	for _, s := range snapshots {
		switch s.state {
		// Block API
		case "creating", "deleting":
			return true
		// Instance API (local/scratch snapshots)
		case "snapshotting", "importing", "exporting":
			return true
		}
	}
	return false
}

// fetchVolumes lists all volumes and snapshots in the active region.
// Failures (even partial ones) ride along in volumesMsg so the pane can show
// a note instead of pretending the project has no volumes.
func (m rootModel) fetchVolumes() tea.Cmd {
	return func() tea.Msg {
		volumes, snapshots, err := m.listVolumesAllZones()
		return volumesMsg{volumes: volumes, snapshots: snapshots, err: err}
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

// createSnapshot takes a snapshot of a volume. sbs volumes go through the
// Block Storage API; legacy local/scratch volumes through the Instance API.
func (m rootModel) createSnapshot(vol volumeItem, name string) tea.Cmd {
	return func() tea.Msg {
		if isBlockVolume(vol.volType) {
			blockAPI := block.NewAPI(m.scwClient)
			req := &block.CreateSnapshotRequest{
				Zone:     scw.Zone(vol.zone),
				Name:     name,
				VolumeID: vol.id,
			}
			if m.projectID != "" {
				req.ProjectID = m.projectID
			}
			if _, err := blockAPI.CreateSnapshot(req); err != nil {
				return errMsg{fmt.Errorf("create snapshot: %w", err)}
			}
			return snapshotCreatedMsg{}
		}
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

// deleteSnapshot removes a snapshot (Block API for sbs, Instance API for
// legacy local/scratch snapshots).
func (m rootModel) deleteSnapshot(snap snapshotItem) tea.Cmd {
	return func() tea.Msg {
		if isBlockVolume(snap.volType) {
			blockAPI := block.NewAPI(m.scwClient)
			if err := blockAPI.DeleteSnapshot(&block.DeleteSnapshotRequest{
				Zone:       scw.Zone(snap.zone),
				SnapshotID: snap.id,
			}); err != nil {
				return errMsg{fmt.Errorf("delete snapshot: %w", err)}
			}
			return snapshotDeletedMsg{}
		}
		instanceAPI := instance.NewAPI(m.scwClient)
		if err := instanceAPI.DeleteSnapshot(&instance.DeleteSnapshotRequest{
			Zone:       scw.Zone(snap.zone),
			SnapshotID: snap.id,
		}); err != nil {
			return errMsg{fmt.Errorf("delete snapshot: %w", err)}
		}
		return snapshotDeletedMsg{}
	}
}

// deleteVolume removes a volume (Block API for sbs, Instance API for legacy
// local/scratch volumes).
func (m rootModel) deleteVolume(vol volumeItem) tea.Cmd {
	return func() tea.Msg {
		if isBlockVolume(vol.volType) {
			blockAPI := block.NewAPI(m.scwClient)
			if err := blockAPI.DeleteVolume(&block.DeleteVolumeRequest{
				Zone:     scw.Zone(vol.zone),
				VolumeID: vol.id,
			}); err != nil {
				return errMsg{fmt.Errorf("delete volume: %w", err)}
			}
			return volumeDeletedMsg{}
		}
		instanceAPI := instance.NewAPI(m.scwClient)
		if err := instanceAPI.DeleteVolume(&instance.DeleteVolumeRequest{
			Zone:     scw.Zone(vol.zone),
			VolumeID: vol.id,
		}); err != nil {
			return errMsg{fmt.Errorf("delete volume: %w", err)}
		}
		return volumeDeletedMsg{}
	}
}

// attachVolume attaches a volume to a server in the same zone. Attaching is
// only available through the Instance API — the Block API has no attach
// endpoint (Scaleway docs: "attaching a volume to an Instance must be done
// using the Instances API").
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

// detachVolume detaches a volume from its server (Instance API, see
// attachVolume).
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
