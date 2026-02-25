package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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
	defer f.Close()

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

		// Combined score: fill × uniqueness × text-heaviness + keyword bonus
		score := fillRatio * uniqueness * (0.5 + 0.5*textRatio) * (1.0 + keywordRatio)

		if score > bestScore {
			bestScore = score
			bestRow = i
		}
	}

	return bestRow
}

// rowsToSheetData converts raw string rows to SheetData with smart header detection.
func rowsToSheetData(rows [][]string) *SheetData {
	if len(rows) < 1 {
		return nil
	}

	// Detect the real header row (may not be row 0 for BIR forms).
	headerIdx := detectHeaderRow(rows)

	headers := rows[headerIdx]
	columns := make([]string, len(headers))
	for i, h := range headers {
		columns[i] = strings.TrimSpace(h)
	}

	// Remove empty trailing columns.
	for len(columns) > 0 && columns[len(columns)-1] == "" {
		columns = columns[:len(columns)-1]
	}

	dataRows := rows[headerIdx+1:]

	// Skip fully empty data.
	var nonEmptyCount int
	for _, row := range dataRows {
		if !isEmptyRow(row) {
			nonEmptyCount++
		}
	}

	if nonEmptyCount == 0 && len(dataRows) > 0 {
		// Only headers, no data.
		return &SheetData{
			Columns:  columns,
			RowCount: 0,
			Preview:  []map[string]interface{}{},
		}
	}

	// Build preview (up to MaxPreviewRows non-empty rows).
	var preview []map[string]interface{}
	for _, row := range dataRows {
		if isEmptyRow(row) {
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
	ext := strings.ToLower(filepath.Ext(filename))

	var allRawRows [][]string

	switch ext {
	case ".xlsx", ".xls":
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("open excel: %w", err)
		}
		defer f.Close()

		sheetName := targetSheet
		if sheetName == "" {
			sheets := f.GetSheetList()
			if len(sheets) == 0 {
				return nil, fmt.Errorf("no sheets in file")
			}
			sheetName = sheets[0]
		}
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheetName, err)
		}
		allRawRows = rows

	case ".csv":
		reader := csv.NewReader(bytes.NewReader(content))
		reader.LazyQuotes = true
		reader.TrimLeadingSpace = true
		reader.FieldsPerRecord = -1
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("CSV parse: %w", err)
			}
			allRawRows = append(allRawRows, record)
		}

	default:
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}

	if len(allRawRows) < 2 {
		return nil, fmt.Errorf("file has no data rows")
	}

	headerIdx := detectHeaderRow(allRawRows)
	headers := allRawRows[headerIdx]
	columns := make([]string, len(headers))
	for i, h := range headers {
		columns[i] = strings.TrimSpace(h)
	}
	for len(columns) > 0 && columns[len(columns)-1] == "" {
		columns = columns[:len(columns)-1]
	}

	var result []map[string]interface{}
	for _, row := range allRawRows[headerIdx+1:] {
		if isEmptyRow(row) {
			continue
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
		result = append(result, m)
	}

	return result, nil
}
