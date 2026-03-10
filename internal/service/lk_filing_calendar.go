package service

import (
	"fmt"
	"time"

	"github.com/tonypk/aistarlight-go/pkg/lkforms"
)

var lkFilingRules = []filingRule{
	{
		form: lkforms.FormVATReturn, name: "VAT Return", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			// VAT return due on the 20th of the following month
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 20, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormPAYE, name: "PAYE / APIT Return", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			// APIT due on the 15th of the following month
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 15, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormWHT, name: "Withholding Tax Return", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			// WHT due on the 15th of the following month
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 15, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormSSCL, name: "Social Security Contribution Levy", frequency: "quarterly",
		deadlineFunc: func(y, m int) time.Time {
			// SSCL due 20th of month following quarter end
			qEnd := quarterEndMonth(m)
			next := time.Date(y, time.Month(qEnd), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 20, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormCIT, name: "Corporate Income Tax Return", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Annual CIT return due 30 Nov of following year (for March FY)
			return time.Date(y+1, time.November, 30, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormITReturn, name: "Individual Income Tax Return", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Individual return due 30 Nov of following year
			return time.Date(y+1, time.November, 30, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: lkforms.FormAPIT, name: "Annual APIT Statement", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			// Annual APIT statement due 30 Apr of following year
			return time.Date(y+1, time.April, 30, 0, 0, 0, 0, time.Local)
		},
	},
}

// GenerateLKFilingCalendar generates upcoming IRD Sri Lanka filing deadlines.
func GenerateLKFilingCalendar(year int, monthsAhead int) []FilingEntry {
	now := time.Now()
	endDate := now.AddDate(0, monthsAhead, 0)

	var entries []FilingEntry

	for _, rule := range lkFilingRules {
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
