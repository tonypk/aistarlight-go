package service

import (
	"fmt"
	"math"
)

// BalanceResult holds the balance tracking analysis.
type BalanceResult struct {
	OpeningBalance     float64            `json:"opening_balance"`
	ClosingBalance     float64            `json:"closing_balance"`
	BankClosingBalance float64            `json:"bank_closing_balance,omitempty"`
	TotalDebits        float64            `json:"total_debits"`
	TotalCredits       float64            `json:"total_credits"`
	ComputedClosing    float64            `json:"computed_closing"`
	BalanceDifference  float64            `json:"balance_difference"`
	IsBalanced         bool               `json:"is_balanced"`
	Discrepancies      []BalanceAnomaly   `json:"discrepancies,omitempty"`
}

// BalanceAnomaly represents a balance-related anomaly.
type BalanceAnomaly struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Severity    string  `json:"severity"`
}

// TrackBalance computes balance flow from opening through transactions to closing.
// openingBalance: the starting balance for the period.
// bankClosingBalance: the bank statement's ending balance (0 if not provided).
// transactions: all transactions in the period.
func TrackBalance(openingBalance, bankClosingBalance float64, transactions []map[string]interface{}) BalanceResult {
	var totalDebits, totalCredits float64

	for _, txn := range transactions {
		amount := parseAmount(txn["amount"])
		if amount <= 0 {
			continue
		}

		sourceType := toString(txn["source_type"])
		entryType := toString(txn["type"])

		// Determine if debit or credit based on source type and entry type
		isCredit := false
		switch {
		case sourceType == "sales_record":
			isCredit = true // sales increase balance
		case sourceType == "purchase_record":
			isCredit = false // purchases decrease balance
		case sourceType == "bank_statement" && entryType == "credit":
			isCredit = true
		case sourceType == "bank_statement" && entryType == "debit":
			isCredit = false
		case entryType == "credit":
			isCredit = true
		default:
			isCredit = false
		}

		if isCredit {
			totalCredits += amount
		} else {
			totalDebits += amount
		}
	}

	computedClosing := openingBalance + totalCredits - totalDebits
	balanceDiff := 0.0
	isBalanced := true

	if bankClosingBalance != 0 {
		balanceDiff = computedClosing - bankClosingBalance
		isBalanced = math.Abs(balanceDiff) < 0.01
	}

	result := BalanceResult{
		OpeningBalance:     openingBalance,
		ClosingBalance:     computedClosing,
		BankClosingBalance: bankClosingBalance,
		TotalDebits:        math.Round(totalDebits*100) / 100,
		TotalCredits:       math.Round(totalCredits*100) / 100,
		ComputedClosing:    math.Round(computedClosing*100) / 100,
		BalanceDifference:  math.Round(balanceDiff*100) / 100,
		IsBalanced:         isBalanced,
	}

	// Detect balance discrepancies
	if bankClosingBalance != 0 && !isBalanced {
		severity := "medium"
		if math.Abs(balanceDiff) >= 10000 {
			severity = "high"
		}
		result.Discrepancies = append(result.Discrepancies, BalanceAnomaly{
			Type:        "balance_mismatch",
			Description: fmt.Sprintf("Computed closing balance (%.2f) differs from bank statement (%.2f) by %.2f", computedClosing, bankClosingBalance, balanceDiff),
			Amount:      balanceDiff,
			Severity:    severity,
		})
	}

	// Check for negative closing balance
	if computedClosing < 0 {
		result.Discrepancies = append(result.Discrepancies, BalanceAnomaly{
			Type:        "negative_balance",
			Description: fmt.Sprintf("Closing balance is negative: %.2f — possible missing credits or incorrect opening balance", computedClosing),
			Amount:      computedClosing,
			Severity:    "high",
		})
	}

	// Check if debits significantly exceed credits (unusual for most businesses)
	if totalCredits > 0 && totalDebits > totalCredits*2 {
		result.Discrepancies = append(result.Discrepancies, BalanceAnomaly{
			Type:        "debit_heavy",
			Description: fmt.Sprintf("Total debits (%.2f) are more than 2x total credits (%.2f) — verify all income is recorded", totalDebits, totalCredits),
			Amount:      totalDebits - totalCredits,
			Severity:    "medium",
		})
	}

	return result
}
