package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	orgID := m.organizationID
	defaultProjectID := m.projectID
	// Specific project filter (0 = all, 1..n = project index)
	filterProjectID := ""
	if m.billingProjectIdx > 0 && m.billingProjectIdx <= len(m.projects) {
		filterProjectID = m.projects[m.billingProjectIdx-1].id
	}
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
				req := &billing.ListConsumptionsRequest{
					BillingPeriod: &p,
					Page:          scw.Int32Ptr(page),
				}
				if filterProjectID != "" {
					req.ProjectID = &filterProjectID
				} else if orgID != "" {
					req.OrganizationID = &orgID
				} else if defaultProjectID != "" {
					req.ProjectID = &defaultProjectID
				}
				resp, err := api.ListConsumptions(req)
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
		detail, err := fetchConsumptionDetail(api, period, orgID, defaultProjectID, filterProjectID)
		if err != nil {
			return errMsg{fmt.Errorf("billing detail: %w", err)}
		}

		return billingOverviewMsg{months: months, detail: detail, period: period}
	}
}

// fetchConsumptionDetail returns sorted consumption rows for a given period.
func fetchConsumptionDetail(api *billing.API, period, orgID, defaultProjectID, filterProjectID string) ([]billingConsumptionRow, error) {
	var rows []billingConsumptionRow
	var page int32 = 1
	for {
		req := &billing.ListConsumptionsRequest{
			BillingPeriod: &period,
			Page:          scw.Int32Ptr(page),
		}
		if filterProjectID != "" {
			req.ProjectID = &filterProjectID
		} else if orgID != "" {
			req.OrganizationID = &orgID
		} else if defaultProjectID != "" {
			req.ProjectID = &defaultProjectID
		}
		resp, err := api.ListConsumptions(req)
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Consumptions {
			rows = append(rows, billingConsumptionRow{
				category: c.CategoryName,
				product:  c.ProductName,
				valueEUR: moneyToFloat(c.Value),
			})
		}
		if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
			break
		}
		page++
	}

	// Aggregate duplicate category+product combinations (same product across zones/resources)
	type rowKey struct{ category, product string }
	totals := make(map[rowKey]float64)
	var order []rowKey
	seen := make(map[rowKey]bool)
	for _, r := range rows {
		k := rowKey{r.category, r.product}
		totals[k] += r.valueEUR
		if !seen[k] {
			order = append(order, k)
			seen[k] = true
		}
	}
	aggregated := make([]billingConsumptionRow, 0, len(order))
	for _, k := range order {
		aggregated = append(aggregated, billingConsumptionRow{
			category: k.category,
			product:  k.product,
			valueEUR: totals[k],
		})
	}

	// Sort by category alphabetically, then by value descending within each category
	sort.Slice(aggregated, func(i, j int) bool {
		if aggregated[i].category != aggregated[j].category {
			return aggregated[i].category < aggregated[j].category
		}
		return aggregated[i].valueEUR > aggregated[j].valueEUR
	})
	return aggregated, nil
}

// exportBillingCSV fetches billing data for [from..to] and writes a CSV to ~/
func (m rootModel) exportBillingCSV(from, to string) tea.Cmd {
	orgID := m.organizationID
	defaultProjectID := m.projectID
	filterProjectID := ""
	projectName := "all"
	if m.billingProjectIdx > 0 && m.billingProjectIdx <= len(m.projects) {
		p := m.projects[m.billingProjectIdx-1]
		filterProjectID = p.id
		projectName = p.name
	}
	return func() tea.Msg {
		api := billing.NewAPI(m.scwClient)

		// Build ordered list of periods from → to inclusive.
		var periods []string
		for cur := from; cur <= to; cur = nextMonth(cur) {
			periods = append(periods, cur)
			if len(periods) > 36 { // safety cap
				break
			}
		}

		// category → period → total
		type key struct{ category, period string }
		totals := make(map[key]float64)
		categories := make(map[string]bool)

		for _, p := range periods {
			var page int32 = 1
			period := p
			for {
				req := &billing.ListConsumptionsRequest{
					BillingPeriod: &period,
					Page:          scw.Int32Ptr(page),
				}
				if filterProjectID != "" {
					req.ProjectID = &filterProjectID
				} else if orgID != "" {
					req.OrganizationID = &orgID
				} else if defaultProjectID != "" {
					req.ProjectID = &defaultProjectID
				}
				resp, err := api.ListConsumptions(req)
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

		// Build CSV file
		home, err := os.UserHomeDir()
		if err != nil {
			return errMsg{fmt.Errorf("find home dir: %w", err)}
		}
		safeName := strings.NewReplacer(" ", "-", "/", "-").Replace(strings.ToLower(projectName))
		fname := fmt.Sprintf("scw-tui-export-%s-%s-%s.csv", safeName, from, to)
		path := filepath.Join(home, fname)
		f, err := os.Create(path)
		if err != nil {
			return errMsg{fmt.Errorf("create csv: %w", err)}
		}
		defer func() { _ = f.Close() }()

		w := csv.NewWriter(f)

		// Metadata rows
		if err := w.Write([]string{"Project", projectName}); err != nil {
			return errMsg{fmt.Errorf("write metadata: %w", err)}
		}
		if err := w.Write([]string{"Period", from + " to " + to}); err != nil {
			return errMsg{fmt.Errorf("write metadata: %w", err)}
		}
		if err := w.Write(nil); err != nil { // blank separator row
			return errMsg{fmt.Errorf("write separator: %w", err)}
		}

		// Column header: Category, Jan 2025, ..., Total
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

		// Data rows
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

		// Grand total row
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
