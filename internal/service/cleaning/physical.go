package cleaning

import (
	"strings"
)

// headerKeywords are words commonly found in column headers of BIR forms and
// Philippine financial documents.
var headerKeywords = map[string]struct{}{
	"name": {}, "registered name": {}, "tin": {}, "address": {},
	"date": {}, "invoice": {}, "amount": {}, "gross": {}, "net": {},
	"tax": {}, "vat": {}, "rate": {}, "total": {}, "description": {},
	"supplier": {}, "customer": {}, "buyer": {}, "vendor": {}, "payee": {},
	"employee": {}, "employer": {}, "salary": {}, "compensation": {},
	"purchase": {}, "sales": {}, "revenue": {}, "expense": {},
	"debit": {}, "credit": {}, "balance": {}, "reference": {},
	"no.": {}, "no": {}, "number": {}, "code": {}, "type": {},
	"status": {}, "remarks": {}, "period": {}, "month": {}, "year": {},
	"exempt": {}, "zero rated": {}, "taxable": {}, "vatable": {},
	"input": {}, "output": {}, "withholding": {}, "creditable": {},
}

// DetectDataRegions finds contiguous blocks of non-blank rows by analyzing
// row density. Returns all candidate regions sorted by row count descending.
func DetectDataRegions(rows CellGrid) []DataRegion {
	if len(rows) == 0 {
		return nil
	}

	var regions []DataRegion
	inRegion := false
	regionStart := 0
	regionNonEmpty := 0
	consecutiveBlanks := 0
	const blankTolerance = 2 // allow up to 2 consecutive blank rows within a region

	for i, row := range rows {
		empty := IsEmptyRow(row)
		if empty {
			if inRegion {
				consecutiveBlanks++
				if consecutiveBlanks > blankTolerance {
					// End the region at the last non-blank row
					endRow := i - consecutiveBlanks
					if endRow >= regionStart && regionNonEmpty > 0 {
						regions = append(regions, DataRegion{
							StartRow: regionStart,
							EndRow:   endRow,
							RowCount: regionNonEmpty,
						})
					}
					inRegion = false
					regionNonEmpty = 0
				}
			}
		} else {
			if !inRegion {
				inRegion = true
				regionStart = i
				regionNonEmpty = 0
			}
			consecutiveBlanks = 0
			regionNonEmpty++
		}
	}

	// Close final region
	if inRegion && regionNonEmpty > 0 {
		endRow := len(rows) - 1
		// Trim trailing blanks
		for endRow > regionStart && IsEmptyRow(rows[endRow]) {
			endRow--
		}
		regions = append(regions, DataRegion{
			StartRow: regionStart,
			EndRow:   endRow,
			RowCount: regionNonEmpty,
		})
	}

	return regions
}

// BestDataRegion returns the largest region (by RowCount) from detected regions.
// For ties, prefers the one appearing later (more likely to be detail, not summary).
func BestDataRegion(regions []DataRegion) *DataRegion {
	if len(regions) == 0 {
		return nil
	}
	best := &regions[0]
	for i := 1; i < len(regions); i++ {
		if regions[i].RowCount >= best.RowCount {
			best = &regions[i]
		}
	}
	return best
}

// PruneEmptyColumns identifies columns that are entirely empty across the given
// rows. Returns the indices of empty columns.
func PruneEmptyColumns(rows CellGrid) []int {
	if len(rows) == 0 {
		return nil
	}

	maxCols := MaxColWidth(rows)
	if maxCols == 0 {
		return nil
	}

	// Track which columns have any non-empty value
	hasValue := make([]bool, maxCols)
	for _, row := range rows {
		for col := 0; col < len(row) && col < maxCols; col++ {
			if strings.TrimSpace(row[col]) != "" {
				hasValue[col] = true
			}
		}
	}

	var empty []int
	for col := 0; col < maxCols; col++ {
		if !hasValue[col] {
			empty = append(empty, col)
		}
	}
	return empty
}

// PruneEmptyRows returns the count of leading and trailing blank rows.
func PruneEmptyRows(rows CellGrid) (leadingBlanks, trailingBlanks int) {
	for i := 0; i < len(rows); i++ {
		if !IsEmptyRow(rows[i]) {
			break
		}
		leadingBlanks++
	}

	for i := len(rows) - 1; i >= leadingBlanks; i-- {
		if !IsEmptyRow(rows[i]) {
			break
		}
		trailingBlanks++
	}

	return leadingBlanks, trailingBlanks
}

// RemoveColumns returns a new grid with the specified column indices removed.
func RemoveColumns(rows CellGrid, colsToRemove []int) CellGrid {
	if len(colsToRemove) == 0 {
		return rows
	}

	removeSet := make(map[int]struct{}, len(colsToRemove))
	for _, c := range colsToRemove {
		removeSet[c] = struct{}{}
	}

	result := make(CellGrid, len(rows))
	for i, row := range rows {
		var newRow []string
		for col, cell := range row {
			if _, skip := removeSet[col]; !skip {
				newRow = append(newRow, cell)
			}
		}
		result[i] = newRow
	}
	return result
}

