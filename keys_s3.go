package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Object Storage (dashboard + object browser)
// ─────────────────────────────────────────────

// handleBucketFilterKey handles keys while the bucket list filter is active.
func (m rootModel) handleBucketFilterKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.bucketFiltering = false
		m.bucketFilter = ""
		m.bucketCursor = 0
		m.bucketScrollY = 0
	case "enter":
		// Lock in the first match and exit filter mode.
		m.bucketFiltering = false
		m.bucketCursor = 0
		m.bucketScrollY = 0
	case "backspace", "ctrl+h":
		if len([]rune(m.bucketFilter)) > 0 {
			runes := []rune(m.bucketFilter)
			m.bucketFilter = string(runes[:len(runes)-1])
		} else {
			// Empty filter + backspace → exit filter mode.
			m.bucketFiltering = false
		}
		m.bucketCursor = 0
		m.bucketScrollY = 0
	case "up":
		return m.handleUp()
	case "down":
		return m.handleDown()
	default:
		if msg.Text != "" {
			m.bucketFilter += msg.Text
			m.bucketCursor = 0
			m.bucketScrollY = 0
		}
	}
	return m, nil
}

// bucketStartFilterKey begins inline filtering of the bucket list.
func (m rootModel) bucketStartFilterKey() (rootModel, tea.Cmd) {
	m.bucketFiltering = true
	m.bucketFilter = ""
	m.bucketCursor = 0
	m.bucketScrollY = 0
	return m, nil
}

// bucketListUp / bucketListDown move the bucket cursor and refresh the
// selected bucket's size.
func (m rootModel) bucketListUp() (rootModel, tea.Cmd) {
	fb := m.filteredBuckets()
	if len(fb) == 0 {
		return m, nil
	}
	if m.bucketCursor > 0 {
		m.bucketCursor--
		m.bucketScrollX = 0
		if m.bucketCursor < m.bucketScrollY {
			m.bucketScrollY = m.bucketCursor
		}
		return m, m.maybeCalculateSize()
	}
	return m, nil
}

func (m rootModel) bucketListDown() (rootModel, tea.Cmd) {
	fb := m.filteredBuckets()
	if len(fb) == 0 {
		return m, nil
	}
	if m.bucketCursor < len(fb)-1 {
		m.bucketCursor++
		m.bucketScrollX = 0
		return m, m.maybeCalculateSize()
	}
	return m, nil
}

// bucketEnterKey opens the selected bucket.
func (m rootModel) bucketEnterKey() (rootModel, tea.Cmd) {
	fb := m.filteredBuckets()
	if len(fb) == 0 {
		return m, nil
	}
	b := fb[m.bucketCursor]
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(b.name, ""))
}

// s3Up / s3Down move the object browser cursor.
func (m rootModel) s3Up() (rootModel, tea.Cmd) {
	if m.browserCursor > 0 {
		m.browserCursor--
		m.browserScrollX = 0
		if m.browserCursor < m.browserScrollY {
			m.browserScrollY = m.browserCursor
		}
	}
	return m, nil
}

func (m rootModel) s3Down() (rootModel, tea.Cmd) {
	if m.browserCursor < len(m.browserEntries)-1 {
		m.browserCursor++
		m.browserScrollX = 0
	}
	return m, nil
}

// s3EnterKey descends into the folder under the cursor.
func (m rootModel) s3EnterKey() (rootModel, tea.Cmd) {
	entry := m.browserEntries[m.browserCursor]
	if entry.isDir {
		// Descend into the folder.
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, entry.fullKey))
	}
	// File selected — no action for now (could open detail overlay later).
	return m, nil
}

// s3Esc goes up one folder level, or back to the bucket list at the root.
func (m rootModel) s3Esc() (rootModel, tea.Cmd) {
	if m.browserPrefix == "" {
		// At root of bucket — go back to the bucket list.
		m.state = stateDashboard
	} else {
		// Go up one level.
		parent := parentPrefix(m.browserPrefix)
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, parent))
	}
	return m, nil
}

