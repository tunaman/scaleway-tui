package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
