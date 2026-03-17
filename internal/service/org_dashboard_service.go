package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// OrgDashboardService provides org-level analytics across multiple companies.
type OrgDashboardService struct {
	q   *sqlc.Queries
	fs  *FinancialStatementService
	cas *CASService
}

// NewOrgDashboardService creates an OrgDashboardService.
func NewOrgDashboardService(q *sqlc.Queries, fs *FinancialStatementService, cas *CASService) *OrgDashboardService {
	return &OrgDashboardService{q: q, fs: fs, cas: cas}
}

// OrgDashboardData holds the full org-level dashboard response.
type OrgDashboardData struct {
	Organization  OrgInfo          `json:"organization"`
	Companies     []CompanySummary `json:"companies"`
	TotalFinance  OrgFinanceTotals `json:"total_finance"`
	CompanyCount  int              `json:"company_count"`
	MemberCount   int              `json:"member_count"`
	ComplianceAll bool             `json:"compliance_all_pass"`
}

// OrgInfo holds basic org metadata for the dashboard.
type OrgInfo struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Plan         string    `json:"plan"`
	MaxCompanies int       `json:"max_companies"`
	MaxUsers     int       `json:"max_users"`
}

// CompanySummary holds per-company stats for the org dashboard.
type CompanySummary struct {
	ID           uuid.UUID `json:"id"`
	CompanyName  string    `json:"company_name"`
	TIN          string    `json:"tin_number"`
	Jurisdiction string    `json:"jurisdiction"`
	Plan         string    `json:"plan"`
	ReportCount  int64     `json:"report_count"`
	DraftCount   int64     `json:"draft_count"`
	VendorCount  int64     `json:"vendor_count"`
	LastCheckAt  *string   `json:"last_check_at"`
	CASPass      *bool     `json:"cas_pass"`
	// Financial snapshot
	Revenue   decimal.Decimal `json:"revenue"`
	Expenses  decimal.Decimal `json:"expenses"`
	NetIncome decimal.Decimal `json:"net_income"`
}

// OrgFinanceTotals holds aggregated financials across all org companies.
type OrgFinanceTotals struct {
	TotalRevenue   decimal.Decimal `json:"total_revenue"`
	TotalExpenses  decimal.Decimal `json:"total_expenses"`
	TotalNetIncome decimal.Decimal `json:"total_net_income"`
	TotalAssets    decimal.Decimal `json:"total_assets"`
}

// GetDashboard returns the full org-level dashboard data.
func (s *OrgDashboardService) GetDashboard(ctx context.Context, orgID uuid.UUID) (*OrgDashboardData, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("org not found: %w", err)
	}

	pgOrgID := pgtype.UUID{Bytes: orgID, Valid: true}

	companySummaries, err := s.q.GetOrgCompanySummaries(ctx, pgOrgID)
	if err != nil {
		return nil, fmt.Errorf("get company summaries: %w", err)
	}

	memberCount, err := s.q.CountOrgMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("count members: %w", err)
	}

	// Fetch financial snapshots in parallel (capped concurrency)
	type finResult struct {
		idx     int
		summary *FinancialSummary
	}

	results := make([]CompanySummary, len(companySummaries))
	var wg sync.WaitGroup
	finCh := make(chan finResult, len(companySummaries))

	for i, cs := range companySummaries {
		tin := ""
		if cs.TinNumber != nil {
			tin = *cs.TinNumber
		}
		results[i] = CompanySummary{
			ID:           cs.ID,
			CompanyName:  cs.CompanyName,
			TIN:          tin,
			Jurisdiction: cs.Jurisdiction,
			Plan:         cs.Plan,
			ReportCount:  cs.ReportCount,
			DraftCount:   cs.DraftCount,
			VendorCount:  cs.VendorCount,
		}

		if !cs.LastCheckAt.IsZero() {
			t := cs.LastCheckAt.Format("2006-01-02T15:04:05Z")
			results[i].LastCheckAt = &t
			pass := cs.OverallPass
			results[i].CASPass = &pass
		}

		wg.Add(1)
		go func(idx int, companyID uuid.UUID) {
			defer wg.Done()
			now := time.Now()
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			monthEnd := monthStart.AddDate(0, 1, -1)
			is, ferr := s.fs.IncomeStatement(ctx, companyID, monthStart, monthEnd)
			if ferr != nil {
				slog.WarnContext(ctx, "org dashboard: financial snapshot failed",
					"company_id", companyID, "error", ferr)
				return
			}
			if is == nil {
				return
			}
			finCh <- finResult{idx: idx, summary: &FinancialSummary{
				Revenue:   is.TotalRevenue,
				Expenses:  is.TotalExpenses.Add(is.TotalCOGS),
				NetIncome: is.NetIncome,
			}}
		}(i, cs.ID)
	}

	go func() {
		wg.Wait()
		close(finCh)
	}()

	totals := OrgFinanceTotals{}
	for fr := range finCh {
		results[fr.idx].Revenue = fr.summary.Revenue
		results[fr.idx].Expenses = fr.summary.Expenses
		results[fr.idx].NetIncome = fr.summary.NetIncome
		totals.TotalRevenue = totals.TotalRevenue.Add(fr.summary.Revenue)
		totals.TotalExpenses = totals.TotalExpenses.Add(fr.summary.Expenses)
		totals.TotalNetIncome = totals.TotalNetIncome.Add(fr.summary.NetIncome)
	}

	allPass := len(results) > 0
	for _, r := range results {
		if r.CASPass == nil || !*r.CASPass {
			allPass = false
			break
		}
	}

	return &OrgDashboardData{
		Organization: OrgInfo{
			ID:           org.ID,
			Name:         org.Name,
			Plan:         org.Plan,
			MaxCompanies: int(org.MaxCompanies),
			MaxUsers:     int(org.MaxUsers),
		},
		Companies:     results,
		TotalFinance:  totals,
		CompanyCount:  len(results),
		MemberCount:   int(memberCount),
		ComplianceAll: allPass,
	}, nil
}

