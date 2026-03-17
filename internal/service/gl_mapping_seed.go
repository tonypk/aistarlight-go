package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// DefaultMapping defines a seed mapping entry.
type DefaultMapping struct {
	AccountNumber   string
	AccountName     string
	AccountType     string
	NormalBalance   string
	SourceDimension string
	SourceValue     string
	DebitCredit     string
}

// PHDefaultMappings returns the default GL mappings for Philippines jurisdiction.
func PHDefaultMappings() []DefaultMapping {
	return []DefaultMapping{
		// Earnings → Expense accounts (Debit)
		{"6100", "Salary Expense", "expense", "debit", "earning", "basic_pay", "debit"},
		{"6110", "Overtime Expense", "expense", "debit", "earning", "ot_pay", "debit"},
		{"6120", "Holiday Pay Expense", "expense", "debit", "earning", "holiday_pay", "debit"},
		{"6130", "Night Differential Expense", "expense", "debit", "earning", "night_diff", "debit"},
		{"6140", "Bonus Expense", "expense", "debit", "earning", "bonus_pay", "debit"},

		// Employee deductions → Payable accounts (Credit)
		{"2210", "SSS Payable", "liability", "credit", "deduction", "sss_employee", "credit"},
		{"2220", "PhilHealth Payable", "liability", "credit", "deduction", "philhealth_employee", "credit"},
		{"2230", "Pag-IBIG Payable", "liability", "credit", "deduction", "pagibig_employee", "credit"},
		{"2240", "Withholding Tax Payable", "liability", "credit", "deduction", "withholding_tax", "credit"},

		// Employer contributions → Expense accounts (Debit)
		{"6200", "SSS Employer Expense", "expense", "debit", "contribution", "sss_employer", "debit"},
		{"6210", "PhilHealth Employer Expense", "expense", "debit", "contribution", "philhealth_employer", "debit"},
		{"6220", "Pag-IBIG Employer Expense", "expense", "debit", "contribution", "pagibig_employer", "debit"},

		// Net pay → Cash/Bank (Credit)
		{"1010", "Cash - Payroll Bank", "asset", "debit", "net_pay", "cash", "credit"},
	}
}

// SeedResult contains the outcome of a seed operation.
type SeedResult struct {
	AccountsCreated int `json:"accounts_created"`
	AccountsExisted int `json:"accounts_existed"`
	MappingsCreated int `json:"mappings_created"`
	MappingsExisted int `json:"mappings_existed"`
}

// SeedDefaults creates default accounts and GL mappings for a company+jurisdiction.
func (s *GLMappingService) SeedDefaults(ctx context.Context, companyID uuid.UUID, jurisdiction string) (*SeedResult, error) {
	var defaults []DefaultMapping
	switch jurisdiction {
	case "PH":
		defaults = PHDefaultMappings()
	default:
		return nil, fmt.Errorf("no default mappings for jurisdiction %s", jurisdiction)
	}

	result := &SeedResult{}

	for _, d := range defaults {
		// Ensure account exists
		account, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
			CompanyID:     companyID,
			AccountNumber: d.AccountNumber,
		})
		if err != nil {
			// Create the account
			account, err = s.q.CreateAccount(ctx, sqlc.CreateAccountParams{
				ID:            uuid.New(),
				CompanyID:     companyID,
				AccountNumber: d.AccountNumber,
				Name:          d.AccountName,
				AccountType:   d.AccountType,
				IsActive:      true,
				IsSystem:      true,
				NormalBalance: d.NormalBalance,
			})
			if err != nil {
				slog.Warn("failed to create seed account", "number", d.AccountNumber, "error", err)
				continue
			}
			result.AccountsCreated++
		} else {
			result.AccountsExisted++
		}

		// Create GL mapping (ignore duplicates)
		_, err = s.q.CreateGLMappingRule(ctx, sqlc.CreateGLMappingRuleParams{
			CompanyID:       companyID,
			Jurisdiction:    jurisdiction,
			SourceDimension: d.SourceDimension,
			SourceValue:     d.SourceValue,
			TargetAccountID: account.ID,
			DebitCredit:     d.DebitCredit,
			Priority:        0,
			EffectiveFrom:   pgtype.Date{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil {
			// Likely unique constraint violation — mapping already exists
			result.MappingsExisted++
		} else {
			result.MappingsCreated++
		}
	}

	return result, nil
}
