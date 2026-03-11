package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service/cleaning"
	"github.com/xuri/excelize/v2"
)

const (
	MaxUploadSizeMB = 10
	MaxUploadSize   = MaxUploadSizeMB * 1024 * 1024 // 10MB
	MaxPreviewRows  = 10
	MaxSheets       = 20
	MaxRowsPerSheet = 200_000
)

// SheetData represents parsed data from a single sheet.
type SheetData struct {
	Columns  []string                 `json:"columns"`
	RowCount int                      `json:"row_count"`
	Preview  []map[string]interface{} `json:"preview"`
}

// ParsedFile represents the result of parsing an uploaded file.
type ParsedFile struct {
	Type   string                `json:"type"`
	Sheets map[string]*SheetData `json:"sheets"`
}

// ParseUploadedFile parses Excel or CSV content and returns structured data.
func ParseUploadedFile(content []byte, filename string) (*ParsedFile, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".xlsx", ".xls":
		return parseExcel(content)
	case ".csv":
		return parseCSV(content)
	default:
		return nil, fmt.Errorf("unsupported format: %s (accepted: .xlsx, .xls, .csv)", ext)
	}
}

func parseExcel(content []byte) (*ParsedFile, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("cannot open Excel file: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheetList := f.GetSheetList()
	if len(sheetList) > MaxSheets {
		return nil, fmt.Errorf("file has too many sheets (%d); maximum is %d", len(sheetList), MaxSheets)
	}

	sheets := make(map[string]*SheetData)
	for _, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheetName, err)
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) > MaxRowsPerSheet {
			return nil, fmt.Errorf("sheet %q has too many rows (%d); maximum is %d", sheetName, len(rows), MaxRowsPerSheet)
		}

		sd := rowsToSheetData(rows)
		if sd != nil {
			sheets[sheetName] = sd
		}
	}

	if len(sheets) == 0 {
		return nil, fmt.Errorf("file contains no data — all sheets are empty")
	}

	return &ParsedFile{Type: "excel", Sheets: sheets}, nil
}

func parseCSV(content []byte) (*ParsedFile, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow varying number of fields per record

	var rows [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV parse error: %w", err)
		}
		rows = append(rows, record)
		if len(rows) > MaxRowsPerSheet {
			return nil, fmt.Errorf("CSV exceeds maximum row limit of %d", MaxRowsPerSheet)
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	sd := rowsToSheetData(rows)
	if sd == nil {
		return nil, fmt.Errorf("CSV file contains only headers with no data rows")
	}

	return &ParsedFile{
		Type:   "csv",
		Sheets: map[string]*SheetData{"Sheet1": sd},
	}, nil
}

// headerKeywords are words commonly found in column headers of BIR forms and
// Philippine financial documents. A row containing several of these is very
// likely the header row.
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

// looksNumeric returns true if the string looks like a number, date, or
// TIN-like pattern — i.e. NOT a header label.
func looksNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Strip common formatting
	cleaned := strings.NewReplacer(",", "", "$", "", "₱", "", "PHP", "", "%", "", " ", "").Replace(s)
	// Pure number (possibly negative/decimal)
	digitDot := 0
	for _, c := range cleaned {
		if (c >= '0' && c <= '9') || c == '.' || c == '-' {
			digitDot++
		}
	}
	if len(cleaned) > 0 && float64(digitDot)/float64(len(cleaned)) > 0.7 {
		return true
	}
	return false
}

// isSequentialNumbers returns true if the row looks like sequential field numbering
// (e.g. "1", "2", "3" or "(1)", "(2)", "(3)"). BIR forms often have such a row
// just above or below the real column headers.
func isSequentialNumbers(row []string) bool {
	var nums []int
	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			continue
		}
		// Strip parentheses: "(1)" → "1"
		v = strings.TrimPrefix(v, "(")
		v = strings.TrimSuffix(v, ")")
		v = strings.TrimSpace(v)
		n := 0
		isInt := true
		for _, c := range v {
			if c < '0' || c > '9' {
				isInt = false
				break
			}
			n = n*10 + int(c-'0')
		}
		if !isInt || n < 0 {
			return false
		}
		nums = append(nums, n)
	}
	if len(nums) < 3 {
		return false
	}
	for i := 1; i < len(nums); i++ {
		if nums[i] != nums[i-1]+1 {
			return false
		}
	}
	return true
}

