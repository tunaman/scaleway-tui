package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestHandleFilterKeyAppliesEdits drives every inline filter mode through
// type → backspace → enter → esc and checks the edits land on the model
// returned by Update. Regression: handleFilterKey had a value receiver, so it
// returned a stale copy and the filter could never be typed into or exited
// (the app looked hung — every key, incl. q/esc, was swallowed).
func TestHandleFilterKeyAppliesEdits(t *testing.T) {
	modes := []struct {
		name   string
		active func(*rootModel) *bool
		filter func(*rootModel) *string
	}{
		{"bucket", func(m *rootModel) *bool { return &m.bucketFiltering }, func(m *rootModel) *string { return &m.bucketFilter }},
		{"regBrowser", func(m *rootModel) *bool { return &m.regBrowserFiltering }, func(m *rootModel) *string { return &m.regBrowserFilter }},
		{"regTag", func(m *rootModel) *bool { return &m.regTagFiltering }, func(m *rootModel) *string { return &m.regTagFilter }},
		{"registry", func(m *rootModel) *bool { return &m.registryFiltering }, func(m *rootModel) *string { return &m.registryFilter }},
		{"secret", func(m *rootModel) *bool { return &m.secretFiltering }, func(m *rootModel) *string { return &m.secretFilter }},
		{"secBrowser", func(m *rootModel) *bool { return &m.secBrowserFiltering }, func(m *rootModel) *string { return &m.secBrowserFilter }},
		{"iam", func(m *rootModel) *bool { return &m.iamFiltering }, func(m *rootModel) *string { return &m.iamFilter }},
		{"instances", func(m *rootModel) *bool { return &m.instFiltering }, func(m *rootModel) *string { return &m.instFilter }},
		{"volumes", func(m *rootModel) *bool { return &m.volFiltering }, func(m *rootModel) *string { return &m.volFilter }},
	}
	press := func(m rootModel, k tea.KeyPressMsg) rootModel {
		res, _ := m.Update(k)
		return res.(rootModel)
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			m := rootModel{width: 160, height: 45, state: stateDashboard, focus: focusContent}
			*mode.active(&m) = true

			m = press(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
			m = press(m, tea.KeyPressMsg{Code: 'b', Text: "b"})
			if got := *mode.filter(&m); got != "ab" {
				t.Fatalf("after typing: filter = %q, want %q", got, "ab")
			}
			m = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
			if got := *mode.filter(&m); got != "a" {
				t.Fatalf("after backspace: filter = %q, want %q", got, "a")
			}
			m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if *mode.active(&m) {
				t.Fatal("enter did not leave filter mode")
			}
			if got := *mode.filter(&m); got != "a" {
				t.Fatalf("enter should keep the filter: got %q", got)
			}

			*mode.active(&m) = true
			m = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if *mode.active(&m) || *mode.filter(&m) != "" {
				t.Fatalf("esc did not clear filter mode: active=%v filter=%q", *mode.active(&m), *mode.filter(&m))
			}
		})
	}
}

// TestVolumesFilterResetsCursor checks the reset callback (bound to the
// caller's model) takes effect on the returned model.
func TestVolumesFilterResetsCursor(t *testing.T) {
	m := rootModel{width: 160, height: 45, state: stateDashboard, focus: focusContent, activeService: serviceVolumes,
		volumes:      []volumeItem{{id: "1", name: "web-data"}, {id: "2", name: "scratch"}, {id: "3", name: "web-logs"}},
		volFiltering: true, volCursor: 2, volScrollY: 1, volSnapCursor: 1}
	res, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = res.(rootModel)
	if m.volFilter != "w" || m.volCursor != 0 || m.volScrollY != 0 || m.volSnapCursor != 0 {
		t.Fatalf("filter=%q cursor=%d scroll=%d snapCursor=%d", m.volFilter, m.volCursor, m.volScrollY, m.volSnapCursor)
	}
	if n := len(m.filteredVolumes()); n != 2 {
		t.Fatalf("filteredVolumes = %d, want 2", n)
	}
}
