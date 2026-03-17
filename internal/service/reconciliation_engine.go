package service

import (
	"math"
	"sort"
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

	// Post-import validation warnings
	ValidationWarnings []ValidationWarning `json:"validation_warnings,omitempty"`
}

// MatchedPair represents a matched record-bank entry pair.
type MatchedPair struct {
	MatchGroupID uuid.UUID   `json:"match_group_id"`
	RecordID     string      `json:"record_id"`
	BankID       string      `json:"bank_id"`
	RecordAmount float64     `json:"record_amount"`
	BankAmount   float64     `json:"bank_amount"`
	DateDiffDays int         `json:"date_diff_days"`
	Score        *MatchScore `json:"score,omitempty"`
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
	SplitMatches     []SplitMatch     `json:"split_matches,omitempty"`
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

		isSales := sourceType == "sales_record"

		if isSales {
			switch vatType {
			case "government":
				s.SalesToGovernment = s.SalesToGovernment.Add(amount)
				govVAT := vatAmount
				if govVAT.IsZero() {
					govVAT = amount.Mul(birforms.GovtVATRate)
				}
				s.OutputVATGovernment = s.OutputVATGovernment.Add(govVAT)
			case "zero_rated":
				s.ZeroRatedSales = s.ZeroRatedSales.Add(amount)
			case "exempt":
				s.VATExemptSales = s.VATExemptSales.Add(amount)
			default: // vatable
				s.VatableSales = s.VatableSales.Add(amount)
				outputTax := vatAmount
				if outputTax.IsZero() {
					outputTax = amount.Mul(birforms.VATRate)
				}
				s.OutputVAT = s.OutputVAT.Add(outputTax)
			}
		} else {
			// Purchases
			inputVAT := vatAmount
			if inputVAT.IsZero() && vatType != "exempt" && vatType != "zero_rated" {
				// Only compute 12% for vatable purchases; exempt/zero-rated have no input VAT
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

// MatchTransactions runs a multi-signal scoring algorithm to pair records with bank entries.
// It uses amount similarity, date proximity, reference number matching, and description
// similarity to find the best matches. It also attempts split matching (N:1) for
// unmatched bank entries.
func MatchTransactions(records, bankEntries []map[string]interface{}, amountTolerance float64, dateToleranceDays int) MatchResult {
	cfg := DefaultScorerConfig()
	if amountTolerance > 0 {
		cfg.AmountTolerance = amountTolerance
	}
	if dateToleranceDays > 0 {
		cfg.DateToleranceDays = dateToleranceDays
	}

	usedBank := make([]bool, len(bankEntries))
	usedRec := make([]bool, len(records))
	var matched []MatchedPair

	// Build all candidate pairs with scores
	type scoredPair struct {
		recIdx int
		bankIdx int
		score  MatchScore
	}
	var pairs []scoredPair

	for i, rec := range records {
		for j, bank := range bankEntries {
			score := ScoreMatch(rec, bank, cfg)
			if score.Total >= cfg.MinMatchScore {
				pairs = append(pairs, scoredPair{recIdx: i, bankIdx: j, score: score})
			}
		}
	}

	// Sort by total score descending (greedy best-first); stable for deterministic output
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].score.Total > pairs[j].score.Total
	})

	// Assign best matches greedily
	for _, p := range pairs {
		if usedRec[p.recIdx] || usedBank[p.bankIdx] {
			continue
		}
		usedRec[p.recIdx] = true
		usedBank[p.bankIdx] = true

		rec := records[p.recIdx]
		bank := bankEntries[p.bankIdx]
		recDate := toString(rec["date"])
		bankDate := toString(bank["date"])
		score := p.score

		matched = append(matched, MatchedPair{
			MatchGroupID: uuid.New(),
			RecordID:     toString(rec["id"]),
			BankID:       toString(bank["id"]),
			RecordAmount: parseAmount(rec["amount"]),
			BankAmount:   parseAmount(bank["amount"]),
			DateDiffDays: dateDiffDays(recDate, bankDate),
			Score:        &score,
		})
	}

	// Collect unmatched
	var unmatchedRecords []UnmatchedEntry
	for i, rec := range records {
		if !usedRec[i] {
			unmatchedRecords = append(unmatchedRecords, UnmatchedEntry{
				ID:          toString(rec["id"]),
				Amount:      parseAmount(rec["amount"]),
				Date:        toString(rec["date"]),
				Description: toString(rec["description"]),
			})
		}
	}

	var unmatchedBank []UnmatchedEntry
	for j, bank := range bankEntries {
		if !usedBank[j] {
			unmatchedBank = append(unmatchedBank, UnmatchedEntry{
				ID:          toString(bank["id"]),
				Amount:      parseAmount(bank["amount"]),
				Date:        toString(bank["date"]),
				Description: toString(bank["description"]),
			})
		}
	}

	// Attempt split matching on unmatched bank entries
	var splitMatches []SplitMatch
	unmatchedRecMaps := make([]map[string]interface{}, 0, len(unmatchedRecords))
	for i, rec := range records {
		if !usedRec[i] {
			unmatchedRecMaps = append(unmatchedRecMaps, rec)
		}
	}

	if len(unmatchedRecMaps) >= 2 {
		splitUsedBank := make(map[string]bool)
		splitUsedRecs := make(map[string]bool)

		for j, bank := range bankEntries {
			if usedBank[j] {
				continue
			}
			splits := FindSplitMatches(unmatchedRecMaps, bank, cfg.AmountTolerance)
			if len(splits) == 0 {
				continue
			}
			// Take the best split (smallest difference)
			best := splits[0]
			for _, s := range splits[1:] {
				if s.Difference < best.Difference {
					best = s
				}
			}
			// Check that none of these records are already used in another split
			conflict := false
			for _, rid := range best.RecordIDs {
				if splitUsedRecs[rid] {
					conflict = true
					break
				}
			}
			if conflict || splitUsedBank[best.BankID] {
				continue
			}
			splitUsedBank[best.BankID] = true
			for _, rid := range best.RecordIDs {
				splitUsedRecs[rid] = true
			}
			best.MatchType = "split"
			splitMatches = append(splitMatches, best)
		}

		// Remove split-matched entries from unmatched lists
		if len(splitMatches) > 0 {
			bankSet := make(map[string]bool)
			recSet := make(map[string]bool)
			for _, sm := range splitMatches {
				bankSet[sm.BankID] = true
				for _, rid := range sm.RecordIDs {
					recSet[rid] = true
				}
			}
			filtered := unmatchedRecords[:0]
			for _, ur := range unmatchedRecords {
				if !recSet[ur.ID] {
					filtered = append(filtered, ur)
				}
			}
			unmatchedRecords = filtered

			filteredBank := unmatchedBank[:0]
			for _, ub := range unmatchedBank {
				if !bankSet[ub.ID] {
					filteredBank = append(filteredBank, ub)
				}
			}
			unmatchedBank = filteredBank
		}
	}

	totalItems := len(records) + len(bankEntries)
	matchRate := 0.0
	if totalItems > 0 {
		matchedCount := len(matched)*2 + splitMatchedCount(splitMatches)
		matchRate = float64(matchedCount) / float64(totalItems)
	}

	return MatchResult{
		MatchedPairs:     matched,
		UnmatchedRecords: unmatchedRecords,
		UnmatchedBank:    unmatchedBank,
		SplitMatches:     splitMatches,
		MatchRate:        matchRate,
	}
}

// splitMatchedCount returns the total number of entries involved in split matches.
func splitMatchedCount(splits []SplitMatch) int {
	count := 0
	for _, s := range splits {
		count += len(s.RecordIDs) + 1 // records + bank entry
	}
	return count
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