// summaryRowKeywords are phrases that indicate a summary/total row.
// Matched case-insensitively against each cell in the row.
var summaryRowKeywords = []string{
	"grand total",
	"sub total",
	"subtotal",
	"sub-total",
	"total sales",
	"total purchases",
	"total amount",
	"total vat",
	"total input tax",
	"total output tax",
	"total tax",
	"net vat payable",
	"vat payable",
	"total exempt",
	"total zero-rated",
	"total zero rated",
	"total vatable",
	"total gross",
	"overall total",
	"summary",
}

// isSummaryRow returns true if the row contains a summary/total keyword
// (e.g. "Grand Total", "Subtotal", "TOTAL SALES") that should be excluded
// from transaction data. Only triggers when the keyword appears as a
// dominant value (most columns are empty / the text cell is a label).
func isSummaryRow(row []string, numColumns int) bool {
	if numColumns == 0 {
		return false
	}
	nonEmpty := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			nonEmpty++
		}
	}
	// A summary row typically has very few filled cells relative to data rows
	// (just the label + a total number, maybe 2-4 cells).
	// If more than 60% of columns are filled, it's probably a regular data row
	// that just happens to contain "total" in a description field.
	if numColumns > 4 && float64(nonEmpty)/float64(numColumns) > 0.6 {
		return false
	}

	for _, cell := range row {
		lower := strings.ToLower(strings.TrimSpace(cell))
		if lower == "" {
			continue
		}
		// Check exact match against "total" standalone
		if lower == "total" {
			return true
		}
		for _, kw := range summaryRowKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// isDataRow returns true if the row has enough non-empty cells to be considered
// a real data row (not a sub-header continuation like [, "NUMBER", , , ...]).
func isDataRow(row []string, numColumns int) bool {
	if numColumns <= 3 {
		return !isEmptyRow(row)
	}
	nonEmpty := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			nonEmpty++
		}
	}
	// Require at least 30% of columns filled, or at least 3 cells
	return nonEmpty >= 3 && float64(nonEmpty)/float64(numColumns) >= 0.2
}

