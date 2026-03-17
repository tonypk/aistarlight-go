package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	integration "github.com/tonypk/aistarlight-go/internal/domain/integration"
	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// PayrollTaxBridge auto-fills BIR tax drafts from payroll events.
type PayrollTaxBridge struct {
	q      *sqlc.Queries
	logger *slog.Logger
}

// NewPayrollTaxBridge creates a PayrollTaxBridge.
func NewPayrollTaxBridge(q *sqlc.Queries, logger *slog.Logger) *PayrollTaxBridge {
	return &PayrollTaxBridge{q: q, logger: logger}
}

// ProcessPayrollForTax generates 1601C and 2316 drafts from a payroll event.
func (b *PayrollTaxBridge) ProcessPayrollForTax(ctx context.Context, companyID uuid.UUID, evt *integration.PayrollRunCompletedEvent) error {
	if evt.Jurisdiction != "PH" {
		b.logger.Info("skipping tax bridge for non-PH jurisdiction", "jurisdiction", evt.Jurisdiction)
		return nil
	}

	if err := b.upsert1601CDraft(ctx, companyID, evt); err != nil {
		b.logger.Error("failed to upsert 1601C draft", "error", err)
		// Don't fail the whole event — journal was already created
	}

	if err := b.upsert2316YTD(ctx, companyID, evt); err != nil {
		b.logger.Error("failed to upsert 2316 YTD", "error", err)
	}

	return nil
}

// upsert1601CDraft creates/updates a BIR 1601C tax draft for the payroll month.
func (b *PayrollTaxBridge) upsert1601CDraft(ctx context.Context, companyID uuid.UUID, evt *integration.PayrollRunCompletedEvent) error {
	payDate, err := time.Parse("2006-01-02", evt.PayDate)
	if err != nil {
		return fmt.Errorf("parse pay_date: %w", err)
	}

	// 1601C is monthly — use the month of the period start
	periodStart, err := time.Parse("2006-01-02", evt.PeriodStart)
	if err != nil {
		return fmt.Errorf("parse period_start: %w", err)
	}
	monthStart := time.Date(periodStart.Year(), periodStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)

	// Aggregate payroll data for 1601C fields
	totalCompensation := decimal.Zero
	totalMinWage := decimal.Zero      // holiday pay, OT, NSD (non-taxable portion)
	total13thMonth := decimal.Zero     // bonus_pay mapped to 13th month
	totalSSS := decimal.Zero           // SSS + PhilHealth + PagIBIG employee contributions
	totalWithholding := decimal.Zero

	for _, emp := range evt.EmployeeLines {
		for _, earning := range emp.Earnings {
			amt, _ := decimal.NewFromString(earning.Amount)
			totalCompensation = totalCompensation.Add(amt)

			// Non-taxable categories
			switch earning.Code {
			case "holiday_pay", "night_diff":
				totalMinWage = totalMinWage.Add(amt)
			case "bonus_pay":
				total13thMonth = total13thMonth.Add(amt)
			}
		}
		for _, ded := range emp.Deductions {
			amt, _ := decimal.NewFromString(ded.Amount)
			switch ded.Code {
			case "sss_employee", "philhealth_employee", "pagibig_employee":
				totalSSS = totalSSS.Add(amt)
			}
		}
	}

	// Withholding tax from dedicated lines
	for _, wh := range evt.WithholdingLines {
		amt, _ := decimal.NewFromString(wh.TaxAmount)
		totalWithholding = totalWithholding.Add(amt)
	}

	// Check for existing draft for this month (may have data from earlier payroll run)
	existing, err := b.q.GetTaxDraftByPeriod(ctx, sqlc.GetTaxDraftByPeriodParams{
		CompanyID:   companyID,
		FormType:    birforms.FormBIR1601C,
		PeriodStart: pgtype.Date{Time: monthStart, Valid: true},
		PeriodEnd:   pgtype.Date{Time: monthEnd, Valid: true},
	})
	if err == nil && existing.Result != nil {
		// Merge with existing data (accumulate for semi-monthly payrolls)
		var prev map[string]string
		if json.Unmarshal(existing.Result, &prev) == nil {
			totalCompensation = totalCompensation.Add(toDecimal(prev["line_1_total_compensation"]))
			totalMinWage = totalMinWage.Add(toDecimal(prev["line_2_statutory_minimum_wage"]))
			total13thMonth = total13thMonth.Add(toDecimal(prev["line_3_nontaxable_13th_month"]))
			totalSSS = totalSSS.Add(toDecimal(prev["line_5_sss_gsis_phic_hdmf"]))
			totalWithholding = totalWithholding.Add(toDecimal(prev["line_9_tax_withheld"]))
		}
	}

	// Calculate using tax engine
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":     totalCompensation.String(),
			"statutory_minimum_wage": totalMinWage.String(),
			"nontaxable_13th_month":  total13thMonth.String(),
			"nontaxable_deminimis":   "0",
			"sss_gsis_phic_hdmf":     totalSSS.String(),
			"other_nontaxable":       "0",
			"tax_withheld":           totalWithholding.String(),
			"adjustment":             "0",
			"surcharge":              "0",
			"interest":               "0",
			"compromise":             "0",
		},
	}

	result, err := CalculateReport(birforms.FormBIR1601C, input)
	if err != nil {
		return fmt.Errorf("calculate 1601C: %w", err)
	}

	// Add metadata
	result["payroll_source"] = "true"
	result["pay_date"] = payDate.Format("2006-01-02")
	result["payroll_run_id"] = fmt.Sprintf("%d", evt.PayrollRunID)
	result["head_count"] = fmt.Sprintf("%d", evt.Totals.HeadCount)

	resultJSON, _ := json.Marshal(result)

	_, err = b.q.UpsertTaxDraft(ctx, sqlc.UpsertTaxDraftParams{
		CompanyID:   companyID,
		FormType:    birforms.FormBIR1601C,
		PeriodStart: pgtype.Date{Time: monthStart, Valid: true},
		PeriodEnd:   pgtype.Date{Time: monthEnd, Valid: true},
		Result:      resultJSON,
		TriggeredBy: strPtr("payroll_webhook"),
	})
	if err != nil {
		return fmt.Errorf("upsert tax draft: %w", err)
	}

	b.logger.Info("1601C draft upserted from payroll",
		"company_id", companyID,
		"period", monthStart.Format("2006-01"),
		"total_compensation", totalCompensation.String(),
		"tax_withheld", totalWithholding.String(),
	)
	return nil
}