// DetectHeaderCandidates scores the first N rows as potential header rows.
// Returns row indices sorted by score descending.
func DetectHeaderCandidates(rows CellGrid, scanLimit int) []int {
	if len(rows) == 0 {
		return nil
	}
	if scanLimit <= 0 || scanLimit > len(rows) {
		scanLimit = len(rows)
	}
	if scanLimit > 30 {
		scanLimit = 30
	}

	totalCols := MaxColWidth(rows)
	if totalCols == 0 {
		return nil
	}

	type scored struct {
		index int
		score float64
	}
	var candidates []scored

	for i := 0; i < scanLimit; i++ {
		row := rows[i]
		if len(row) == 0 || IsEmptyRow(row) {
			continue
		}
		if IsSequentialNumbers(row) {
			continue
		}

		nonEmpty := 0
		numericCount := 0
		keywordHits := 0
		unique := make(map[string]struct{})

		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v == "" {
				continue
			}
			nonEmpty++
			unique[v] = struct{}{}
			if LooksNumeric(v) {
				numericCount++
			}
			lower := strings.ToLower(v)
			if _, ok := headerKeywords[lower]; ok {
				keywordHits++
			} else {
				for kw := range headerKeywords {
					if strings.Contains(lower, kw) {
						keywordHits++
						break
					}
				}
			}
		}

		if nonEmpty == 0 {
			continue
		}

		fillRatio := float64(nonEmpty) / float64(totalCols)
		if fillRatio < 0.3 {
			continue
		}

		uniqueness := float64(len(unique)) / float64(nonEmpty)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		if textRatio == 0 {
			continue
		}
		keywordRatio := float64(keywordHits) / float64(nonEmpty)
		score := fillRatio * uniqueness * (0.5 + 0.5*textRatio) * (1.0 + keywordRatio)

		candidates = append(candidates, scored{index: i, score: score})
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	result := make([]int, len(candidates))
	for i, c := range candidates {
		result[i] = c.index
	}
	return result
}

// DetectHeaderZone finds the contiguous block of rows that form the complete
// multi-row header. Returns (zoneStart, zoneEnd inclusive, dataStart).
func DetectHeaderZone(rows CellGrid, headerIdx int) (zoneStart, zoneEnd, dataStart int) {
	numCols := MaxColWidth(rows)
	zoneStart = headerIdx
	zoneEnd = headerIdx

	// Look UP: include rows that are text-heavy (super-headers).
	for i := headerIdx - 1; i >= 0; i-- {
		row := rows[i]
		if IsEmptyRow(row) {
			break
		}
		nonEmpty := 0
		numericCount := 0
		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v == "" {
				continue
			}
			nonEmpty++
			if LooksNumeric(v) {
				numericCount++
			}
		}
		if nonEmpty == 0 {
			break
		}
		fillRatio := float64(nonEmpty) / float64(numCols)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		if fillRatio >= 0.3 && textRatio > 0.7 && nonEmpty > 1 {
			zoneStart = i
		} else {
			break
		}
	}

	// Look DOWN: include sub-header rows.
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		if IsEmptyRow(row) {
			break
		}
		if IsSequentialNumbers(row) {
			continue
		}
		nonEmpty := 0
		numericCount := 0
		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v == "" {
				continue
			}
			nonEmpty++
			if LooksNumeric(v) {
				numericCount++
			}
		}
		if nonEmpty == 0 {
			break
		}
		fillRatio := float64(nonEmpty) / float64(numCols)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		if fillRatio <= 0.5 && textRatio > 0.5 {
			zoneEnd = i
		} else {
			break
		}
	}

	// Data starts after header zone, skipping numbering/blank rows.
	dataStart = zoneEnd + 1
	for dataStart < len(rows) {
		if IsSequentialNumbers(rows[dataStart]) || IsEmptyRow(rows[dataStart]) {
			dataStart++
			continue
		}
		break
	}

	return zoneStart, zoneEnd, dataStart
}

// MergeHeaderRows merges multiple header rows into single column names
// by concatenating non-empty values per column with spaces.
func MergeHeaderRows(rows CellGrid, start, end int) []string {
	if start > end || start < 0 || end >= len(rows) {
		if start >= 0 && start < len(rows) {
			return rows[start]
		}
		return nil
	}

	maxCols := 0
	for i := start; i <= end; i++ {
		if len(rows[i]) > maxCols {
			maxCols = len(rows[i])
		}
	}

	merged := make([]string, maxCols)
	for col := 0; col < maxCols; col++ {
		var parts []string
		for i := start; i <= end; i++ {
			if col < len(rows[i]) {
				v := strings.TrimSpace(rows[i][col])
				if v != "" {
					parts = append(parts, v)
				}
			}
		}
		merged[col] = strings.Join(parts, " ")
	}
	return merged
}

// CleanupColumns removes trailing empty columns and replaces empty names
// with positional labels (Column_1, Column_2, etc.).
func CleanupColumns(columns []string) []string {
	result := make([]string, len(columns))
	copy(result, columns)

	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	for i, c := range result {
		if c == "" {
			result[i] = "Column_" + itoa(i+1)
		}
	}
	return result
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// RunPhysicalHeuristics performs the complete physical analysis pass on raw rows.
func RunPhysicalHeuristics(rows CellGrid) PhysicalResult {
	result := PhysicalResult{}

	if len(rows) == 0 {
		return result
	}

	// 1. Prune empty rows (count leading/trailing blanks)
	result.TrimmedTopRows, result.TrimmedBotRows = PruneEmptyRows(rows)

	// 2. Prune empty columns
	result.PrunedColumns = PruneEmptyColumns(rows)

	// 3. Detect data regions
	result.DataRegions = DetectDataRegions(rows)
	result.BestRegion = BestDataRegion(result.DataRegions)

	// 4. Detect header candidates
	result.HeaderCandidates = DetectHeaderCandidates(rows, 30)

	return result
}
