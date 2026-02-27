package service

import (
	"fmt"
	"time"

	"github.com/tonypk/aistarlight-go/pkg/irasforms"
)

var sgFilingRules = []filingRule{
	{
		form: irasforms.FormGSTF5, name: "GST Return (GST F5)", frequency: "quarterly",
		deadlineFunc: func(y, m int) time.Time {
			// GST F5 due 1 month after the end of the accounting period
			qEnd := quarterEndMonth(m)
			deadline := time.Date(y, time.Month(qEnd), 1, 0, 0, 0, 0, time.Local).AddDate(0, 2, -1)
			return deadline
		},
	},
	{
		form: irasforms.FormFormC, name: "Corporate Income Tax (Form C)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Form C due 30 Nov of following year (for Dec FY end)
			return time.Date(y+1, time.November, 30, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: irasforms.FormFormCS, name: "Simplified Corporate Tax (Form C-S)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Same deadline as Form C: 30 Nov of following year
			return time.Date(y+1, time.November, 30, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: irasforms.FormECI, name: "Estimated Chargeable Income (ECI)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// ECI due within 3 months after financial year end
			return time.Date(y+1, time.March, 31, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: irasforms.FormFormB, name: "Individual Income Tax (Form B)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Paper filing: 15 Apr; e-filing: 18 Apr — use 18 Apr
			return time.Date(y+1, time.April, 18, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: irasforms.FormIR8A, name: "Employer Return (IR8A)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// IR8A due by 1 Mar of following year
			return time.Date(y+1, time.March, 1, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: irasforms.FormS45, name: "Withholding Tax (S45)", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			// S45 due by 15th of 2nd month after payment date
			return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 2, 14)
		},
	},
}

// GenerateSGFilingCalendar generates upcoming IRAS filing deadlines for Singapore.
func GenerateSGFilingCalendar(year int, monthsAhead int) []FilingEntry {
	now := time.Now()
	endDate := now.AddDate(0, monthsAhead, 0)

	var entries []FilingEntry

	for _, rule := range sgFilingRules {
		switch rule.frequency {
		case "monthly":
			for m := 1; m <= 12; m++ {
				deadline := rule.deadlineFunc(year, m)
				if deadline.Before(now.AddDate(0, -1, 0)) || deadline.After(endDate) {
					continue
				}
				entries = append(entries, makeEntry(rule, fmt.Sprintf("%d-%02d", year, m), deadline, now))
			}
		case "quarterly":
			for _, qStart := range []int{1, 4, 7, 10} {
				deadline := rule.deadlineFunc(year, qStart)
				if deadline.Before(now.AddDate(0, -1, 0)) || deadline.After(endDate) {
					continue
				}
				q := (qStart + 2) / 3
				entries = append(entries, makeEntry(rule, fmt.Sprintf("%dQ%d", year, q), deadline, now))
			}
		case "annual":
			deadline := rule.deadlineFunc(year, 1)
			if !deadline.Before(now.AddDate(0, -1, 0)) && !deadline.After(endDate) {
				entries = append(entries, makeEntry(rule, fmt.Sprintf("%d", year), deadline, now))
			}
		}
	}

	return entries
}
