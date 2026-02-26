package service

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// GenerateTransactionExcel creates an .xlsx file from transaction data.
func GenerateTransactionExcel(w io.Writer, txns []TransactionResponse, period string, company CompanyInfo) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Transactions"
	f.SetSheetName("Sheet1", sheet)

	// Styles
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5496"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})
	amountStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4, // #,##0.00
	})
	pctStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt: 10, // 0.00%
	})

	// Title row
	f.SetCellValue(sheet, "A1", fmt.Sprintf("%s — Transactions (%s)", company.CompanyName, period))
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	f.MergeCell(sheet, "A1", "K1")

	// Headers
	headers := []string{
		"Date", "Description", "Amount", "VAT Amount", "VAT Type",
		"Category", "TIN", "Confidence", "Source", "Match Status", "Source Type",
	}
	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheet, fmt.Sprintf("%s3", col), h)
	}
	f.SetCellStyle(sheet, "A3", fmt.Sprintf("%s3", colName(len(headers))), headerStyle)

	// Data
	for i, txn := range txns {
		row := i + 4
		date := ""
		if txn.Date != nil {
			date = *txn.Date
		}
		desc := ""
		if txn.Description != nil {
			desc = *txn.Description
		}
		tin := ""
		if txn.TIN != nil {
			tin = *txn.TIN
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), date)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), desc)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), txn.Amount)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), txn.VATAmount)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), txn.VATType)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), txn.Category)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), tin)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), txn.Confidence)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), txn.ClassificationSource)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", row), txn.MatchStatus)
		f.SetCellValue(sheet, fmt.Sprintf("K%d", row), txn.SourceType)
	}

	// Apply number formats
	lastRow := len(txns) + 3
	if lastRow > 3 {
		f.SetCellStyle(sheet, "C4", fmt.Sprintf("D%d", lastRow), amountStyle)
		f.SetCellStyle(sheet, "H4", fmt.Sprintf("H%d", lastRow), pctStyle)
	}

	// Auto-fit column widths (approximate)
	widths := map[string]float64{
		"A": 12, "B": 35, "C": 15, "D": 15, "E": 12,
		"F": 15, "G": 18, "H": 12, "I": 16, "J": 14, "K": 16,
	}
	for col, width := range widths {
		f.SetColWidth(sheet, col, col, width)
	}

	return f.Write(w)
}

// GenerateReportExcel creates an .xlsx file from BIR report data.
func GenerateReportExcel(w io.Writer, reportType string, calcData map[string]string, company CompanyInfo) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := strings.ReplaceAll(reportType, "_", " ")
	if len(sheet) > 31 { // Excel sheet name limit
		sheet = sheet[:31]
	}
	f.SetSheetName("Sheet1", sheet)

	// Styles
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5496"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	labelStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	amountStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4, // #,##0.00
	})

	// Title
	f.SetCellValue(sheet, "A1", fmt.Sprintf("%s — %s", company.CompanyName, reportType))
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	f.MergeCell(sheet, "A1", "C1")

	// Company info
	f.SetCellValue(sheet, "A2", "TIN:")
	f.SetCellValue(sheet, "B2", company.TINNumber)
	f.SetCellValue(sheet, "A3", "RDO Code:")
	f.SetCellValue(sheet, "B3", company.RDOCode)
	f.SetCellStyle(sheet, "A2", "A3", labelStyle)

	// Header row
	f.SetCellValue(sheet, "A5", "Field")
	f.SetCellValue(sheet, "B5", "Value")
	f.SetCellStyle(sheet, "A5", "B5", headerStyle)

	// Sort fields for consistent output
	keys := make([]string, 0, len(calcData))
	for k := range calcData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Data rows
	row := 6
	for _, key := range keys {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), key)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), calcData[key])
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
		row++
	}

	// Try to format amount cells
	if row > 6 {
		f.SetCellStyle(sheet, "B6", fmt.Sprintf("B%d", row-1), amountStyle)
	}

	// Column widths
	f.SetColWidth(sheet, "A", "A", 30)
	f.SetColWidth(sheet, "B", "B", 25)

	return f.Write(w)
}

// colName returns the Excel column name for a 1-based column number.
func colName(n int) string {
	name, _ := excelize.ColumnNumberToName(n)
	return name
}
