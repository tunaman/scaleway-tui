package main

import (
	"strings"
	"testing"
)

// TestRenderDownloadProgress draws the download progress overlay with fake
// data and checks the title, the filename, and the multi-file counter.
func TestRenderDownloadProgress(t *testing.T) {
	m := rootModel{width: 160, height: 45}
	m.download.active = true
	m.download.filename = "report.pdf"
	m.download.bytesRead = 512 * 1024
	m.download.total = 2 * 1024 * 1024
	m.download.fileNum = 2
	m.download.fileTotal = 3

	out := m.renderDownloadProgress("base")

	for _, want := range []string{
		"DOWNLOADING",
		"report.pdf",
		"(2/3)",
		"25.0%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("download overlay output missing %q", want)
		}
	}
}

// TestBrowserStatusBarDownloadMsg checks that a completed-download summary is
// shown ahead of the hotkey list, and that the X hotkey is advertised.
func TestBrowserStatusBarDownloadMsg(t *testing.T) {
	m := rootModel{width: 200}
	m.downloadMsg = "Downloaded 3 file(s) → ~/Downloads"

	out := m.renderBrowserStatusBar()

	if !strings.Contains(out, "Downloaded 3 file(s) → ~/Downloads") {
		t.Error("status bar missing download summary")
	}
	if !strings.Contains(out, "Download") {
		t.Error("status bar missing X Download hotkey")
	}
}

// TestSafeRelPath verifies that hostile S3 keys cannot escape the download
// directory via ".." segments.
func TestSafeRelPath(t *testing.T) {
	cases := map[string]string{
		"a/b/c.txt":     "a/b/c.txt",
		"../etc/passwd": "etc/passwd",
		// ".." segments are dropped, never resolved — the result must stay
		// inside the download directory.
		"a/../../b.txt": "a/b.txt",
		"simple.txt":    "simple.txt",
		"a//b.txt":      "a/b.txt",
	}
	for in, want := range cases {
		if got := safeRelPath(in); got != want {
			t.Errorf("safeRelPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExpandHome verifies that a leading "~" resolves to the home directory
// and other paths pass through untouched.
func TestExpandHome(t *testing.T) {
	if got := expandHome("plain/path"); got != "plain/path" {
		t.Errorf("expandHome(plain/path) = %q, want unchanged", got)
	}
	got := expandHome("~/Downloads")
	if strings.HasPrefix(got, "~") || !strings.HasSuffix(got, "/Downloads") {
		t.Errorf("expandHome(~/Downloads) = %q, want absolute path under home", got)
	}
}
