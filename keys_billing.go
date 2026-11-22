package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Billing
// ─────────────────────────────────────────────

// handleBillingProjectOverlayKey handles the billing project picker overlay.
func (m rootModel) handleBillingProjectOverlayKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	total := len(m.projects) + 1 // 0=all, 1..n=project
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.billingProjectOverlay = false
	case "up", "k":
		if m.billingProjectCursor > 0 {
			m.billingProjectCursor--
		}
	case "down", "j":
		if m.billingProjectCursor < total-1 {
			m.billingProjectCursor++
		}
	case "enter":
		m.billingProjectOverlay = false
		if m.billingProjectCursor != m.billingProjectIdx {
			m.billingProjectIdx = m.billingProjectCursor
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
		}
	}
	return m, nil
}

// handleBillingExportOverlayKey handles the CSV export date-range picker.
func (m rootModel) handleBillingExportOverlayKey(msg tea.KeyPressMsg) (rootModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.billingExportOverlay = false
	case "tab":
		m.billingExportField = 1 - m.billingExportField
	case "left", "h":
		if m.billingExportField == 0 {
			m.billingExportFrom = prevMonth(m.billingExportFrom)
		} else {
			prev := prevMonth(m.billingExportTo)
			if prev >= m.billingExportFrom {
				m.billingExportTo = prev
			}
		}
	case "right", "l":
		now := time.Now().Format("2006-01")
		if m.billingExportField == 0 {
			next := nextMonth(m.billingExportFrom)
			if next <= m.billingExportTo {
				m.billingExportFrom = next
			}
		} else {
			next := nextMonth(m.billingExportTo)
			if next <= now {
				m.billingExportTo = next
			}
		}
	case "enter":
		m.billingExportOverlay = false
		m.loading = true
		m.billingExportMsg = ""
		return m, tea.Batch(m.spin.Tick, m.exportBillingCSV(m.billingExportFrom, m.billingExportTo))
	}
	return m, nil
}

// billingProjectKey opens the project picker (filter by project or all).
func (m rootModel) billingProjectKey() (rootModel, tea.Cmd) {
	if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading {
		m.billingProjectOverlay = true
		m.billingProjectCursor = m.billingProjectIdx
	}
	return m, nil
}

// billingExportKey opens the date-range picker and exports to CSV on confirm.
func (m rootModel) billingExportKey() (rootModel, tea.Cmd) {
	if m.state == stateDashboard && m.activeService == serviceBilling && !m.loading && !m.billingExportOverlay {
		m.billingExportFrom = time.Now().AddDate(0, -11, 0).Format("2006-01")
		m.billingExportTo = time.Now().Format("2006-01")
		m.billingExportField = 0
		m.billingExportOverlay = true
	}
	return m, nil
}

// billingPrevMonthKey / billingNextMonthKey navigate the billing period.
func (m rootModel) billingPrevMonthKey() (rootModel, tea.Cmd) {
	m.billingPeriod = prevMonth(m.billingPeriod)
	m.loading = true
	m.billingExportMsg = ""
	return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
}

func (m rootModel) billingNextMonthKey() (rootModel, tea.Cmd) {
	next := nextMonth(m.billingPeriod)
	// Don't navigate into the future beyond current month
	now := time.Now().Format("2006-01")
	if next <= now {
		m.billingPeriod = next
		m.loading = true
		m.billingExportMsg = ""
		return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
	}
	return m, nil
}

// billingDetailUp / billingDetailDown move the billing detail cursor.
func (m rootModel) billingDetailUp() (rootModel, tea.Cmd) {
	if m.billingCursor > 0 {
		m.billingCursor--
		if m.billingCursor < m.billingScrollY {
			m.billingScrollY = m.billingCursor
		}
	}
	return m, nil
}

func (m rootModel) billingDetailDown() (rootModel, tea.Cmd) {
	if m.billingCursor < len(m.billingDetail)-1 {
		m.billingCursor++
	}
	return m, nil
}

// billingEnterKey refreshes the billing overview.
func (m rootModel) billingEnterKey() (rootModel, tea.Cmd) {
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
}
