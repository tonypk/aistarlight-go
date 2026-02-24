package service

import (
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// VATSummary holds aggregated VAT data from classified transactions.
type VATSummary struct {
	Period string `json:"period"`

	// Output VAT (Sales)
	VatableSales       decimal.Decimal `json:"vatable_sales"`
	SalesToGovernment  decimal.Decimal `json:"sales_to_government"`
	ZeroRatedSales     decimal.Decimal `json:"zero_rated_sales"`
	VATExemptSales     decimal.Decimal `json:"vat_exempt_sales"`
	TotalSales         decimal.Decimal `json:"total_sales"`
	OutputVAT          decimal.Decimal `json:"output_vat"`
	OutputVATGovernment decimal.Decimal `json:"output_vat_government"`
	TotalOutputVAT     decimal.Decimal `json:"total_output_vat"`

	// Input VAT (Purchases)
	InputVATGoods    decimal.Decimal `json:"input_vat_goods"`
	InputVATCapital  decimal.Decimal `json:"input_vat_capital"`
	InputVATServices decimal.Decimal `json:"input_vat_services"`
	InputVATImports  decimal.Decimal `json:"input_vat_imports"`
	TotalInputVAT    decimal.Decimal `json:"total_input_vat"`

	// Net
	NetVAT decimal.Decimal `json:"net_vat"`

	// Stats
	TransactionCount    int            `json:"transaction_count"`
	ClassificationStats map[string]int `json:"classification_stats"`
}

// MatchedPair represents a matched record-bank entry pair.
type MatchedPair struct {
	MatchGroupID uuid.UUID `json:"match_group_id"`
	RecordID     string    `json:"record_id"`
	BankID       string    `json:"bank_id"`
	RecordAmount float64   `json:"record_amount"`
	BankAmount   float64   `json:"bank_amount"`
	DateDiffDays int       `json:"date_diff_days"`
}

// UnmatchedEntry represents an unmatched record or bank entry.
type UnmatchedEntry struct {
	ID          string  `json:"id"`
	Amount      float64 `json:"amount"`
	Date        string  `json:"date"`
	Description string  `json:"description"`
}

// MatchResult holds the output of the matching algorithm.
type MatchResult struct {
	MatchedPairs     []MatchedPair    `json:"matched_pairs"`
	UnmatchedRecords []UnmatchedEntry `json:"unmatched_records"`
	UnmatchedBank    []UnmatchedEntry `json:"unmatched_bank"`
	MatchRate        float64          `json:"match_rate"`
}

// GenerateVATSummary aggregates output/input VAT from classified transactions.
func GenerateVATSummary(transactions []map[string]interface{}, period string) VATSummary {
	s := VATSummary{
		Period:              period,
		ClassificationStats: make(map[string]int),
	}

	for _, tx := range transactions {
		sourceType := toString(tx["source_type"])
		amount := toDecimal(tx["amount"])
		vatAmount := toDecimal(tx["vat_amount"])
		vatType := toString(tx["vat_type"])
		category := toString(tx["category"])

		s.TransactionCount++
		s.ClassificationStats[vatType]++

		isSales := sourceType == "sales_record" || category == "sale"

		if isSales {
			switch vatType {
			case "government":
				s.SalesToGovernment = s.SalesToGovernment.Add(amount)
				s.OutputVATGovernment = s.OutputVATGovernment.Add(amount.Mul(birforms.GovtVATRate))
			case "zero_rated":
				s.ZeroRatedSales = s.ZeroRatedSales.Add(amount)
			case "exempt":
				s.VATExemptSales = s.VATExemptSales.Add(amount)
			default: // vatable
				s.VatableSales = s.VatableSales.Add(amount)
				s.OutputVAT = s.OutputVAT.Add(amount.Mul(birforms.VATRate))
			}
		} else {
			// Purchases
			inputVAT := vatAmount
			if inputVAT.IsZero() {
				inputVAT = amount.Mul(birforms.VATRate)
			}

			switch category {
			case "capital":
				s.InputVATCapital = s.InputVATCapital.Add(inputVAT)
			case "services":
				s.InputVATServices = s.InputVATServices.Add(inputVAT)
			case "imports":
				s.InputVATImports = s.InputVATImports.Add(inputVAT)
			default: // goods
				s.InputVATGoods = s.InputVATGoods.Add(inputVAT)
			}
		}
	}

	s.TotalSales = s.VatableSales.Add(s.SalesToGovernment).Add(s.ZeroRatedSales).Add(s.VATExemptSales)
	s.TotalOutputVAT = s.OutputVAT.Add(s.OutputVATGovernment)
	s.TotalInputVAT = s.InputVATGoods.Add(s.InputVATCapital).Add(s.InputVATServices).Add(s.InputVATImports)
	s.NetVAT = s.TotalOutputVAT.Sub(s.TotalInputVAT)

	return s
}

// MatchTransactions runs a greedy matching algorithm to pair records with bank entries.
func MatchTransactions(records, bankEntries []map[string]interface{}, amountTolerance float64, dateToleranceDays int) MatchResult {
	if amountTolerance <= 0 {
		amountTolerance = 0.01
	}
	if dateToleranceDays <= 0 {
		dateToleranceDays = 3
	}

	used := make([]bool, len(bankEntries))
	var matched []MatchedPair
	var unmatchedRecords []UnmatchedEntry

	for _, rec := range records {
		recAmount := parseAmount(rec["amount"])
		recDate := toString(rec["date"])
		recID := toString(rec["id"])

		bestIdx := -1
		bestScore := math.MaxFloat64

		for j, bank := range bankEntries {
			if used[j] {
				continue
			}

			bankAmount := parseAmount(bank["amount"])
			amountDiff := math.Abs(recAmount - bankAmount)
			tolerance := math.Max(amountTolerance, math.Abs(recAmount)*0.001)
			if amountDiff > tolerance {
				continue
			}

			dateDiff := 0
			bankDate := toString(bank["date"])
			if recDate != "" && bankDate != "" {
				dateDiff = dateDiffDays(recDate, bankDate)
				if dateDiff > dateToleranceDays {
					continue
				}
			}

			score := amountDiff + float64(dateDiff)*0.01
			if score < bestScore {
				bestScore = score
				bestIdx = j
			}
		}

		if bestIdx >= 0 {
			used[bestIdx] = true
			bankEntry := bankEntries[bestIdx]
			matched = append(matched, MatchedPair{
				MatchGroupID: uuid.New(),
				RecordID:     recID,
				BankID:       toString(bankEntry["id"]),
				RecordAmount: recAmount,
				BankAmount:   parseAmount(bankEntry["amount"]),
				DateDiffDays: dateDiffDays(recDate, toString(bankEntry["date"])),
			})
		} else {
			unmatchedRecords = append(unmatchedRecords, UnmatchedEntry{
				ID:          recID,
				Amount:      recAmount,
				Date:        recDate,
				Description: toString(rec["description"]),
			})
		}
	}

	var unmatchedBank []UnmatchedEntry
	for j, bank := range bankEntries {
		if !used[j] {
			unmatchedBank = append(unmatchedBank, UnmatchedEntry{
				ID:          toString(bank["id"]),
				Amount:      parseAmount(bank["amount"]),
				Date:        toString(bank["date"]),
				Description: toString(bank["description"]),
			})
		}
	}

	totalItems := len(records) + len(bankEntries)
	matchRate := 0.0
	if totalItems > 0 {
		matchRate = float64(len(matched)*2) / float64(totalItems)
	}

	return MatchResult{
		MatchedPairs:     matched,
		UnmatchedRecords: unmatchedRecords,
		UnmatchedBank:    unmatchedBank,
		MatchRate:        matchRate,
	}
}

// ComparisonLine represents a single BIR field comparison.
type ComparisonLine struct {
	Line       string `json:"line"`
	Label      string `json:"label"`
	Computed   string `json:"computed"`
	Declared   string `json:"declared"`
	Difference string `json:"difference"`
	Match      bool   `json:"match"`
}

// ComparisonResult holds the output of declared-vs-computed comparison.
type ComparisonResult struct {
	Comparisons     []ComparisonLine `json:"comparisons"`
	MatchedLines    int              `json:"matched_lines"`
	TotalLines      int              `json:"total_lines"`
	TotalDifference string           `json:"total_difference"`
	FullyMatched    bool             `json:"fully_matched"`
}

// CompareWithDeclared compares computed VAT summary with declared BIR 2550M values.
func CompareWithDeclared(summary VATSummary, declared map[string]string) ComparisonResult {
	fieldMap := []struct {
		line    string
		label   string
		value   decimal.Decimal
	}{
		{"line_1_vatable_sales", "vatable_sales", summary.VatableSales},
		{"line_2_sales_to_government", "sales_to_government", summary.SalesToGovernment},
		{"line_3_zero_rated_sales", "zero_rated_sales", summary.ZeroRatedSales},
		{"line_4_exempt_sales", "exempt_sales", summary.VATExemptSales},
		{"line_5_total_sales", "total_sales", summary.TotalSales},
		{"line_6_output_vat", "output_vat", summary.OutputVAT},
		{"line_6a_output_vat_government", "output_vat_government", summary.OutputVATGovernment},
		{"line_6b_total_output_vat", "total_output_vat", summary.TotalOutputVAT},
		{"line_7_input_vat_goods", "input_vat_goods", summary.InputVATGoods},
		{"line_8_input_vat_capital", "input_vat_capital", summary.InputVATCapital},
		{"line_9_input_vat_services", "input_vat_services", summary.InputVATServices},
		{"line_10_input_vat_imports", "input_vat_imports", summary.InputVATImports},
		{"line_11_total_input_vat", "total_input_vat", summary.TotalInputVAT},
	}

	threshold := decimal.NewFromFloat(0.01)
	var comparisons []ComparisonLine
	matchedCount := 0
	totalDiff := decimal.Zero

	for _, f := range fieldMap {
		declaredVal := decimal.Zero
		if v, ok := declared[f.line]; ok {
			declaredVal = toDecimal(v)
		} else if v, ok := declared[f.label]; ok {
			declaredVal = toDecimal(v)
		}

		diff := f.value.Sub(declaredVal)
		isMatch := diff.Abs().LessThan(threshold)
		if isMatch {
			matchedCount++
		}
		totalDiff = totalDiff.Add(diff.Abs())

		comparisons = append(comparisons, ComparisonLine{
			Line:       f.line,
			Label:      f.label,
			Computed:   f.value.String(),
			Declared:   declaredVal.String(),
			Difference: diff.String(),
			Match:      isMatch,
		})
	}

	return ComparisonResult{
		Comparisons:     comparisons,
		MatchedLines:    matchedCount,
		TotalLines:      len(fieldMap),
		TotalDifference: totalDiff.String(),
		FullyMatched:    matchedCount == len(fieldMap),
	}
}

func dateDiffDays(dateA, dateB string) int {
	if dateA == "" || dateB == "" {
		return 0
	}
	tA, errA := time.Parse("2006-01-02", dateA)
	tB, errB := time.Parse("2006-01-02", dateB)
	if errA != nil || errB != nil {
		return 0
	}
	diff := tA.Sub(tB)
	days := int(math.Abs(diff.Hours() / 24))
	return days
}
