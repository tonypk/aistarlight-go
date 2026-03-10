package service

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// generateGenericPDF produces a generic PDF for unsupported form types.
func generateGenericPDF(w io.Writer, reportType string, data map[string]string, co CompanyInfo) error {
	b := newPDF()

	formName := reportType
	if idx := len("BIR_"); len(reportType) > idx && reportType[:4] == "BIR_" {
		formName = reportType[4:]
	}

	b.titleBanner("BIR Form "+formName, reportType)

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company", co.CompanyName},
		{"TIN", co.TINNumber},
	})

	b.gap(8)
	b.sectionHeader("Report Data")
	for key, value := range data {
		b.lineItem("", key, value, false)
		if b.y > pdfPageH-80 {
			b.pdf.AddPage()
			b.y = 40
		}
	}

	return b.output(w)
}

// ReconciliationPDFInput holds all data needed for a reconciliation report PDF.
type ReconciliationPDFInput struct {
	SessionID  string
	Period     string
	Status     string
	FileCount  int
	Company    CompanyInfo
	VATSummary map[string]string
	MatchStats map[string]interface{}
	Anomalies  []AnomalyEntry
}

// AnomalyEntry represents a single anomaly for the reconciliation PDF.
type AnomalyEntry struct {
	Severity    string
	Description string
	Status      string
}

// GenerateReconciliationPDF produces a VAT reconciliation report PDF.
func GenerateReconciliationPDF(w io.Writer, input ReconciliationPDFInput) error {
	b := newPDF()

	// Title with period
	b.pdf.SetFillColor(30, 58, 95)
	b.pdf.Rect(0, 0, pdfPageW, 70, "F")
	b.pdf.SetTextColor(255, 255, 255)
	b.pdf.SetFont("Helvetica", "B", 18)
	b.centeredCell(16, 20, "VAT Reconciliation Report")
	b.pdf.SetFont("Helvetica", "", 10)
	b.centeredCell(36, 14, "Period: "+input.Period)
	b.pdf.SetFont("Helvetica", "", 7)
	b.centeredCell(52, 12,
		fmt.Sprintf("Company: %s | TIN: %s", input.Company.CompanyName, input.Company.TINNumber))
	b.y = 90

	// Session Info
	b.sectionHeader("Session Information")
	b.pdf.SetFont("Helvetica", "", 8)
	b.pdf.SetTextColor(0, 0, 0)
	items := [][2]string{
		{"Session ID", input.SessionID},
		{"Status", input.Status},
		{"Files", fmt.Sprintf("%d", input.FileCount)},
	}
	for _, item := range items {
		b.pdf.SetXY(pdfMarginL+6, b.y)
		b.pdf.CellFormat(100, 14, item[0]+":", "", 0, "L", false, 0, "")
		b.pdf.SetXY(pdfMarginL+110, b.y)
		b.pdf.CellFormat(200, 14, item[1], "", 0, "L", false, 0, "")
		b.y += 14
	}

	// VAT Summary
	b.gap(8)
	b.sectionHeader("VAT Summary")
	vatItems := [][2]string{
		{"Vatable Sales", "vatable_sales"},
		{"Sales to Government", "sales_to_government"},
		{"Zero-Rated Sales", "zero_rated_sales"},
		{"Exempt Sales", "vat_exempt_sales"},
		{"Total Sales", "total_sales"},
		{"Output VAT", "output_vat"},
		{"Output VAT (Government)", "output_vat_government"},
		{"Total Output VAT", "total_output_vat"},
		{"Input VAT (Goods)", "input_vat_goods"},
		{"Input VAT (Capital)", "input_vat_capital"},
		{"Input VAT (Services)", "input_vat_services"},
		{"Input VAT (Imports)", "input_vat_imports"},
		{"Total Input VAT", "total_input_vat"},
		{"Net VAT", "net_vat"},
	}
	for _, item := range vatItems {
		isTotal := len(item[0]) >= 5 && item[0][:5] == "Total" || item[0] == "Net VAT"
		b.lineItem("", item[0], getD(input.VATSummary, item[1]), isTotal)
	}

	// Match Statistics
	if len(input.MatchStats) > 0 {
		b.gap(12)
		b.sectionHeader("Transaction Matching")
		statItems := [][2]string{
			{"Matched Pairs", fmt.Sprintf("%v", input.MatchStats["matched_pairs"])},
			{"Unmatched Records", fmt.Sprintf("%v", input.MatchStats["unmatched_records"])},
			{"Unmatched Bank Entries", fmt.Sprintf("%v", input.MatchStats["unmatched_bank"])},
		}
		if rate, ok := input.MatchStats["match_rate"]; ok {
			if f, err := strconv.ParseFloat(fmt.Sprintf("%v", rate), 64); err == nil {
				statItems = append(statItems, [2]string{"Match Rate", fmt.Sprintf("%.1f%%", f*100)})
			}
		}
		b.pdf.SetTextColor(0, 0, 0)
		for _, item := range statItems {
			b.pdf.SetFont("Helvetica", "", 8)
			b.pdf.SetXY(pdfMarginL+6, b.y)
			b.pdf.CellFormat(180, 14, item[0]+":", "", 0, "L", false, 0, "")
			b.pdf.SetFont("Helvetica", "B", 8)
			b.pdf.SetXY(pdfMarginL+196, b.y)
			b.pdf.CellFormat(100, 14, item[1], "", 0, "L", false, 0, "")
			b.y += 14
		}
	}

	// Anomalies
	if len(input.Anomalies) > 0 {
		b.gap(12)
		b.sectionHeader(fmt.Sprintf("Anomalies (%d found)", len(input.Anomalies)))

		limit := len(input.Anomalies)
		if limit > 20 {
			limit = 20
		}
		for _, a := range input.Anomalies[:limit] {
			if b.y > pdfPageH-80 {
				b.pdf.AddPage()
				b.y = 40
			}

			switch a.Severity {
			case "high":
				b.pdf.SetTextColor(239, 68, 68)
			case "medium":
				b.pdf.SetTextColor(245, 158, 11)
			default:
				b.pdf.SetTextColor(107, 114, 128)
			}
			b.pdf.SetFont("Helvetica", "B", 7)
			b.pdf.SetXY(pdfMarginL+6, b.y)
			tag := "[" + strings.ToUpper(a.Severity) + "]"
			b.pdf.CellFormat(40, 12, tag, "", 0, "L", false, 0, "")

			b.pdf.SetTextColor(0, 0, 0)
			b.pdf.SetFont("Helvetica", "", 7)
			desc := a.Description
			if len(desc) > 100 {
				desc = desc[:100]
			}
			b.pdf.SetXY(pdfMarginL+50, b.y)
			b.pdf.CellFormat(pdfContentW-120, 12, desc, "", 0, "L", false, 0, "")

			b.pdf.SetFont("Helvetica", "", 6)
			b.pdf.SetXY(pdfPageW-pdfMarginR-60, b.y)
			b.pdf.CellFormat(60, 12, a.Status, "", 0, "R", false, 0, "")
			b.y += 12
		}
	}

	return b.output(w)
}