// detectHeaderRow scans the first N rows and returns the index of the most
// likely header row.  BIR forms often have merged title cells at the top
// (e.g. "PURCHASE TRANSACTION") followed by metadata rows before the real
// column headers appear.
//
// Scoring considers:
//   - Fill ratio: fraction of non-empty cells
//   - Uniqueness: fraction of unique values among non-empty cells
//   - Text ratio: headers should be mostly text, not numbers
//   - Keyword hits: bonus for containing common header words
//   - Sequential numbering rows (1,2,3,4) are skipped entirely
func detectHeaderRow(rows [][]string) int {
	const headerScanLimit = 20
	const minFillRatio = 0.3

	bestRow := 0
	bestScore := 0.0

	// Find the widest row once.
	totalCols := 0
	for _, r := range rows {
		if len(r) > totalCols {
			totalCols = len(r)
		}
	}
	if totalCols == 0 {
		return 0
	}

	limit := headerScanLimit
	if limit > len(rows) {
		limit = len(rows)
	}

	for i := 0; i < limit; i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		// Skip sequential numbering rows (e.g. 1, 2, 3, 4, 5)
		if isSequentialNumbers(row) {
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

			if looksNumeric(v) {
				numericCount++
			}

			// Check keywords (case-insensitive, check full cell and individual words)
			lower := strings.ToLower(v)
			if _, ok := headerKeywords[lower]; ok {
				keywordHits++
			} else {
				// Check if any keyword is a substring of the cell
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
		if fillRatio < minFillRatio {
			continue
		}

		uniqueness := float64(len(unique)) / float64(nonEmpty)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		keywordRatio := float64(keywordHits) / float64(nonEmpty)

		// Skip all-numeric rows — real headers always contain text
		if textRatio == 0 {
			continue
		}

		// Combined score: fill × uniqueness × text-heaviness + keyword bonus
		score := fillRatio * uniqueness * (0.5 + 0.5*textRatio) * (1.0 + keywordRatio)

		if score > bestScore {
			bestScore = score
			bestRow = i
		}
	}

	return bestRow
}

// detectHeaderZone finds the contiguous block of rows that form the complete
// header (multi-row headers common in BIR forms). Returns the start row, end
// row (inclusive), and the index of the first data row.
//
// Example BIR form:
//
//	Row 10: [TAXABLE, TAXPAYER, ..., AMOUNT OF, AMOUNT OF, ...]   ← super-header
//	Row 11: [MONTH, IDENTIFICATION, ..., GROSS PURCHASE, ...]     ← main header (detected)
//	Row 12: [, NUMBER, ...]                                       ← sub-header
//	Row 13: [(1), (2), (3), ...]                                  ← numbering row
//	Row 14: [10-31-24, 103-314-245, ...]                          ← first data row
func detectHeaderZone(rows [][]string, headerIdx int) (zoneStart, zoneEnd, dataStart int) {
	numCols := 0
	for _, r := range rows {
		if len(r) > numCols {
			numCols = len(r)
		}
	}

	zoneStart = headerIdx
	zoneEnd = headerIdx

	// Look UP from header row: include rows that are text-heavy (super-headers).
	for i := headerIdx - 1; i >= 0; i-- {
		row := rows[i]
		if isEmptyRow(row) {
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
			if looksNumeric(v) {
				numericCount++
			}
		}
		if nonEmpty == 0 {
			break
		}
		fillRatio := float64(nonEmpty) / float64(numCols)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		// Super-header: at least 30% filled AND mostly text AND more than 1 cell
		// (single cell = title row, not a header)
		if fillRatio >= 0.3 && textRatio > 0.7 && nonEmpty > 1 {
			zoneStart = i
		} else {
			break
		}
	}

	// Look DOWN from header row: include sub-header rows (low fill, text).
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		if isEmptyRow(row) {
			break
		}
		if isSequentialNumbers(row) {
			continue // Skip numbering rows, don't include in zone
		}
		nonEmpty := 0
		numericCount := 0
		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v == "" {
				continue
			}
			nonEmpty++
			if looksNumeric(v) {
				numericCount++
			}
		}
		if nonEmpty == 0 {
			break
		}
		fillRatio := float64(nonEmpty) / float64(numCols)
		textRatio := 1.0 - float64(numericCount)/float64(nonEmpty)
		// Sub-header continuation: low fill (≤50%) AND text (not data)
		if fillRatio <= 0.5 && textRatio > 0.5 {
			zoneEnd = i
		} else {
			break // Hit a data row
		}
	}

	// Data starts after the header zone (plus any numbering rows)
	dataStart = zoneEnd + 1
	for dataStart < len(rows) {
		if isSequentialNumbers(rows[dataStart]) || isEmptyRow(rows[dataStart]) {
			dataStart++
			continue
		}
		break
	}

	return zoneStart, zoneEnd, dataStart
}

