package cleaning

import (
	"math"
	"strings"
)

// summaryKeywords are phrases indicating summary/total rows. Weighted by specificity.
var summaryKeywords = map[string]float64{
	"grand total":       1.0,
	"overall total":     1.0,
	"net vat payable":   1.0,
	"vat payable":       0.9,
	"sub total":         0.9,
	"subtotal":          0.9,
	"sub-total":         0.9,
	"total sales":       0.9,
	"total purchases":   0.9,
	"total amount":      0.9,
	"total vat":         0.9,
	"total input tax":   0.9,
	"total output tax":  0.9,
	"total tax":         0.9,
	"total exempt":      0.8,
	"total zero-rated":  0.8,
	"total zero rated":  0.8,
	"total vatable":     0.8,
	"total gross":       0.8,
	"total deductions":  0.8,
	"total income":      0.8,
	"total expenses":    0.8,
	"total compensation": 0.8,
	"total withheld":    0.8,
	"total tax withheld": 0.8,
	"total remittance":  0.8,
	"total payable":     0.8,
	"amount due":        0.7,
	"balance due":       0.7,
	"net amount":        0.7,
	"net payable":       0.7,
	"summary":           0.6,
	"page total":        0.8,
	"carried forward":   0.7,
	"brought forward":   0.7,
}

// noteKeywords indicate annotation/metadata rows (not data, not totals).
var noteKeywords = map[string]float64{
	"note:":       0.8,
	"notes:":      0.8,
	"remarks:":    0.7,
	"source:":     0.6,
	"reference:":  0.5,
	"prepared by": 0.9,
	"checked by":  0.9,
	"approved by": 0.9,
	"audited by":  0.9,
	"verified by": 0.9,
	"signature":   0.8,
	"attachment":  0.6,
}

// ClassifierConfig holds tuning parameters for the row classifier.
type ClassifierConfig struct {
	// Weights for combining sub-scores
	KeywordWeight       float64
	StructureWeight     float64
	SumCheckWeight      float64
	HeaderRepeatWeight  float64

	// Thresholds
	SummaryThreshold    float64 // final score above this → summary row
	HeaderRepeatJaccard float64 // Jaccard > this → header repeat
	SumCheckTolerance   float64 // relative tolerance for sum check
}

// DefaultClassifierConfig returns sensible defaults.
func DefaultClassifierConfig() ClassifierConfig {
	return ClassifierConfig{
		KeywordWeight:       0.45,
		StructureWeight:     0.30,
		SumCheckWeight:      0.15,
		HeaderRepeatWeight:  0.10,
		SummaryThreshold:    0.40,
		HeaderRepeatJaccard: 0.70,
		SumCheckTolerance:   0.001,
	}
}

// RowClassifier classifies spreadsheet rows into types.
type RowClassifier struct {
	config        ClassifierConfig
	headerColumns []string // the detected header column names
	totalCols     int      // max column width
}

// NewRowClassifier creates a classifier with the given header columns.
func NewRowClassifier(headerColumns []string, totalCols int, config ClassifierConfig) *RowClassifier {
	return &RowClassifier{
		config:        config,
		headerColumns: headerColumns,
		totalCols:     totalCols,
	}
}

// ClassifyRows classifies all rows in the data region.
// headerRow is the row index of the header for header-repeat detection.
// rows are the raw rows (0-indexed from original sheet).
// dataStartRow and dataEndRow define the data region (inclusive, in original row indices).
func (rc *RowClassifier) ClassifyRows(rows CellGrid, dataStartRow, dataEndRow int) []RowClassification {
	var results []RowClassification

	for i := dataStartRow; i <= dataEndRow && i < len(rows); i++ {
		row := rows[i]
		cls := rc.ClassifyRow(row, i, rows, dataStartRow)
		results = append(results, cls)
	}

	return results
}

