package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// SpendingAnalyticsService provides spending dashboard data.
type SpendingAnalyticsService struct {
	q *sqlc.Queries
}

// NewSpendingAnalyticsService creates a SpendingAnalyticsService.
func NewSpendingAnalyticsService(q *sqlc.Queries) *SpendingAnalyticsService {
	return &SpendingAnalyticsService{q: q}
}

// CategorySummary represents spending grouped by category.
type CategorySummary struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
	Total    string `json:"total"`
}

// VendorSummary represents spending grouped by vendor.
type VendorSummary struct {
	Vendor string `json:"vendor"`
	Count  int64  `json:"count"`
	Total  string `json:"total"`
}

// MonthSummary represents spending grouped by month.
type MonthSummary struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
	Total string `json:"total"`
}

// IncomeExpenseSummary represents income vs expense breakdown.
type IncomeExpenseSummary struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
	Total string `json:"total"`
}

// SpendingDashboard aggregates all spending analytics data.
type SpendingDashboard struct {
	ByCategory    []CategorySummary      `json:"by_category"`
	ByVendor      []VendorSummary        `json:"by_vendor"`
	ByMonth       []MonthSummary         `json:"by_month"`
	IncomeExpense []IncomeExpenseSummary  `json:"income_expense"`
	Period        SpendingPeriod         `json:"period"`
}

// SpendingPeriod describes the date range.
type SpendingPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GetDashboard returns aggregated spending data for a company and date range.
func (s *SpendingAnalyticsService) GetDashboard(ctx context.Context, companyID uuid.UUID, from, to time.Time) (*SpendingDashboard, error) {
	fromDate := pgtype.Date{Time: from, Valid: true}
	toDate := pgtype.Date{Time: to, Valid: true}

	catRows, err := s.q.GetSpendingSummaryByCategory(ctx, sqlc.GetSpendingSummaryByCategoryParams{
		CompanyID: companyID,
		Date:      fromDate,
		Date_2:    toDate,
	})
	if err != nil {
		return nil, err
	}

	vendorRows, err := s.q.GetSpendingSummaryByVendor(ctx, sqlc.GetSpendingSummaryByVendorParams{
		CompanyID: companyID,
		Date:      fromDate,
		Date_2:    toDate,
	})
	if err != nil {
		return nil, err
	}

	monthRows, err := s.q.GetSpendingSummaryByMonth(ctx, sqlc.GetSpendingSummaryByMonthParams{
		CompanyID: companyID,
		Date:      fromDate,
		Date_2:    toDate,
	})
	if err != nil {
		return nil, err
	}

	ieRows, err := s.q.GetIncomeExpenseSummary(ctx, sqlc.GetIncomeExpenseSummaryParams{
		CompanyID: companyID,
		Date:      fromDate,
		Date_2:    toDate,
	})
	if err != nil {
		return nil, err
	}

	dashboard := &SpendingDashboard{
		Period: SpendingPeriod{
			From: from.Format("2006-01-02"),
			To:   to.Format("2006-01-02"),
		},
	}

	for _, r := range catRows {
		dashboard.ByCategory = append(dashboard.ByCategory, CategorySummary{
			Category: r.Category,
			Count:    r.Count,
			Total:    r.Total,
		})
	}

	for _, r := range vendorRows {
		dashboard.ByVendor = append(dashboard.ByVendor, VendorSummary{
			Vendor: r.Vendor,
			Count:  r.Count,
			Total:  r.Total,
		})
	}

	for _, r := range monthRows {
		dashboard.ByMonth = append(dashboard.ByMonth, MonthSummary{
			Month: r.Month,
			Count: r.Count,
			Total: r.Total,
		})
	}

	for _, r := range ieRows {
		dashboard.IncomeExpense = append(dashboard.IncomeExpense, IncomeExpenseSummary{
			Type:  r.Type,
			Count: r.Count,
			Total: r.Total,
		})
	}

	// Ensure non-nil slices for JSON.
	if dashboard.ByCategory == nil {
		dashboard.ByCategory = []CategorySummary{}
	}
	if dashboard.ByVendor == nil {
		dashboard.ByVendor = []VendorSummary{}
	}
	if dashboard.ByMonth == nil {
		dashboard.ByMonth = []MonthSummary{}
	}
	if dashboard.IncomeExpense == nil {
		dashboard.IncomeExpense = []IncomeExpenseSummary{}
	}

	return dashboard, nil
}
