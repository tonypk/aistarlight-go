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
	defer func() { _ = f.Close() }()

	sheet := "Transactions"
	_ = f.SetSheetName("Sheet1", sheet)

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
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("%s — Transactions (%s)", company.CompanyName, period))
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	_ = f.MergeCell(sheet, "A1", "K1")

	// Headers
	headers := []string{
		"Date", "Description", "Amount", "VAT Amount", "VAT Type",
		"Category", "TIN", "Confidence", "Source", "Match Status", "Source Type",
	}
	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s3", col), h)
	}
	_ = f.SetCellStyle(sheet, "A3", fmt.Sprintf("%s3", colName(len(headers))), headerStyle)

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

		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), date)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), desc)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), txn.Amount)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), txn.VATAmount)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), txn.VATType)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), txn.Category)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), tin)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", row), txn.Confidence)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", row), txn.ClassificationSource)
		_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", row), txn.MatchStatus)
		_ = f.SetCellValue(sheet, fmt.Sprintf("K%d", row), txn.SourceType)
	}

	// Apply number formats
	lastRow := len(txns) + 3
	if lastRow > 3 {
		_ = f.SetCellStyle(sheet, "C4", fmt.Sprintf("D%d", lastRow), amountStyle)
		_ = f.SetCellStyle(sheet, "H4", fmt.Sprintf("H%d", lastRow), pctStyle)
	}

	// Auto-fit column widths (approximate)
	widths := map[string]float64{
		"A": 12, "B": 35, "C": 15, "D": 15, "E": 12,
		"F": 15, "G": 18, "H": 12, "I": 16, "J": 14, "K": 16,
	}
	for col, width := range widths {
		_ = f.SetColWidth(sheet, col, col, width)
	}

	return f.Write(w)
}

// GenerateReportExcel creates an .xlsx file from BIR report data.
func GenerateReportExcel(w io.Writer, reportType string, calcData map[string]string, company CompanyInfo) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := strings.ReplaceAll(reportType, "_", " ")
	if len(sheet) > 31 { // Excel sheet name limit
		sheet = sheet[:31]
	}
	_ = f.SetSheetName("Sheet1", sheet)

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
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("%s — %s", company.CompanyName, reportType))
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	_ = f.MergeCell(sheet, "A1", "C1")

	// Company info
	_ = f.SetCellValue(sheet, "A2", "TIN:")
	_ = f.SetCellValue(sheet, "B2", company.TINNumber)
	_ = f.SetCellValue(sheet, "A3", "RDO Code:")
	_ = f.SetCellValue(sheet, "B3", company.RDOCode)
	_ = f.SetCellStyle(sheet, "A2", "A3", labelStyle)

	// Header row
	_ = f.SetCellValue(sheet, "A5", "Field")
	_ = f.SetCellValue(sheet, "B5", "Value")
	_ = f.SetCellStyle(sheet, "A5", "B5", headerStyle)

	// Sort fields for consistent output
	keys := make([]string, 0, len(calcData))
	for k := range calcData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Data rows
	row := 6
	for _, key := range keys {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), key)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), calcData[key])
		_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
		row++
	}

	// Try to format amount cells
	if row > 6 {
		_ = f.SetCellStyle(sheet, "B6", fmt.Sprintf("B%d", row-1), amountStyle)
	}

	// Column widths
	_ = f.SetColWidth(sheet, "A", "A", 30)
	_ = f.SetColWidth(sheet, "B", "B", 25)

	return f.Write(w)
}

