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

// rowsToSheetData converts raw string rows (first row = headers) to SheetData.
func rowsToSheetData(rows [][]string) *SheetData {
	if len(rows) < 1 {
		return nil
	}

	// First row as headers.
	headers := rows[0]
	columns := make([]string, len(headers))
	for i, h := range headers {
		columns[i] = strings.TrimSpace(h)
	}

	dataRows := rows[1:]

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
