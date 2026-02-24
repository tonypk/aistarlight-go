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

// detectHeaderRow scans the first N rows and returns the index of the most
// likely header row.  BIR forms have merged title cells at the top
// (e.g. "PURCHASE TRANSACTION") followed by metadata rows before the real
// column headers appear.  The header row is the one with the highest ratio
// of non-empty, unique cells.
func detectHeaderRow(rows [][]string) int {
	const headerScanLimit = 20
	const minFillRatio = 0.4

	bestRow := 0
	bestScore := 0.0

	limit := headerScanLimit
	if limit > len(rows) {
		limit = len(rows)
	}

	for i := 0; i < limit; i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		// Count the maximum possible columns (widest row in the sheet).
		totalCols := 0
		for _, r := range rows {
			if len(r) > totalCols {
				totalCols = len(r)
			}
		}
		if totalCols == 0 {
			continue
		}

		nonEmpty := 0
		unique := make(map[string]struct{})
		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v != "" {
				nonEmpty++
				unique[v] = struct{}{}
			}
		}

		fillRatio := float64(nonEmpty) / float64(totalCols)
		uniqueness := float64(len(unique)) / float64(max(nonEmpty, 1))
		score := fillRatio * uniqueness

		if fillRatio >= minFillRatio && score > bestScore {
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
