package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// DashboardService provides analytics and statistics.
type DashboardService struct {
	q  *sqlc.Queries
	fs *FinancialStatementService
}

// NewDashboardService creates a DashboardService.
func NewDashboardService(q *sqlc.Queries, fs *FinancialStatementService) *DashboardService {
	return &DashboardService{q: q, fs: fs}
}

// DashboardStats holds summary statistics.
type DashboardStats struct {
	TotalReports    int64          `json:"total_reports"`
	ReportsByStatus map[string]int `json:"reports_by_status"`
	ComplianceScore *int           `json:"compliance_score"`
	SessionCount    int64          `json:"session_count"`
	BankReconCount  int64          `json:"bank_recon_count"`
	ReceiptCount    int64          `json:"receipt_count"`
	VendorCount     int64          `json:"vendor_count"`
	// Pending action counts for smart quick actions
	PendingApprovals       int64 `json:"pending_approvals"`
	UnmatchedTransactions  int64 `json:"unmatched_transactions"`
	LowConfidenceCount     int64 `json:"low_confidence_count"`
	DraftReports           int64 `json:"draft_reports"`
}

// GetStats returns dashboard statistics for a company.
// Partial failures for individual counts are logged but not fatal — the dashboard
// degrades gracefully by showing zeroes for unavailable metrics.
func (s *DashboardService) GetStats(ctx context.Context, companyID uuid.UUID) (*DashboardStats, error) {
	logErr := func(field string, err error) {
		if err != nil {
			slog.WarnContext(ctx, "dashboard stats: partial failure", "field", field, "error", err)
		}
	}

	totalReports, err := s.q.CountReportsByCompany(ctx, companyID)
	logErr("total_reports", err)
	sessionCount, err := s.q.CountReconciliationSessionsByCompany(ctx, companyID)
	logErr("session_count", err)
	bankReconCount, err := s.q.CountBankReconBatchesByCompany(ctx, companyID)
	logErr("bank_recon_count", err)
	receiptCount, err := s.q.CountReceiptBatchesByCompany(ctx, companyID)
	logErr("receipt_count", err)
	vendorCount, err := s.q.CountVendorsByCompany(ctx, companyID)
	logErr("vendor_count", err)
	pendingApprovals, err := s.q.CountPendingApprovals(ctx, companyID)
	logErr("pending_approvals", err)
	unmatchedTxns, err := s.q.CountUnmatchedTransactionsByCompany(ctx, companyID)
	logErr("unmatched_transactions", err)
	lowConfCount, err := s.q.CountLowConfidenceTransactionsByCompany(ctx, companyID)
	logErr("low_confidence_count", err)

	// Get reports by status
	reports, err := s.q.ListReportsByCompany(ctx, sqlc.ListReportsByCompanyParams{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	logErr("reports_by_status", err)

	byStatus := make(map[string]int)
	var latestScore *int
	for _, r := range reports {
		byStatus[r.Status]++
		if r.ComplianceScore != nil && latestScore == nil {
			score := int(*r.ComplianceScore)
			latestScore = &score
		}
	}

	return &DashboardStats{
		TotalReports:           totalReports,
		ReportsByStatus:        byStatus,
		ComplianceScore:        latestScore,
		SessionCount:           sessionCount,
		BankReconCount:         bankReconCount,
		ReceiptCount:           receiptCount,
		VendorCount:            vendorCount,
		PendingApprovals:       pendingApprovals,
		UnmatchedTransactions:  unmatchedTxns,
		LowConfidenceCount:     lowConfCount,
		DraftReports:           int64(byStatus["draft"]),
	}, nil
}

// FinancialSummary holds key financial metrics for the dashboard.
type FinancialSummary struct {
	// Current month P&L
	Revenue      decimal.Decimal `json:"revenue"`
	Expenses     decimal.Decimal `json:"expenses"`
	NetIncome    decimal.Decimal `json:"net_income"`
	GrossProfit  decimal.Decimal `json:"gross_profit"`
	// Balance sheet totals
	TotalAssets      decimal.Decimal `json:"total_assets"`
	TotalLiabilities decimal.Decimal `json:"total_liabilities"`
	TotalEquity      decimal.Decimal `json:"total_equity"`
	CashBalance      decimal.Decimal `json:"cash_balance"`
	// Receivables / Payables
	AccountsReceivable decimal.Decimal `json:"accounts_receivable"`
	AccountsPayable    decimal.Decimal `json:"accounts_payable"`
	// Monthly P&L trend (last 6 months)
	MonthlyPL []MonthlyPLPoint `json:"monthly_pl"`
}

// MonthlyPLPoint represents a single month's P&L data.
type MonthlyPLPoint struct {
	Month    string          `json:"month"`
	Revenue  decimal.Decimal `json:"revenue"`
	Expenses decimal.Decimal `json:"expenses"`
	Net      decimal.Decimal `json:"net"`
}

// GetFinancialSummary returns key financial metrics using the FinancialStatementService.
// Individual statement failures are logged but not fatal — the summary degrades gracefully.
func (s *DashboardService) GetFinancialSummary(ctx context.Context, companyID uuid.UUID) (*FinancialSummary, error) {
	now := time.Now()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	summary := &FinancialSummary{}

	// Current month P&L
	monthEnd := thisMonth.AddDate(0, 1, -1)
	is, err := s.fs.IncomeStatement(ctx, companyID, thisMonth, monthEnd)
	if err != nil {
		slog.WarnContext(ctx, "financial summary: income statement failed", "error", err)
	} else if is != nil {
		summary.Revenue = is.TotalRevenue
		summary.Expenses = is.TotalExpenses.Add(is.TotalCOGS)
		summary.NetIncome = is.NetIncome
		summary.GrossProfit = is.GrossProfit
	}

	// Balance sheet as of today
	bs, err := s.fs.BalanceSheet(ctx, companyID, now)
	if err != nil {
		slog.WarnContext(ctx, "financial summary: balance sheet failed", "error", err)
	} else if bs != nil {
		summary.TotalAssets = bs.TotalAssets
		summary.TotalLiabilities = bs.TotalLiabilities
		summary.TotalEquity = bs.TotalEquity

		// Extract cash and AR/AP from balance sheet groups
		for _, grp := range bs.Assets {
			for _, acct := range grp.Accounts {
				switch acct.SubType {
				case "cash":
					summary.CashBalance = summary.CashBalance.Add(acct.Balance)
				case "receivable":
					summary.AccountsReceivable = summary.AccountsReceivable.Add(acct.Balance)
				}
			}
		}
		for _, grp := range bs.Liabilities {
			for _, acct := range grp.Accounts {
				if acct.SubType == "payable" {
					summary.AccountsPayable = summary.AccountsPayable.Add(acct.Balance)
				}
			}
		}
	}

	// Monthly P&L trend for last 6 months
	for i := 5; i >= 0; i-- {
		mStart := thisMonth.AddDate(0, -i, 0)
		mEnd := mStart.AddDate(0, 1, -1)
		mis, err := s.fs.IncomeStatement(ctx, companyID, mStart, mEnd)
		if err != nil {
			slog.WarnContext(ctx, "financial summary: monthly P&L failed", "month", mStart.Format("2006-01"), "error", err)
			summary.MonthlyPL = append(summary.MonthlyPL, MonthlyPLPoint{
				Month: mStart.Format("2006-01"),
			})
			continue
		}
		summary.MonthlyPL = append(summary.MonthlyPL, MonthlyPLPoint{
			Month:    mStart.Format("2006-01"),
			Revenue:  mis.TotalRevenue,
			Expenses: mis.TotalExpenses.Add(mis.TotalCOGS),
			Net:      mis.NetIncome,
		})
	}

	return summary, nil
}

// PeriodComparison holds a field-by-field comparison between two periods.
type PeriodComparison struct {
	PeriodA    string           `json:"period_a"`
	PeriodB    string           `json:"period_b"`
	ReportType string           `json:"report_type"`
	HasReportA bool             `json:"has_report_a"`
	HasReportB bool             `json:"has_report_b"`
	Fields     []ComparisonField `json:"comparison"`
}

// ComparisonField holds a single field comparison.
type ComparisonField struct {
	Field     string   `json:"field"`
	PeriodA   *float64 `json:"period_a"`
	PeriodB   *float64 `json:"period_b"`
	Diff      *float64 `json:"diff"`
	PctChange *float64 `json:"pct_change"`
}

// ComparePeriods compares two periods' reports field by field.
func (s *DashboardService) ComparePeriods(ctx context.Context, companyID uuid.UUID, periodA, periodB, reportType string) (*PeriodComparison, error) {
	comparison := &PeriodComparison{
		PeriodA:    periodA,
		PeriodB:    periodB,
		ReportType: reportType,
	}

	reports, err := s.q.ListReportsByCompanyAndType(ctx, sqlc.ListReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
		Limit:      100,
		Offset:     0,
	})
	if err != nil {
		return comparison, nil
	}

	var dataA, dataB map[string]interface{}
	for _, r := range reports {
		if r.Period == periodA && len(r.CalculatedData) > 0 {
			_ = json.Unmarshal(r.CalculatedData, &dataA)
			comparison.HasReportA = true
		}
		if r.Period == periodB && len(r.CalculatedData) > 0 {
			_ = json.Unmarshal(r.CalculatedData, &dataB)
			comparison.HasReportB = true
		}
	}

	// Compare key fields — select field set based on form type
	fields := comparisonFieldsForType(reportType)

	for _, field := range fields {
		cf := ComparisonField{Field: field}
		if dataA != nil {
			if v := toDecimal(dataA[field]); !v.IsZero() {
				f := v.InexactFloat64()
				cf.PeriodA = &f
			}
		}
		if dataB != nil {
			if v := toDecimal(dataB[field]); !v.IsZero() {
				f := v.InexactFloat64()
				cf.PeriodB = &f
			}
		}
		if cf.PeriodA != nil || cf.PeriodB != nil {
			if cf.PeriodA != nil && cf.PeriodB != nil {
				diff := *cf.PeriodB - *cf.PeriodA
				cf.Diff = &diff
				if *cf.PeriodA != 0 {
					pct := diff / *cf.PeriodA * 100
					cf.PctChange = &pct
				}
			}
			comparison.Fields = append(comparison.Fields, cf)
		}
	}

	return comparison, nil
}

// comparisonFieldsForType returns the relevant comparison fields for a given BIR form type.
func comparisonFieldsForType(reportType string) []string {
	switch reportType {
	case "BIR_2550M", "BIR_2550Q":
		return []string{
			"vatable_sales", "sales_to_government", "zero_rated_sales", "exempt_sales",
			"total_sales", "output_vat", "output_vat_government",
			"total_output_vat", "input_vat_goods", "input_vat_capital",
			"input_vat_services", "input_vat_imports", "total_input_vat",
			"net_vat", "vat_payable", "net_vat_payable",
			"surcharge", "interest", "total_amount_due",
		}
	case "BIR_1601C":
		return []string{
			"total_compensation", "taxable_compensation", "non_taxable_compensation",
			"tax_withheld", "adjustment", "total_tax_remitted",
			"surcharge", "interest", "compromise", "total_amount_due",
		}
	case "BIR_0619E":
		return []string{
			"total_income_payments", "total_taxes_withheld", "adjustment",
			"tax_still_due", "surcharge", "interest", "compromise",
			"total_amount_due",
		}
	case "BIR_1701":
		return []string{
			"gross_sales_receipts", "cost_of_sales", "gross_income",
			"total_gross_income", "total_deductions", "net_taxable_income",
			"income_tax_due", "total_tax_credits", "tax_payable",
			"surcharge", "interest", "total_amount_due",
		}
	case "BIR_1702":
		return []string{
			"gross_income", "cost_of_sales", "gross_profit",
			"total_gross_income", "total_deductions", "net_taxable_income",
			"rcit_amount", "mcit_amount", "income_tax_due",
			"excess_mcit_prior", "total_tax_credits", "tax_payable",
			"total_amount_due",
		}
	default:
		// Legacy fallback for unrecognized types
		return []string{
			"vatable_sales", "sales_to_government", "zero_rated_sales", "exempt_sales",
			"total_sales", "output_vat", "total_output_vat", "total_input_vat",
			"net_vat", "amount_due", "total_compensation", "tax_withheld", "tax_due",
		}
	}
}

// MonthlyTrendPoint is a single month's report stats.
type MonthlyTrendPoint struct {
	Month        string `json:"month"`
	TotalReports int64  `json:"total_reports"`
	FiledCount   int64  `json:"filed_count"`
	DraftCount   int64  `json:"draft_count"`
}

// GetTrends returns monthly report counts for the last N months.
func (s *DashboardService) GetTrends(ctx context.Context, companyID uuid.UUID, months int) ([]MonthlyTrendPoint, error) {
	if months <= 0 {
		months = 6
	}

	rows, err := s.q.ListReportsByCompanyGroupedByMonth(ctx, sqlc.ListReportsByCompanyGroupedByMonthParams{
		CompanyID: companyID,
		Column2:   int32(months),
	})
	if err != nil {
		return nil, fmt.Errorf("query trends: %w", err)
	}

	result := make([]MonthlyTrendPoint, 0, len(rows))
	for _, r := range rows {
		result = append(result, MonthlyTrendPoint{
			Month:        r.Month,
			TotalReports: r.TotalReports,
			FiledCount:   r.FiledCount,
			DraftCount:   r.DraftCount,
		})
	}
	return result, nil
}

// ActivityItem represents a recent activity entry.
type ActivityItem struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	CreatedAt   string    `json:"created_at"`
}

// GetRecentActivity returns the most recent report activity.
func (s *DashboardService) GetRecentActivity(ctx context.Context, companyID uuid.UUID, limit int) ([]ActivityItem, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.q.ListRecentActivityByCompany(ctx, sqlc.ListRecentActivityByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("query activity: %w", err)
	}

	result := make([]ActivityItem, 0, len(rows))
	for _, r := range rows {
		desc, _ := r.Description.(string)
		result = append(result, ActivityItem{
			ID:          r.ID,
			Type:        r.Type,
			Description: desc,
			CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

// GetCompanyForDashboard returns company details from the company_members relationship.
func (s *DashboardService) GetCompanyForDashboard(ctx context.Context, companyID uuid.UUID) (map[string]interface{}, error) {
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("company not found: %w", err)
	}

	return map[string]interface{}{
		"id":                 company.ID,
		"company_name":       company.CompanyName,
		"tin_number":         company.TinNumber,
		"rdo_code":           company.RdoCode,
		"vat_classification": company.VatClassification,
		"plan":               company.Plan,
	}, nil
}