// mergeHeaderRows merges multiple header rows into a single set of column names
// by concatenating non-empty values per column with spaces.
//
// Example:
//
//	Row 0: [TAXPAYER, , , AMOUNT OF, AMOUNT OF]
//	Row 1: [IDENTIFICATION, , , GROSS PURCHASE, EXEMPT PURCHASE]
//	Row 2: [NUMBER, , , , ]
//	→ ["TAXPAYER IDENTIFICATION NUMBER", "", "", "AMOUNT OF GROSS PURCHASE", "AMOUNT OF EXEMPT PURCHASE"]
func mergeHeaderRows(rows [][]string, start, end int) []string {
	if start > end || start < 0 || end >= len(rows) {
		if start >= 0 && start < len(rows) {
			return rows[start]
		}
		return nil
	}

	// Find max width
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

// rowsToSheetData converts raw string rows to SheetData with smart header detection.
func rowsToSheetData(rows [][]string) *SheetData {
	if len(rows) < 1 {
		return nil
	}

	// Detect the real header row, then expand to the full header zone.
	headerIdx := detectHeaderRow(rows)
	zoneStart, zoneEnd, dataStart := detectHeaderZone(rows, headerIdx)

	// Merge multi-row headers into single column names.
	var columns []string
	if zoneStart == zoneEnd {
		// Single header row — use as-is.
		for _, h := range rows[headerIdx] {
			columns = append(columns, strings.TrimSpace(h))
		}
	} else {
		columns = mergeHeaderRows(rows, zoneStart, zoneEnd)
	}

	// Remove empty trailing columns.
	for len(columns) > 0 && columns[len(columns)-1] == "" {
		columns = columns[:len(columns)-1]
	}

	// Replace any remaining empty column names with positional labels.
	for i, c := range columns {
		if c == "" {
			columns[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}

	dataRows := rows[dataStart:]
	numCols := len(columns)

	// Count real data rows (skip empty, sub-headers, numbering rows, and summary/total rows).
	var nonEmptyCount int
	for _, row := range dataRows {
		if !isDataRow(row, numCols) || isSequentialNumbers(row) || isSummaryRow(row, numCols) {
			continue
		}
		nonEmptyCount++
	}

	if nonEmptyCount == 0 && len(dataRows) > 0 {
		// Only headers, no data.
		return &SheetData{
			Columns:  columns,
			RowCount: 0,
			Preview:  []map[string]interface{}{},
		}
	}

	// Build preview (up to MaxPreviewRows real data rows).
	var preview []map[string]interface{}
	for _, row := range dataRows {
		if !isDataRow(row, numCols) || isSequentialNumbers(row) || isSummaryRow(row, numCols) {
			continue
		}
		if len(preview) >= MaxPreviewRows {
			break
		}
		m := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			if i < len(row) {
				val := strings.TrimSpace(row[i])
				if val == "" {
					m[col] = nil
				} else {
					m[col] = val
				}
			} else {
				m[col] = nil
			}
		}
		preview = append(preview, m)
	}

	return &SheetData{
		Columns:  columns,
		RowCount: nonEmptyCount,
		Preview:  preview,
	}
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// ParseUploadedFileAllRows re-parses a file and returns ALL rows (not just preview) for a given sheet.
func ParseUploadedFileAllRows(content []byte, filename, targetSheet string) ([]map[string]interface{}, error) {
	rawSheets, err := extractRawRows(content, filename)
	if err != nil {
		return nil, err
	}

	sheetName := targetSheet
	if sheetName == "" {
		for name := range rawSheets {
			sheetName = name
			break
		}
	}

	rows, ok := rawSheets[sheetName]
	if !ok {
		return nil, fmt.Errorf("sheet %q not found", sheetName)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("file has no data rows")
	}

	columns, dataStart := resolveHeadersHeuristic(rows)
	return buildAllRowsFromHeaders(rows, columns, dataStart, len(rows)-1), nil
}

// ---------------------------------------------------------------------------
// Shared helpers: raw row extraction, header resolution, SheetData building
// ---------------------------------------------------------------------------

// extractRawRows extracts raw string rows from an Excel or CSV file.
// Returns an ordered map of sheet name → rows.
func extractRawRows(content []byte, filename string) (map[string][][]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".xlsx", ".xls":
		return extractExcelRawRows(content)
	case ".csv":
		rows, err := extractCSVRawRows(content)
		if err != nil {
			return nil, err
		}
		return map[string][][]string{"Sheet1": rows}, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s (accepted: .xlsx, .xls, .csv)", ext)
	}
}

func extractExcelRawRows(content []byte) (map[string][][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("cannot open Excel file: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheetList := f.GetSheetList()
	if len(sheetList) > MaxSheets {
		return nil, fmt.Errorf("file has too many sheets (%d); maximum is %d", len(sheetList), MaxSheets)
	}

	result := make(map[string][][]string)
	for _, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheetName, err)
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) > MaxRowsPerSheet {
			return nil, fmt.Errorf("sheet %q has too many rows (%d); maximum is %d", sheetName, len(rows), MaxRowsPerSheet)
		}
		result[sheetName] = rows
	}

	return result, nil
}

func extractCSVRawRows(content []byte) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	var rows [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV parse error: %w", err)
		}
		rows = append(rows, record)
		if len(rows) > MaxRowsPerSheet {
			return nil, fmt.Errorf("CSV exceeds maximum row limit of %d", MaxRowsPerSheet)
		}
	}

	return rows, nil
}