// s3CreateBucketInput / s3CreateFolderInput open the matching create dialog.
func (m rootModel) s3CreateBucketInput() (rootModel, tea.Cmd) {
	m.input.active = true
	m.input.mode = inputModeBucket
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	return m, nil
}

func (m rootModel) s3CreateFolderInput() (rootModel, tea.Cmd) {
	m.input.active = true
	m.input.mode = inputModeFolder
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	return m, nil
}

// s3UploadInput opens the upload file dialog.
func (m rootModel) s3UploadInput() (rootModel, tea.Cmd) {
	m.input.active = true
	m.input.mode = inputModeUpload
	m.input.value = ""
	m.input.cursor = 0
	m.input.errStr = ""
	return m, nil
}

// s3DeleteKey opens the delete confirmation for the selected entries.
func (m rootModel) s3DeleteKey() (rootModel, tea.Cmd) {
	var targets []bucketEntry
	if len(m.browserSelected) > 0 {
		for _, e := range m.browserEntries {
			if m.browserSelected[e.fullKey] {
				targets = append(targets, e)
			}
		}
	} else if len(m.browserEntries) > 0 {
		targets = []bucketEntry{m.browserEntries[m.browserCursor]}
	}
	if len(targets) > 0 {
		m.confirmItems = targets
		m.showConfirm = true
	}
	return m, nil
}

// s3DeleteConfirmYes / s3DeleteConfirmNo handle the S3 delete confirmation.
func (m rootModel) s3DeleteConfirmYes() (rootModel, tea.Cmd) {
	items := m.confirmItems
	bucket := m.browserBucket
	prefix := m.browserPrefix
	m.showConfirm = false
	m.confirmItems = nil
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.deleteEntries(bucket, prefix, items))
}

func (m rootModel) s3DeleteConfirmNo() (rootModel, tea.Cmd) {
	m.showConfirm = false
	m.confirmItems = nil
	return m, nil
}

// s3YankKey copies the selected entry's full S3 key.
func (m rootModel) s3YankKey() (rootModel, tea.Cmd) {
	if len(m.browserEntries) > 0 {
		return m.copyToClipboard(m.browserEntries[m.browserCursor].fullKey)
	}
	return m, nil
}

// s3DownloadKey starts a download of the selected file(s), or the file under
// the cursor when nothing is selected (asks for a destination directory).
func (m rootModel) s3DownloadKey() (rootModel, tea.Cmd) {
	var targets []bucketEntry
	if len(m.browserSelected) > 0 {
		for _, e := range m.browserEntries {
			if !e.isDir && m.browserSelected[e.fullKey] {
				targets = append(targets, e)
			}
		}
	} else if len(m.browserEntries) > 0 {
		e := m.browserEntries[m.browserCursor]
		if !e.isDir {
			targets = []bucketEntry{e}
		}
	}
	if len(targets) > 0 {
		m.downloadTargets = targets
		m.input.active = true
		m.input.mode = inputModeDownload
		m.input.value = ""
		m.input.cursor = 0
		m.input.errStr = ""
	}
	return m, nil
}

// s3SelectToggleKey toggles selection of the entry under the cursor.
func (m rootModel) s3SelectToggleKey() (rootModel, tea.Cmd) {
	if len(m.browserEntries) == 0 {
		return m, nil
	}
	key := m.browserEntries[m.browserCursor].fullKey
	if m.browserSelected == nil {
		m.browserSelected = make(map[string]bool)
	}
	m.browserSelected[key] = !m.browserSelected[key]
	if m.browserCursor < len(m.browserEntries)-1 {
		m.browserCursor++
	}
	return m, nil
}

// s3SelectAllKey toggles select-all for the visible entries.
func (m rootModel) s3SelectAllKey() (rootModel, tea.Cmd) {
	if len(m.browserEntries) == 0 {
		return m, nil
	}
	if m.browserSelected == nil {
		m.browserSelected = make(map[string]bool)
	}
	allSelected := len(m.browserSelected) == len(m.browserEntries)
	for _, e := range m.browserEntries {
		if allSelected {
			delete(m.browserSelected, e.fullKey)
		} else {
			m.browserSelected[e.fullKey] = true
		}
	}
	return m, nil
}
