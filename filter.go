package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Filter helpers + maybeCalculateSize
// ─────────────────────────────────────────────

func (m rootModel) filteredRegistryNamespaces() []registryNamespace {
	if m.registryFilter == "" {
		return m.registryNamespaces
	}
	needle := strings.ToLower(m.registryFilter)
	var out []registryNamespace
	for _, ns := range m.registryNamespaces {
		if strings.Contains(strings.ToLower(ns.name), needle) {
			out = append(out, ns)
		}
	}
	return out
}

func (m rootModel) filteredRegistryImages() []registryImage {
	if m.regBrowserFilter == "" {
		return m.regBrowserImages
	}
	needle := strings.ToLower(m.regBrowserFilter)
	var out []registryImage
	for _, img := range m.regBrowserImages {
		if strings.Contains(strings.ToLower(img.name), needle) {
			out = append(out, img)
		}
	}
	return out
}

// filteredRegistryTags returns the tags of img filtered by the current regTagFilter.
func (m rootModel) filteredRegistryTags(img registryImage) []registryTag {
	if m.regTagFilter == "" {
		return img.tags
	}
	needle := strings.ToLower(m.regTagFilter)
	var out []registryTag
	for _, t := range img.tags {
		if strings.Contains(strings.ToLower(t.name), needle) {
			out = append(out, t)
		}
	}
	return out
}

// filteredBuckets returns the subset of m.buckets whose names contain the
// current filter string (case-insensitive). When the filter is empty the full
// slice is returned without allocating a new one.
func (m rootModel) filteredBuckets() []bucket {
	if m.bucketFilter == "" {
		return m.buckets
	}
	needle := strings.ToLower(m.bucketFilter)
	var out []bucket
	for _, b := range m.buckets {
		if strings.Contains(strings.ToLower(b.name), needle) {
			out = append(out, b)
		}
	}
	return out
}

func (m rootModel) filteredSecrets() []secretItem {
	if m.secretFilter == "" {
		return m.secrets
	}
	needle := strings.ToLower(m.secretFilter)
	var out []secretItem
	for _, s := range m.secrets {
		if strings.Contains(strings.ToLower(s.name), needle) {
			out = append(out, s)
		}
	}
	return out
}

func (m rootModel) filteredSecretVersions() []secretVersion {
	if m.secBrowserFilter == "" {
		return m.secBrowserVersions
	}
	needle := strings.ToLower(m.secBrowserFilter)
	var out []secretVersion
	for _, v := range m.secBrowserVersions {
		revStr := fmt.Sprintf("%d", v.revision)
		if strings.Contains(revStr, needle) ||
			strings.Contains(strings.ToLower(v.description), needle) ||
			strings.Contains(strings.ToLower(v.status), needle) {
			out = append(out, v)
		}
	}
	return out
}

// ─────────────────────────────────────────────
// IAM filter helpers (multi-field, read m.iamFilter)
// ─────────────────────────────────────────────

func (m rootModel) filteredIAMUsers() []iamUser {
	if m.iamFilter == "" {
		return m.iamUsers
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamUser
	for _, u := range m.iamUsers {
		if strings.Contains(strings.ToLower(u.email), needle) ||
			strings.Contains(strings.ToLower(u.userType), needle) ||
			strings.Contains(strings.ToLower(u.status), needle) {
			out = append(out, u)
		}
	}
	return out
}

func (m rootModel) filteredIAMApplications() []iamApplication {
	if m.iamFilter == "" {
		return m.iamApplications
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamApplication
	for _, a := range m.iamApplications {
		if strings.Contains(strings.ToLower(a.name), needle) ||
			strings.Contains(strings.ToLower(a.description), needle) {
			out = append(out, a)
		}
	}
	return out
}

func (m rootModel) filteredIAMGroups() []iamGroup {
	if m.iamFilter == "" {
		return m.iamGroups
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamGroup
	for _, g := range m.iamGroups {
		if strings.Contains(strings.ToLower(g.name), needle) ||
			strings.Contains(strings.ToLower(g.description), needle) {
			out = append(out, g)
		}
	}
	return out
}

func (m rootModel) filteredIAMPolicies() []iamPolicy {
	if m.iamFilter == "" {
		return m.iamPolicies
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamPolicy
	for _, p := range m.iamPolicies {
		if strings.Contains(strings.ToLower(p.name), needle) ||
			strings.Contains(strings.ToLower(p.principalKind), needle) ||
			strings.Contains(strings.ToLower(m.iamResolveName(p.principalID)), needle) {
			out = append(out, p)
		}
	}
	return out
}

func (m rootModel) filteredIAMAPIKeys() []iamAPIKey {
	if m.iamFilter == "" {
		return m.iamAPIKeys
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamAPIKey
	for _, k := range m.iamAPIKeys {
		if strings.Contains(strings.ToLower(k.accessKey), needle) ||
			strings.Contains(strings.ToLower(k.description), needle) ||
			strings.Contains(strings.ToLower(k.bearerKind), needle) ||
			strings.Contains(strings.ToLower(m.iamResolveName(k.bearerID)), needle) {
			out = append(out, k)
		}
	}
	return out
}

func (m rootModel) filteredIAMLogs() []iamLog {
	if m.iamFilter == "" {
		return m.iamLogs
	}
	needle := strings.ToLower(m.iamFilter)
	var out []iamLog
	for _, l := range m.iamLogs {
		if strings.Contains(strings.ToLower(l.action), needle) ||
			strings.Contains(strings.ToLower(l.resourceType), needle) ||
			strings.Contains(strings.ToLower(l.resourceID), needle) ||
			strings.Contains(strings.ToLower(m.iamResolveName(l.bearerID)), needle) {
			out = append(out, l)
		}
	}
	return out
}

func (m *rootModel) maybeCalculateSize() tea.Cmd {
	if m.activeService != serviceObjectStorage {
		return nil
	}
	fb := m.filteredBuckets()
	if len(fb) == 0 || m.bucketCursor >= len(fb) {
		return nil
	}
	if m.bucketCursor == m.prevBucketSel {
		return nil
	}
	m.prevBucketSel = m.bucketCursor
	return m.calculateSize()
}