// resolveHeadersHeuristic uses the rule-based heuristic to detect columns and data start.
func resolveHeadersHeuristic(rows [][]string) (columns []string, dataStart int) {
	headerIdx := detectHeaderRow(rows)
	zoneStart, zoneEnd, ds := detectHeaderZone(rows, headerIdx)

	if zoneStart == zoneEnd {
		for _, h := range rows[headerIdx] {
			columns = append(columns, strings.TrimSpace(h))
		}
	} else {
		columns = mergeHeaderRows(rows, zoneStart, zoneEnd)
	}

	columns = cleanupColumns(columns)
	return columns, ds
}

// resolveHeadersWithAI tries AI first, falls back to heuristic on failure.
// Returns columns, dataStart, dataEnd (inclusive, -1 means until end of rows).
func resolveHeadersWithAI(ctx context.Context, ai *openai.Client, rows [][]string) (columns []string, dataStart int, dataEnd int) {
	if ai != nil {
		result, err := DetectHeadersWithAI(ctx, ai, rows)
		if err == nil && len(result.Columns) > 0 {
			return result.Columns, result.DataStartRow, result.DataEndRow
		}
		slog.Warn("AI header detection failed, falling back to heuristic", "error", err)
	}
	cols, start := resolveHeadersHeuristic(rows)
	return cols, start, len(rows) - 1
}

