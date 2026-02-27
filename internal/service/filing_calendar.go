package service

import (
	"fmt"
	"sort"
	"time"
)

// FilingEntry represents a single BIR filing deadline.
type FilingEntry struct {
	Form          string `json:"form"`
	Name          string `json:"name"`
	Period        string `json:"period"`
	Deadline      string `json:"deadline"`
	DaysRemaining int    `json:"days_remaining"`
	Status        string `json:"status"` // overdue, upcoming, scheduled
}

type filingRule struct {
	form      string
	name      string
	frequency string // monthly, quarterly, annual
	deadlineFunc func(year, month int) time.Time
}

var filingRules = []filingRule{
	{
		form: "BIR 2550M", name: "Monthly VAT Declaration", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 20, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 1601-C", name: "Monthly Withholding Tax (Compensation)", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 10, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 0619-E", name: "Monthly Expanded Withholding Tax", frequency: "monthly",
		deadlineFunc: func(y, m int) time.Time {
			next := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 10, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 2550Q", name: "Quarterly VAT Return", frequency: "quarterly",
		deadlineFunc: func(y, m int) time.Time {
			// Due 25th of month following quarter end
			qEnd := quarterEndMonth(m)
			next := time.Date(y, time.Month(qEnd), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 25, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 1601-EQ", name: "Quarterly Expanded Withholding Tax", frequency: "quarterly",
		deadlineFunc: func(y, m int) time.Time {
			qEnd := quarterEndMonth(m)
			next := time.Date(y, time.Month(qEnd), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
			return time.Date(next.Year(), next.Month(), 25, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 1702Q", name: "Quarterly Income Tax Return (Corp)", frequency: "quarterly",
		deadlineFunc: func(y, m int) time.Time {
			// 60 days after quarter end
			qEnd := quarterEndMonth(m)
			end := time.Date(y, time.Month(qEnd)+1, 0, 0, 0, 0, 0, time.Local)
			return end.AddDate(0, 0, 60)
		},
	},
	{
		form: "BIR 1701", name: "Annual Income Tax Return (Individual)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			return time.Date(y+1, time.April, 15, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 1702", name: "Annual Income Tax Return (Corp)", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			return time.Date(y+1, time.April, 15, 0, 0, 0, 0, time.Local)
		},
	},
	{
		form: "BIR 2316", name: "Certificate of Compensation Payment/Tax Withheld", frequency: "annual",
		deadlineFunc: func(y, _ int) time.Time {
			return time.Date(y+1, time.January, 31, 0, 0, 0, 0, time.Local)
		},
	},
}

// GenerateFilingCalendar generates upcoming filing deadlines.
// Routes to jurisdiction-specific calendar based on the jurisdiction parameter.
func GenerateFilingCalendar(year int, monthsAhead int, jurisdiction string) []FilingEntry {
	if jurisdiction == "SG" {
		entries := GenerateSGFilingCalendar(year, monthsAhead)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Deadline < entries[j].Deadline
		})
		return entries
	}

	// Default: Philippine (BIR) deadlines
	now := time.Now()
	endDate := now.AddDate(0, monthsAhead, 0)

	var entries []FilingEntry

	for _, rule := range filingRules {
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

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Deadline < entries[j].Deadline
	})

	return entries
}

func makeEntry(rule filingRule, period string, deadline time.Time, now time.Time) FilingEntry {
	days := int(deadline.Sub(now).Hours() / 24)

	status := "scheduled"
	if days < 0 {
		status = "overdue"
	} else if days <= 7 {
		status = "upcoming"
	}

	return FilingEntry{
		Form:          rule.form,
		Name:          rule.name,
		Period:        period,
		Deadline:      deadline.Format("2006-01-02"),
		DaysRemaining: days,
		Status:        status,
	}
}

func quarterEndMonth(m int) int {
	switch {
	case m <= 3:
		return 3
	case m <= 6:
		return 6
	case m <= 9:
		return 9
	default:
		return 12
	}
}