// GenerateBookkeepingExcel creates an .xlsx bookkeeping export with transactions and summary sheets.
func GenerateBookkeepingExcel(w io.Writer, txns []TransactionResponse, period string, company CompanyInfo, currencySymbol string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Sheet 1: Transactions
	sheet := "Transactions"
	_ = f.SetSheetName("Sheet1", sheet)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5496"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    []excelize.Border{{Type: "bottom", Color: "000000", Style: 1}},
	})
	amountStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 4})
	pctStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 10})
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	labelStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	totalStyle, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Size: 11},
		NumFmt: 4,
		Border: []excelize.Border{{Type: "top", Color: "000000", Style: 2}},
	})

	// Title
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("%s — Bookkeeping Export (%s)", company.CompanyName, period))
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	_ = f.MergeCell(sheet, "A1", "J1")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Currency: %s", currencySymbol))

	// Headers
	headers := []string{"Date", "Description", "Amount", "VAT Amount", "VAT Type", "Category", "Confidence", "Source Type", "Submitted By", "Receipt"}
	for i, h := range headers {
		col := colName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s4", col), h)
	}
	_ = f.SetCellStyle(sheet, "A4", fmt.Sprintf("%s4", colName(len(headers))), headerStyle)

	// Data
	var totalAmount, totalVAT float64
	for i, txn := range txns {
		row := i + 5
		date := ""
		if txn.Date != nil {
			date = *txn.Date
		}
		desc := ""
		if txn.Description != nil {
			desc = *txn.Description
		}
		submittedBy := ""
		if txn.SubmittedByName != nil {
			submittedBy = *txn.SubmittedByName
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), date)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), desc)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), txn.Amount)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), txn.VATAmount)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), txn.VATType)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), txn.Category)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), txn.Confidence)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", row), txn.SourceType)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", row), submittedBy)
		if txn.ReceiptImageURL != nil {
			cell := fmt.Sprintf("J%d", row)
			_ = f.SetCellValue(sheet, cell, "View Receipt")
			_ = f.SetCellHyperLink(sheet, cell, *txn.ReceiptImageURL, "External")
			linkStyle, _ := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{Color: "0563C1", Underline: "single"},
			})
			_ = f.SetCellStyle(sheet, cell, cell, linkStyle)
		}
		totalAmount += txn.Amount
		totalVAT += txn.VATAmount
	}

	// Summary row
	lastRow := len(txns) + 4
	if lastRow > 4 {
		_ = f.SetCellStyle(sheet, "C5", fmt.Sprintf("D%d", lastRow), amountStyle)
		_ = f.SetCellStyle(sheet, "G5", fmt.Sprintf("G%d", lastRow), pctStyle)

		sumRow := lastRow + 1
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", sumRow), "TOTAL")
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", sumRow), totalAmount)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", sumRow), totalVAT)
		_ = f.SetCellStyle(sheet, fmt.Sprintf("B%d", sumRow), fmt.Sprintf("D%d", sumRow), totalStyle)
	}

	widths := map[string]float64{
		"A": 12, "B": 35, "C": 15, "D": 15, "E": 12, "F": 12, "G": 12, "H": 16, "I": 18, "J": 14,
	}
	for col, width := range widths {
		_ = f.SetColWidth(sheet, col, col, width)
	}

	// Sheet 2: Summary by category and VAT type
	summarySheet := "Summary"
	_, _ = f.NewSheet(summarySheet)

	_ = f.SetCellValue(summarySheet, "A1", "Summary by Category")
	_ = f.SetCellStyle(summarySheet, "A1", "A1", titleStyle)
	_ = f.SetCellValue(summarySheet, "A3", "Category")
	_ = f.SetCellValue(summarySheet, "B3", "Count")
	_ = f.SetCellValue(summarySheet, "C3", "Total Amount")
	_ = f.SetCellValue(summarySheet, "D3", "Total VAT")
	_ = f.SetCellStyle(summarySheet, "A3", "D3", headerStyle)

	// Aggregate by category
	catStats := make(map[string][3]float64) // [count, amount, vat]
	for _, txn := range txns {
		cat := txn.Category
		if cat == "" {
			cat = "uncategorized"
		}
		s := catStats[cat]
		s[0]++
		s[1] += txn.Amount
		s[2] += txn.VATAmount
		catStats[cat] = s
	}
	catKeys := make([]string, 0, len(catStats))
	for k := range catStats {
		catKeys = append(catKeys, k)
	}
	sort.Strings(catKeys)

	row := 4
	for _, cat := range catKeys {
		s := catStats[cat]
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("A%d", row), cat)
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("B%d", row), int(s[0]))
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("C%d", row), s[1])
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("D%d", row), s[2])
		row++
	}
	if row > 4 {
		_ = f.SetCellStyle(summarySheet, "C4", fmt.Sprintf("D%d", row-1), amountStyle)
	}

	// VAT type summary
	vatRow := row + 2
	_ = f.SetCellValue(summarySheet, fmt.Sprintf("A%d", vatRow), "Summary by VAT Type")
	_ = f.SetCellStyle(summarySheet, fmt.Sprintf("A%d", vatRow), fmt.Sprintf("A%d", vatRow), labelStyle)
	vatRow++
	_ = f.SetCellValue(summarySheet, fmt.Sprintf("A%d", vatRow), "VAT Type")
	_ = f.SetCellValue(summarySheet, fmt.Sprintf("B%d", vatRow), "Count")
	_ = f.SetCellValue(summarySheet, fmt.Sprintf("C%d", vatRow), "Total Amount")
	_ = f.SetCellValue(summarySheet, fmt.Sprintf("D%d", vatRow), "Total VAT")
	_ = f.SetCellStyle(summarySheet, fmt.Sprintf("A%d", vatRow), fmt.Sprintf("D%d", vatRow), headerStyle)
	vatRow++

	vatStats := make(map[string][3]float64)
	for _, txn := range txns {
		vt := txn.VATType
		if vt == "" {
			vt = "unknown"
		}
		s := vatStats[vt]
		s[0]++
		s[1] += txn.Amount
		s[2] += txn.VATAmount
		vatStats[vt] = s
	}
	vatKeys := make([]string, 0, len(vatStats))
	for k := range vatStats {
		vatKeys = append(vatKeys, k)
	}
	sort.Strings(vatKeys)

	for _, vt := range vatKeys {
		s := vatStats[vt]
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("A%d", vatRow), vt)
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("B%d", vatRow), int(s[0]))
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("C%d", vatRow), s[1])
		_ = f.SetCellValue(summarySheet, fmt.Sprintf("D%d", vatRow), s[2])
		vatRow++
	}
	if vatRow > row+4 {
		_ = f.SetCellStyle(summarySheet, fmt.Sprintf("C%d", row+4), fmt.Sprintf("D%d", vatRow-1), amountStyle)
	}

	_ = f.SetColWidth(summarySheet, "A", "A", 20)
	_ = f.SetColWidth(summarySheet, "B", "B", 10)
	_ = f.SetColWidth(summarySheet, "C", "C", 18)
	_ = f.SetColWidth(summarySheet, "D", "D", 18)

	return f.Write(w)
}

// colName returns the Excel column name for a 1-based column number.
func colName(n int) string {
	name, _ := excelize.ColumnNumberToName(n)
	return name
}