// cleanupColumns removes trailing empty columns and replaces empty names with positional labels.
func cleanupColumns(columns []string) []string {
	for len(columns) > 0 && columns[len(columns)-1] == "" {
		columns = columns[:len(columns)-1]
	}
	for i, c := range columns {
		if c == "" {
			columns[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}
	return columns
}

// buildSheetDataFromHeaders builds a SheetData from pre-resolved columns and data boundaries.
// dataEnd is inclusive (-1 means until end of rows).
func buildSheetDataFromHeaders(rows [][]string, columns []string, dataStart, dataEnd int) *SheetData {
	if len(columns) == 0 {
		return nil
	}

	if dataEnd < 0 || dataEnd >= len(rows) {
		dataEnd = len(rows) - 1
	}
	dataRows := rows[dataStart : dataEnd+1]
	numCols := len(columns)

	var nonEmptyCount int
	for _, row := range dataRows {
		if !isDataRow(row, numCols) || isSequentialNumbers(row) || isSummaryRow(row, numCols) {
			continue
		}
		nonEmptyCount++
	}

	var preview []map[string]interface{}
	for _, row := range dataRows {
		if !isDataRow(row, numCols) || isSequentialNumbers(row) || isSummaryRow(row, numCols) {
			continue
		}
		if len(preview) >= MaxPreviewRows {
			break
		}
		preview = append(preview, rowToMap(row, columns))
	}

	if preview == nil {
		preview = []map[string]interface{}{}
	}

	return &SheetData{
		Columns:  columns,
		RowCount: nonEmptyCount,
		Preview:  preview,
	}
}

// buildAllRowsFromHeaders builds all data rows from pre-resolved columns and data boundaries.
// dataEnd is inclusive (-1 means until end of rows).
func buildAllRowsFromHeaders(rows [][]string, columns []string, dataStart, dataEnd int) []map[string]interface{} {
	if dataEnd < 0 || dataEnd >= len(rows) {
		dataEnd = len(rows) - 1
	}
	numCols := len(columns)
	var result []map[string]interface{}
	for _, row := range rows[dataStart : dataEnd+1] {
		if !isDataRow(row, numCols) || isSequentialNumbers(row) || isSummaryRow(row, numCols) {
			continue
		}
		result = append(result, rowToMap(row, columns))
	}
	return result
}

// rowToMap converts a raw row to a column-name-keyed map.
func rowToMap(row []string, columns []string) map[string]interface{} {
	m := make(map[string]interface{}, len(columns))
	for i, col := range columns {
		if i < len(row) {
			val := strings.TrimSpace(row[i])
			if val == "" {
				m[col] = nil
			} else {
				m[col] = val
			}
		} else {
			m[col] = nil
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// AI-enhanced parsing (used by handlers when OpenAI client is available)
// ---------------------------------------------------------------------------

// ParseUploadedFileWithAI parses a file using AI for header detection.
// Falls back to heuristic if AI is unavailable or fails.
func ParseUploadedFileWithAI(ctx context.Context, ai *openai.Client, content []byte, filename string) (*ParsedFile, error) {
	rawSheets, err := extractRawRows(content, filename)
	if err != nil {
		return nil, err
	}

	if len(rawSheets) == 0 {
		return nil, fmt.Errorf("file contains no data — all sheets are empty")
	}

	fileType := "csv"
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" || ext == ".xls" {
		fileType = "excel"
	}

	sheets := make(map[string]*SheetData)
	for sheetName, rows := range rawSheets {
		if len(rows) == 0 {
			continue
		}
		columns, dataStart, dataEnd := resolveHeadersWithAI(ctx, ai, rows)
		sd := buildSheetDataFromHeaders(rows, columns, dataStart, dataEnd)
		if sd != nil {
			sheets[sheetName] = sd
		}
	}

	if len(sheets) == 0 {
		return nil, fmt.Errorf("file contains no data — all sheets are empty")
	}

	return &ParsedFile{Type: fileType, Sheets: sheets}, nil
}

// ParseUploadedFileAllRowsWithAI re-parses a file with AI header detection
// and returns ALL rows for a given sheet.
func ParseUploadedFileAllRowsWithAI(ctx context.Context, ai *openai.Client, content []byte, filename, targetSheet string) ([]map[string]interface{}, error) {
	rawSheets, err := extractRawRows(content, filename)
	if err != nil {
		return nil, err
	}

	sheetName := targetSheet
	if sheetName == "" {
		for name := range rawSheets {
			sheetName = name
			break
		}
	}

	rows, ok := rawSheets[sheetName]
	if !ok {
		return nil, fmt.Errorf("sheet %q not found", sheetName)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("file has no data rows")
	}

	columns, dataStart, dataEnd := resolveHeadersWithAI(ctx, ai, rows)
	return buildAllRowsFromHeaders(rows, columns, dataStart, dataEnd), nil
}

// ---------------------------------------------------------------------------
// Cleaning pipeline integration
// ---------------------------------------------------------------------------

// CleaningParsedFile extends ParsedFile with cleaning results.
type CleaningParsedFile struct {
	*ParsedFile
	CleaningResults map[string]*cleaning.CleaningResult `json:"cleaning_results,omitempty"`
}

// ParseUploadedFileWithCleaning parses a file using the full cleaning pipeline.
// Falls back to ParseUploadedFileWithAI if the cleaning pipeline fails.
func ParseUploadedFileWithCleaning(
	ctx context.Context,
	ai *openai.Client,
	q *sqlc.Queries,
	content []byte,
	filename string,
	companyID uuid.UUID,
	period string,
) (*CleaningParsedFile, error) {
	rawSheets, err := extractRawRows(content, filename)
	if err != nil {
		return nil, err
	}

	if len(rawSheets) == 0 {
		return nil, fmt.Errorf("file contains no data — all sheets are empty")
	}

	fileType := "csv"
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" || ext == ".xls" {
		fileType = "excel"
	}

	// Build cleaning pipeline
	var aiSvc *cleaning.AISemanticService
	if ai != nil {
		aiSvc = cleaning.NewAISemanticService(ai)
	}
	var templateSvc *cleaning.TemplateService
	if q != nil {
		templateSvc = cleaning.NewTemplateService(q)
	}
	pipeline := cleaning.NewPipeline(aiSvc, templateSvc)

	sheets := make(map[string]*SheetData)
	cleaningResults := make(map[string]*cleaning.CleaningResult)

	for sheetName, rows := range rawSheets {
		if len(rows) == 0 {
			continue
		}

		// Try cleaning pipeline
		result, pipeErr := pipeline.Run(ctx, rows, cleaning.PipelineConfig{
			CompanyID: companyID,
			Period:    period,
		})

		if pipeErr != nil {
			slog.Warn("cleaning pipeline failed, falling back to AI/heuristic",
				"sheet", sheetName,
				"error", pipeErr,
			)
			// Fallback to existing AI-based parsing
			columns, dataStart, dataEnd := resolveHeadersWithAI(ctx, ai, rows)
			sd := buildSheetDataFromHeaders(rows, columns, dataStart, dataEnd)
			if sd != nil {
				sheets[sheetName] = sd
			}
			continue
		}

		// Convert cleaning result to SheetData
		preview := result.DataRows
		if len(preview) > MaxPreviewRows {
			preview = preview[:MaxPreviewRows]
		}
		if preview == nil {
			preview = []map[string]interface{}{}
		}

		sheets[sheetName] = &SheetData{
			Columns:  result.Columns,
			RowCount: len(result.DataRows),
			Preview:  preview,
		}
		cleaningResults[sheetName] = result
	}

	if len(sheets) == 0 {
		return nil, fmt.Errorf("file contains no data — all sheets are empty")
	}

	return &CleaningParsedFile{
		ParsedFile:      &ParsedFile{Type: fileType, Sheets: sheets},
		CleaningResults: cleaningResults,
	}, nil
}

// ParseUploadedFileAllRowsWithCleaning returns ALL rows for a given sheet using
// the cleaning pipeline. Falls back to ParseUploadedFileAllRowsWithAI on failure.
func ParseUploadedFileAllRowsWithCleaning(
	ctx context.Context,
	ai *openai.Client,
	q *sqlc.Queries,
	content []byte,
	filename, targetSheet string,
	companyID uuid.UUID,
	period string,
) ([]map[string]interface{}, *cleaning.CleaningReport, error) {
	rawSheets, err := extractRawRows(content, filename)
	if err != nil {
		return nil, nil, err
	}

	sheetName := targetSheet
	if sheetName == "" {
		for name := range rawSheets {
			sheetName = name
			break
		}
	}

	rows, ok := rawSheets[sheetName]
	if !ok {
		return nil, nil, fmt.Errorf("sheet %q not found", sheetName)
	}

	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("file has no data rows")
	}

	// Try cleaning pipeline
	var aiSvc *cleaning.AISemanticService
	if ai != nil {
		aiSvc = cleaning.NewAISemanticService(ai)
	}
	var templateSvc *cleaning.TemplateService
	if q != nil {
		templateSvc = cleaning.NewTemplateService(q)
	}
	pipeline := cleaning.NewPipeline(aiSvc, templateSvc)

	result, pipeErr := pipeline.Run(ctx, rows, cleaning.PipelineConfig{
		CompanyID: companyID,
		Period:    period,
	})

	if pipeErr != nil {
		slog.Warn("cleaning pipeline failed for all-rows, falling back",
			"error", pipeErr,
		)
		allRows, err := ParseUploadedFileAllRowsWithAI(ctx, ai, content, filename, targetSheet)
		return allRows, nil, err
	}

	return result.DataRows, &result.Report, nil
}