// ClassifyRow classifies a single row.
func (rc *RowClassifier) ClassifyRow(row []string, rowIndex int, allRows CellGrid, dataStartRow int) RowClassification {
	// Quick checks
	if IsEmptyRow(row) {
		return RowClassification{
			RowIndex:   rowIndex,
			Type:       RowTypeBlank,
			Confidence: 1.0,
			Reason:     "all cells empty",
		}
	}

	if IsSequentialNumbers(row) {
		return RowClassification{
			RowIndex:   rowIndex,
			Type:       RowTypeNumbering,
			Confidence: 1.0,
			Reason:     "sequential numbering pattern",
		}
	}

	// Compute sub-scores
	kwScore, kwReason := rc.keywordScore(row)
	structScore, structReason := rc.structureScore(row)
	sumScore, sumReason := rc.sumCheckScore(row, rowIndex, allRows, dataStartRow)
	hdrScore, hdrReason := rc.headerRepeatScore(row)

	// Check for note/metadata rows — note keywords take priority over summary keywords
	// when the row is very sparse (1-2 cells, indicating annotation not a total line)
	noteScore, noteReason := rc.noteScore(row)
	if noteScore > 0.5 {
		return RowClassification{
			RowIndex:   rowIndex,
			Type:       RowTypeNote,
			Confidence: noteScore,
			Reason:     noteReason,
		}
	}

	// Header repeat check (takes priority if very high)
	if hdrScore > rc.config.HeaderRepeatJaccard {
		return RowClassification{
			RowIndex:   rowIndex,
			Type:       RowTypeHeaderRepeat,
			Confidence: hdrScore,
			Reason:     hdrReason,
		}
	}

	// False-positive protection: "Total Logistics Inc." style rows
	// If the row has a date-like value AND a TIN-like value AND high fill ratio,
	// it's almost certainly data, even if it contains "total".
	if rc.hasFalsePositiveProtection(row) && kwScore < 0.9 {
		return RowClassification{
			RowIndex:   rowIndex,
			Type:       RowTypeData,
			Confidence: 0.85,
			Reason:     "false-positive protection: has date + TIN/doc_no + high fill",
		}
	}

	// Weighted combination for summary detection
	cfg := rc.config
	finalScore := kwScore*cfg.KeywordWeight +
		structScore*cfg.StructureWeight +
		sumScore*cfg.SumCheckWeight +
		hdrScore*cfg.HeaderRepeatWeight

	if finalScore >= cfg.SummaryThreshold {
		rowType := RowTypeSubtotal
		reason := "subtotal"

		// Distinguish grand total from subtotal
		if rc.isGrandTotal(row) {
			rowType = RowTypeGrandTotal
			reason = "grand_total"
		}

		reasons := []string{reason}
		if kwReason != "" {
			reasons = append(reasons, "kw:"+kwReason)
		}
		if structReason != "" {
			reasons = append(reasons, "struct:"+structReason)
		}
		if sumReason != "" {
			reasons = append(reasons, "sum:"+sumReason)
		}

		return RowClassification{
			RowIndex:   rowIndex,
			Type:       rowType,
			Confidence: math.Min(finalScore, 1.0),
			Reason:     strings.Join(reasons, "; "),
		}
	}

	// Default: data row
	return RowClassification{
		RowIndex:   rowIndex,
		Type:       RowTypeData,
		Confidence: 1.0 - finalScore,
		Reason:     "data row (no summary signals)",
	}
}

// keywordScore checks for summary/total keywords in the row.
func (rc *RowClassifier) keywordScore(row []string) (float64, string) {
	bestScore := 0.0
	bestKeyword := ""

	for _, cell := range row {
		lower := strings.ToLower(strings.TrimSpace(cell))
		if lower == "" {
			continue
		}

		// Exact "total" standalone
		if lower == "total" {
			if bestScore < 0.85 {
				bestScore = 0.85
				bestKeyword = "total"
			}
			continue
		}

		for kw, weight := range summaryKeywords {
			if strings.Contains(lower, kw) {
				if weight > bestScore {
					bestScore = weight
					bestKeyword = kw
				}
			}
		}
	}

	return bestScore, bestKeyword
}

// structureScore evaluates structural features: low fill ratio with few cells
// filled suggests a summary/total row.
func (rc *RowClassifier) structureScore(row []string) (float64, string) {
	if rc.totalCols == 0 {
		return 0, ""
	}

	fillRatio := FillRatio(row, rc.totalCols)
	nonEmpty := NonEmptyCount(row)

	// Summary rows typically have very few filled cells
	// (just the label + total amount, maybe 2-4 cells)
	score := 0.0
	reason := ""

	if rc.totalCols > 4 && fillRatio < 0.3 && nonEmpty <= 3 {
		score = 0.8
		reason = "very sparse row (fill<30%, <=3 cells)"
	} else if rc.totalCols > 4 && fillRatio <= 0.5 && nonEmpty <= 4 {
		score = 0.6
		reason = "sparse row (fill<=50%, <=4 cells)"
	} else if rc.totalCols > 4 && fillRatio <= 0.7 && nonEmpty <= 5 {
		score = 0.3
		reason = "moderate fill with few cells"
	} else if rc.totalCols > 4 && fillRatio > 0.7 {
		// High fill ratio suggests data, not summary
		score = 0.0
		reason = ""
	}

	return score, reason
}

