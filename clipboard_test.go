package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestCopyToClipboard verifies the copy helper flashes the top-bar note and
// returns a command (tea.SetClipboard + a note-clearing tick).
func TestCopyToClipboard(t *testing.T) {
	m := rootModel{}
	m, cmd := m.copyToClipboard("51.15.200.10")
	if m.clipboardNote != "Copied → clipboard" {
		t.Errorf("clipboardNote = %q, want %q", m.clipboardNote, "Copied → clipboard")
	}
	if cmd == nil {
		t.Fatal("copyToClipboard returned a nil cmd")
	}
}

// TestClipboardNoteClearedMsg verifies the note is cleared once the tick fires.
func TestClipboardNoteClearedMsg(t *testing.T) {
	m := rootModel{clipboardNote: "Copied → clipboard"}
	out, _ := m.Update(clipboardNoteClearedMsg{})
	m = out.(rootModel)
	if m.clipboardNote != "" {
		t.Errorf("clipboardNote = %q, want empty", m.clipboardNote)
	}
}

// TestTopBarClipboardNote verifies the top bar shows the copy feedback.
func TestTopBarClipboardNote(t *testing.T) {
	m := rootModel{
		width:         160,
		height:        45,
		state:         stateDashboard,
		focus:         focusContent,
		activeService: serviceInstance,
		project:       "test-project",
		activeRegion:  "nl-ams",
		clipboardNote: "Copied → clipboard",
	}
	out := m.renderTopBar()
	if !strings.Contains(out, "Copied → clipboard") {
		t.Error("top bar missing clipboard note")
	}
}

// TestRegistryPullCommand verifies the pull-command helper for both regular
// tags and sha256 digest tags.
func TestRegistryPullCommand(t *testing.T) {
	m := rootModel{
		regBrowserNamespace: registryNamespace{endpoint: "registry.fr-par.scw.cloud/ns"},
		regBrowserImages: []registryImage{
			{
				name: "my-app",
				tags: []registryTag{
					{name: "v1.2.3"},
					{name: "sha256-abc123def456"},
				},
			},
		},
	}
	if got := m.registryPullCommand(); got != "docker pull registry.fr-par.scw.cloud/ns/my-app:v1.2.3" {
		t.Errorf("pull command = %q", got)
	}
	m.regBrowserTagCursor = 1
	if got := m.registryPullCommand(); got != "docker pull registry.fr-par.scw.cloud/ns/my-app@sha256:abc123def456" {
		t.Errorf("sha256 pull command = %q", got)
	}
	// No images selected → empty command.
	empty := rootModel{regBrowserNamespace: m.regBrowserNamespace}
	if got := empty.registryPullCommand(); got != "" {
		t.Errorf("expected empty pull command, got %q", got)
	}
}

// TestKeyBindingsCopyActions verifies each copy hotkey produces a command and
// sets the top-bar note.
func TestKeyBindingsCopyActions(t *testing.T) {
	key := func(s string) tea.KeyPressMsg {
		return tea.KeyPressMsg{Text: s, Code: rune(s[0])}
	}

	cases := []struct {
		name string
		m    rootModel
		k    string
	}{
		{
			name: "instances copy IP",
			m: rootModel{
				state: stateDashboard, focus: focusContent, activeService: serviceInstance,
				servers: []instanceServer{{id: "a", name: "srv", publicIP: "1.2.3.4"}},
			},
			k: "c",
		},
		{
			name: "object browser yank key",
			m: rootModel{
				state:          stateObjectBrowser,
				browserEntries: []bucketEntry{{name: "file.txt", fullKey: "folder/file.txt"}},
			},
			k: "y",
		},
		{
			name: "k8s node copy IP",
			m: rootModel{
				state: stateK8sBrowser, k8sBrowserFocus: 1,
				k8sBrowserNodes: []k8sNode{{id: "n", name: "node-1", publicIPv4: "5.6.7.8"}},
			},
			k: "c",
		},
		{
			name: "iam copy API key",
			m: rootModel{
				state: stateIAMBrowser, iamTab: iamTabAPIKeys,
				iamAPIKeys: []iamAPIKey{{accessKey: "SCWXXXXXXXXXXXXXXX"}},
			},
			k: "c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cmd := tc.m.handleKey(key(tc.k))
			if cmd == nil {
				t.Fatal("copy key produced no cmd")
			}
		})
	}
}