// BatchComplianceResult holds the result of batch CAS compliance checks.
type BatchComplianceResult struct {
	Results []CompanyComplianceResult `json:"results"`
	AllPass bool                      `json:"all_pass"`
}

// CompanyComplianceResult holds one company's compliance check result.
type CompanyComplianceResult struct {
	CompanyID   uuid.UUID `json:"company_id"`
	CompanyName string    `json:"company_name"`
	Pass        bool      `json:"pass"`
	Score       int       `json:"score"`
	Error       string    `json:"error,omitempty"`
}

// BatchComplianceCheck runs CAS compliance checks for all companies in an org.
func (s *OrgDashboardService) BatchComplianceCheck(ctx context.Context, orgID, checkedBy uuid.UUID) (*BatchComplianceResult, error) {
	pgOrgID := pgtype.UUID{Bytes: orgID, Valid: true}
	companyIDs, err := s.q.GetOrgCompanyIDs(ctx, pgOrgID)
	if err != nil {
		return nil, fmt.Errorf("get company IDs: %w", err)
	}

	type checkResult struct {
		idx    int
		result CompanyComplianceResult
	}

	var wg sync.WaitGroup
	ch := make(chan checkResult, len(companyIDs))

	// Fetch company names for display
	summaries, _ := s.q.GetOrgCompanySummaries(ctx, pgOrgID)
	nameMap := make(map[uuid.UUID]string, len(summaries))
	for _, cs := range summaries {
		nameMap[cs.ID] = cs.CompanyName
	}

	for i, cid := range companyIDs {
		wg.Add(1)
		go func(idx int, companyID uuid.UUID) {
			defer wg.Done()
			result, cerr := s.cas.RunComplianceCheck(ctx, companyID, checkedBy)
			r := CompanyComplianceResult{
				CompanyID:   companyID,
				CompanyName: nameMap[companyID],
			}
			if cerr != nil {
				r.Error = cerr.Error()
			} else if result != nil {
				r.Pass = result.OverallPass
				checks := []bool{
					result.SequentialNumberingOk,
					result.HashChainIntact,
					result.DoubleEntryBalanced,
					result.PeriodsProperlyClosed,
					result.AuditTrailComplete,
					result.SubsidiaryLedgersOk,
				}
				passCount := 0
				for _, ok := range checks {
					if ok {
						passCount++
					}
				}
				r.Score = passCount * 100 / len(checks)
			}
			ch <- checkResult{idx: idx, result: r}
		}(i, cid)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make([]CompanyComplianceResult, len(companyIDs))
	allPass := len(companyIDs) > 0
	for cr := range ch {
		results[cr.idx] = cr.result
		if !cr.result.Pass {
			allPass = false
		}
	}

	return &BatchComplianceResult{
		Results: results,
		AllPass: allPass,
	}, nil
}