// sumCheckScore checks whether the row's numeric values approximately equal
// the sum of the preceding N rows. This is a strong indicator of a subtotal row.
func (rc *RowClassifier) sumCheckScore(row []string, rowIndex int, allRows CellGrid, dataStartRow int) (float64, string) {
	if rowIndex <= dataStartRow {
		return 0, ""
	}

	// Find numeric columns in this row
	type numCol struct {
		col   int
		value float64
	}
	var numCols []numCol

	for col, cell := range row {
		v, ok := ParseFloat(cell)
		if ok && v != 0 {
			numCols = append(numCols, numCol{col: col, value: v})
		}
	}

	if len(numCols) == 0 {
		return 0, ""
	}

	// For each numeric column, sum the preceding rows (up to 50) and check
	// if this row's value matches.
	matchCount := 0
	checkCount := 0
	lookback := rowIndex - dataStartRow
	if lookback > 50 {
		lookback = 50
	}
	if lookback < 2 {
		return 0, ""
	}

	for _, nc := range numCols {
		sum := 0.0
		validRows := 0
		for i := rowIndex - 1; i >= rowIndex-lookback && i >= dataStartRow; i-- {
			if i >= len(allRows) {
				continue
			}
			r := allRows[i]
			if nc.col >= len(r) {
				continue
			}
			v, ok := ParseFloat(r[nc.col])
			if ok {
				sum += v
				validRows++
			}
		}
		if validRows < 2 {
			continue
		}

		checkCount++
		tolerance := math.Max(1.0, math.Abs(sum)*rc.config.SumCheckTolerance)
		if math.Abs(nc.value-sum) <= tolerance {
			matchCount++
		}
	}

	if checkCount == 0 {
		return 0, ""
	}

	matchRatio := float64(matchCount) / float64(checkCount)
	if matchRatio >= 0.5 {
		return matchRatio, "sum-check matched"
	}
	return 0, ""
}

// headerRepeatScore checks if the row matches the header columns (Jaccard similarity).
func (rc *RowClassifier) headerRepeatScore(row []string) (float64, string) {
	if len(rc.headerColumns) == 0 {
		return 0, ""
	}

	// Extract non-empty, non-numeric cells from the row
	var rowTexts []string
	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v != "" && !LooksNumeric(v) {
			rowTexts = append(rowTexts, v)
		}
	}

	if len(rowTexts) == 0 {
		return 0, ""
	}

	// Compare against header columns (non-empty only)
	var headerTexts []string
	for _, h := range rc.headerColumns {
		v := strings.TrimSpace(h)
		if v != "" {
			headerTexts = append(headerTexts, v)
		}
	}

	sim := JaccardSimilarity(rowTexts, headerTexts)
	if sim > rc.config.HeaderRepeatJaccard {
		return sim, "header repeat detected"
	}
	return sim * 0.5, "" // partial signal even below threshold
}

// noteScore checks for metadata/annotation keywords.
func (rc *RowClassifier) noteScore(row []string) (float64, string) {
	bestScore := 0.0
	bestReason := ""

	nonEmpty := NonEmptyCount(row)
	if nonEmpty == 0 {
		return 0, ""
	}

	// Notes typically have 1-2 cells, mostly text
	if nonEmpty > 3 {
		return 0, ""
	}

	for _, cell := range row {
		lower := strings.ToLower(strings.TrimSpace(cell))
		if lower == "" {
			continue
		}
		for kw, weight := range noteKeywords {
			if strings.Contains(lower, kw) {
				if weight > bestScore {
					bestScore = weight
					bestReason = kw
				}
			}
		}
	}

	return bestScore, bestReason
}

// isGrandTotal checks if the row specifically contains "grand total" or
// similar indicators of a final total.
func (rc *RowClassifier) isGrandTotal(row []string) bool {
	for _, cell := range row {
		lower := strings.ToLower(strings.TrimSpace(cell))
		if strings.Contains(lower, "grand total") ||
			strings.Contains(lower, "overall total") ||
			lower == "total" {
			return true
		}
	}
	return false
}

// hasFalsePositiveProtection returns true if the row has characteristics of
// a real data row despite containing summary-like keywords.
// Example: "Total Logistics Inc." with date + TIN → real data, not a total.
func (rc *RowClassifier) hasFalsePositiveProtection(row []string) bool {
	hasDate := false
	hasDocNo := false
	fillRatio := FillRatio(row, rc.totalCols)

	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			continue
		}
		if looksLikeDate(v) {
			hasDate = true
		}
		if looksLikeTINOrDocNo(v) {
			hasDocNo = true
		}
	}

	// Protection triggers when: has date + has TIN/doc number + high fill ratio
	return hasDate && hasDocNo && fillRatio > 0.5
}

// looksLikeDate checks if a string looks like a date pattern.
func looksLikeDate(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 6 || len(s) > 20 {
		return false
	}

	// Count separators common in dates
	slashes := strings.Count(s, "/")
	dashes := strings.Count(s, "-")

	// Pattern: MM/DD/YYYY, DD/MM/YYYY, YYYY-MM-DD, etc.
	if slashes >= 2 || (dashes >= 2 && !strings.Contains(s, " ")) {
		// Verify it has digits
		digits := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		return digits >= 4
	}
	return false
}

// looksLikeTINOrDocNo checks if a string looks like a Philippine TIN (###-###-###-###)
// or a document/invoice number.
func looksLikeTINOrDocNo(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 5 {
		return false
	}

	// TIN pattern: digits separated by dashes, e.g. 123-456-789-000
	dashes := strings.Count(s, "-")
	if dashes >= 2 {
		digits := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		// TIN has 9-12 digits with dashes
		if digits >= 6 && float64(digits+dashes)/float64(len(s)) > 0.8 {
			return true
		}
	}

	return false
}
