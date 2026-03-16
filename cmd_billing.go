package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	billing "github.com/scaleway/scaleway-sdk-go/api/billing/v2beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Billing — data fetching
// ─────────────────────────────────────────────

// fetchBillingOverview fetches the last 6 months of totals plus the detail
// rows for the given period (defaults to current month if empty).
func (m rootModel) fetchBillingOverview(period string) tea.Cmd {
	return func() tea.Msg {
		api := billing.NewAPI(m.scwClient)

		if period == "" {
			period = time.Now().Format("2006-01")
		}

		// ── Last 6 months of aggregated totals ──
		months := make([]billingMonth, 0, 6)
		for i := 5; i >= 0; i-- {
			p := time.Now().AddDate(0, -i, 0).Format("2006-01")
			bm := billingMonth{
				period:     p,
				byCategory: make(map[string]float64),
			}
			var page int32 = 1
			for {
				resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
					BillingPeriod: &p,
					Page:          scw.Int32Ptr(page),
				})
				if err != nil {
					break // billing perms may be restricted — skip silently
				}
				for _, c := range resp.Consumptions {
					v := moneyToFloat(c.Value)
					bm.totalExTax += v
					bm.byCategory[c.CategoryName] += v
				}
				if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
					break
				}
				page++
			}
			months = append(months, bm)
		}

		// ── Detail rows for the selected period ──
		detail, err := fetchConsumptionDetail(api, period)
		if err != nil {
			return errMsg{fmt.Errorf("billing detail: %w", err)}
		}

		return billingOverviewMsg{months: months, detail: detail, period: period}
	}
}

// fetchConsumptionDetail returns sorted consumption rows for a given period.
func fetchConsumptionDetail(api *billing.API, period string) ([]billingConsumptionRow, error) {
	var rows []billingConsumptionRow
	var page int32 = 1
	for {
		resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
			BillingPeriod: &period,
			Page:          scw.Int32Ptr(page),
		})
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Consumptions {
			rows = append(rows, billingConsumptionRow{
				category:    c.CategoryName,
				product:     c.ProductName,
				projectName: c.ProjectID, // resolved to name if possible
				valueEUR:    moneyToFloat(c.Value),
			})
		}
		if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
			break
		}
		page++
	}
	// Sort by value descending
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].valueEUR > rows[j].valueEUR
	})
	return rows, nil
}

// exportBillingCSV fetches the last N months and writes a pivot CSV to ~/scw-tui-export-YYYYMM.csv
func (m rootModel) exportBillingCSV(numMonths int) tea.Cmd {
	return func() tea.Msg {
		api := billing.NewAPI(m.scwClient)

		// Collect all periods
		periods := make([]string, numMonths)
		for i := 0; i < numMonths; i++ {
			periods[numMonths-1-i] = time.Now().AddDate(0, -i, 0).Format("2006-01")
		}

		// category → period → total
		type key struct{ category, period string }
		totals := make(map[key]float64)
		categories := make(map[string]bool)

		for _, p := range periods {
			var page int32 = 1
			period := p
			for {
				resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
					BillingPeriod: &period,
					Page:          scw.Int32Ptr(page),
				})
				if err != nil {
					break
				}
				for _, c := range resp.Consumptions {
					k := key{c.CategoryName, period}
					totals[k] += moneyToFloat(c.Value)
					categories[c.CategoryName] = true
				}
				if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
					break
				}
				page++
			}
		}

		// Sort categories
		cats := make([]string, 0, len(categories))
		for c := range categories {
			cats = append(cats, c)
		}
		sort.Strings(cats)

		// Build CSV
		home, err := os.UserHomeDir()
		if err != nil {
			return errMsg{fmt.Errorf("find home dir: %w", err)}
		}
		fname := fmt.Sprintf("scw-tui-export-%s.csv", time.Now().Format("200601"))
		path := filepath.Join(home, fname)
		f, err := os.Create(path)
		if err != nil {
			return errMsg{fmt.Errorf("create csv: %w", err)}
		}
		defer func() { _ = f.Close() }()

		w := csv.NewWriter(f)

		// Header: Category, Jan-2025, Feb-2025, ...
		header := []string{"Category"}
		for _, p := range periods {
			t, err := time.Parse("2006-01", p)
			if err != nil {
				header = append(header, p)
			} else {
				header = append(header, t.Format("Jan 2006"))
			}
		}
		header = append(header, "Total")
		if err := w.Write(header); err != nil {
			return errMsg{fmt.Errorf("write header: %w", err)}
		}

		// Rows
		for _, cat := range cats {
			row := []string{cat}
			var total float64
			for _, p := range periods {
				v := totals[key{cat, p}]
				total += v
				row = append(row, fmt.Sprintf("%.2f", v))
			}
			row = append(row, fmt.Sprintf("%.2f", total))
			if err := w.Write(row); err != nil {
				return errMsg{fmt.Errorf("write row: %w", err)}
			}
		}

		// Totals row
		totRow := []string{"TOTAL"}
		var grandTotal float64
		for _, p := range periods {
			var sum float64
			for _, cat := range cats {
				sum += totals[key{cat, p}]
			}
			grandTotal += sum
			totRow = append(totRow, fmt.Sprintf("%.2f", sum))
		}
		totRow = append(totRow, fmt.Sprintf("%.2f", grandTotal))
		if err := w.Write(totRow); err != nil {
			return errMsg{fmt.Errorf("write totals row: %w", err)}
		}

		w.Flush()
		if err := w.Error(); err != nil {
			return errMsg{fmt.Errorf("flush csv: %w", err)}
		}
		return billingExportDoneMsg{path: path}
	}
}