// upsert2316YTD accumulates per-employee YTD data for annual BIR 2316.
func (b *PayrollTaxBridge) upsert2316YTD(ctx context.Context, companyID uuid.UUID, evt *integration.PayrollRunCompletedEvent) error {
	periodStart, err := time.Parse("2006-01-02", evt.PeriodStart)
	if err != nil {
		return fmt.Errorf("parse period_start: %w", err)
	}

	year := periodStart.Year()
	yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)

	// Load existing YTD data
	var ytdData map[string]EmployeeYTD
	existing, err := b.q.GetTaxDraftByPeriod(ctx, sqlc.GetTaxDraftByPeriodParams{
		CompanyID:   companyID,
		FormType:    birforms.FormBIR2316,
		PeriodStart: pgtype.Date{Time: yearStart, Valid: true},
		PeriodEnd:   pgtype.Date{Time: yearEnd, Valid: true},
	})
	if err == nil && existing.Result != nil {
		var wrapper YTDWrapper
		if json.Unmarshal(existing.Result, &wrapper) == nil {
			ytdData = wrapper.Employees
		}
	}
	if ytdData == nil {
		ytdData = make(map[string]EmployeeYTD)
	}

	// Accumulate this payroll run's data per employee
	for _, emp := range evt.EmployeeLines {
		key := fmt.Sprintf("%d", emp.EmployeeID)
		ytd := ytdData[key]
		ytd.EmployeeID = emp.EmployeeID
		ytd.EmployeeNo = emp.EmployeeNo
		ytd.FullName = emp.FullName

		for _, earning := range emp.Earnings {
			amt, _ := decimal.NewFromString(earning.Amount)
			switch earning.Code {
			case "basic_pay":
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			case "ot_pay":
				ytd.OvertimePay = ytd.OvertimePay.Add(amt)
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			case "holiday_pay":
				ytd.HolidayPay = ytd.HolidayPay.Add(amt)
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			case "night_diff":
				ytd.NightDifferential = ytd.NightDifferential.Add(amt)
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			case "bonus_pay":
				ytd.ThirteenthMonth = ytd.ThirteenthMonth.Add(amt)
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			default:
				ytd.GrossCompensation = ytd.GrossCompensation.Add(amt)
			}
		}

		for _, ded := range emp.Deductions {
			amt, _ := decimal.NewFromString(ded.Amount)
			switch ded.Code {
			case "sss_employee":
				ytd.SSSContrib = ytd.SSSContrib.Add(amt)
			case "philhealth_employee":
				ytd.PhilHealthContrib = ytd.PhilHealthContrib.Add(amt)
			case "pagibig_employee":
				ytd.PagIBIGContrib = ytd.PagIBIGContrib.Add(amt)
			}
		}

		ytdData[key] = ytd
	}

	// Add withholding tax per employee
	for _, wh := range evt.WithholdingLines {
		key := fmt.Sprintf("%d", wh.EmployeeID)
		ytd := ytdData[key]
		amt, _ := decimal.NewFromString(wh.TaxAmount)
		ytd.TaxWithheld = ytd.TaxWithheld.Add(amt)
		ytdData[key] = ytd
	}

	// Save as YTD wrapper
	wrapper := YTDWrapper{
		Year:           year,
		LastPayrollRun: evt.PayrollRunID,
		LastUpdated:    time.Now().UTC().Format(time.RFC3339),
		Employees:      ytdData,
	}

	resultJSON, _ := json.Marshal(wrapper)

	_, err = b.q.UpsertTaxDraft(ctx, sqlc.UpsertTaxDraftParams{
		CompanyID:   companyID,
		FormType:    birforms.FormBIR2316,
		PeriodStart: pgtype.Date{Time: yearStart, Valid: true},
		PeriodEnd:   pgtype.Date{Time: yearEnd, Valid: true},
		Result:      resultJSON,
		TriggeredBy: strPtr("payroll_webhook"),
	})
	if err != nil {
		return fmt.Errorf("upsert 2316 YTD: %w", err)
	}

	b.logger.Info("2316 YTD updated from payroll",
		"company_id", companyID,
		"year", year,
		"employees", len(ytdData),
		"payroll_run_id", evt.PayrollRunID,
	)
	return nil
}

