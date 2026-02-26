package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// DashboardService provides analytics and statistics.
type DashboardService struct {
	q *sqlc.Queries
}

// NewDashboardService creates a DashboardService.
func NewDashboardService(q *sqlc.Queries) *DashboardService {
	return &DashboardService{q: q}
}

// DashboardStats holds summary statistics.
type DashboardStats struct {
	TotalReports    int64          `json:"total_reports"`
	ReportsByStatus map[string]int `json:"reports_by_status"`
	ComplianceScore *int           `json:"compliance_score"`
	SessionCount    int64          `json:"session_count"`
	BankReconCount  int64          `json:"bank_recon_count"`
	ReceiptCount    int64          `json:"receipt_count"`
	SupplierCount   int64          `json:"supplier_count"`
}

// GetStats returns dashboard statistics for a company.
func (s *DashboardService) GetStats(ctx context.Context, companyID uuid.UUID) (*DashboardStats, error) {
	totalReports, _ := s.q.CountReportsByCompany(ctx, companyID)
	sessionCount, _ := s.q.CountReconciliationSessionsByCompany(ctx, companyID)
	bankReconCount, _ := s.q.CountBankReconBatchesByCompany(ctx, companyID)
	receiptCount, _ := s.q.CountReceiptBatchesByCompany(ctx, companyID)
	supplierCount, _ := s.q.CountSuppliersByCompany(ctx, companyID)

	// Get reports by status
	reports, _ := s.q.ListReportsByCompany(ctx, sqlc.ListReportsByCompanyParams{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})

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
		TotalReports:    totalReports,
		ReportsByStatus: byStatus,
		ComplianceScore: latestScore,
		SessionCount:    sessionCount,
		BankReconCount:  bankReconCount,
		ReceiptCount:    receiptCount,
		SupplierCount:   supplierCount,
	}, nil
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