// EmployeeYTD tracks year-to-date compensation data per employee for BIR 2316.
type EmployeeYTD struct {
	EmployeeID       int64           `json:"employee_id"`
	EmployeeNo       string          `json:"employee_no"`
	FullName         string          `json:"full_name"`
	GrossCompensation decimal.Decimal `json:"gross_compensation"`
	OvertimePay      decimal.Decimal `json:"overtime_pay"`
	HolidayPay       decimal.Decimal `json:"holiday_pay"`
	NightDifferential decimal.Decimal `json:"night_differential"`
	ThirteenthMonth  decimal.Decimal `json:"thirteenth_month"`
	SSSContrib       decimal.Decimal `json:"sss_contribution"`
	PhilHealthContrib decimal.Decimal `json:"philhealth_contribution"`
	PagIBIGContrib   decimal.Decimal `json:"pagibig_contribution"`
	TaxWithheld      decimal.Decimal `json:"tax_withheld"`
}

// YTDWrapper wraps per-employee YTD data stored in tax_drafts.result.
type YTDWrapper struct {
	Year           int                      `json:"year"`
	LastPayrollRun int64                    `json:"last_payroll_run"`
	LastUpdated    string                   `json:"last_updated"`
	Employees      map[string]EmployeeYTD   `json:"employees"`
}
