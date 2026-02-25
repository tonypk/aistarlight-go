package cleaning

import (
	"testing"
)

func newTestClassifier() *RowClassifier {
	headers := []string{"Date", "Supplier Name", "TIN", "Gross Purchase", "Input Tax"}
	return NewRowClassifier(headers, 5, DefaultClassifierConfig())
}

func TestClassifyRow_GrandTotal(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"", "Grand Total", "", "15000.00", "1800.00"}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeGrandTotal {
		t.Errorf("expected grand_total, got %s (reason: %s)", cls.Type, cls.Reason)
	}
	if cls.Confidence < 0.4 {
		t.Errorf("confidence too low: %f", cls.Confidence)
	}
}

func TestClassifyRow_Subtotal(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"", "Total Sales", "", "5000.00", ""}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeSubtotal && cls.Type != RowTypeGrandTotal {
		t.Errorf("expected subtotal or grand_total, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_DataRow(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"01/15/2024", "ABC Trading Co.", "123-456-789-000", "5000.00", "600.00"}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeData {
		t.Errorf("expected data, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_FalsePositiveProtection_TotalLogistics(t *testing.T) {
	// "Total Logistics Inc." should NOT be classified as a total row
	// because it has a date, TIN, and high fill ratio.
	rc := newTestClassifier()
	row := []string{"2024-01-15", "Total Logistics Inc.", "123-456-789-000", "5000.00", "600.00"}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeData {
		t.Errorf("'Total Logistics Inc.' should be data, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_BlankRow(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"", "", "", "", ""}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeBlank {
		t.Errorf("expected blank, got %s", cls.Type)
	}
	if cls.Confidence != 1.0 {
		t.Errorf("blank confidence should be 1.0, got %f", cls.Confidence)
	}
}

func TestClassifyRow_NumberingRow(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"(1)", "(2)", "(3)", "(4)", "(5)"}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeNumbering {
		t.Errorf("expected numbering, got %s", cls.Type)
	}
}

func TestClassifyRow_HeaderRepeat(t *testing.T) {
	rc := newTestClassifier()
	// Row that repeats the header columns (common in multi-page BIR forms)
	row := []string{"Date", "Supplier Name", "TIN", "Gross Purchase", "Input Tax"}
	cls := rc.ClassifyRow(row, 20, nil, 5)
	if cls.Type != RowTypeHeaderRepeat {
		t.Errorf("expected header_repeat, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_NoteRow(t *testing.T) {
	rc := newTestClassifier()
	row := []string{"", "Prepared by: John", "", "", ""}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeNote {
		t.Errorf("expected note, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_SumCheck(t *testing.T) {
	rc := newTestClassifier()

	// Build rows where the last row is a subtotal of the preceding rows
	allRows := CellGrid{
		{"Date", "Name", "TIN", "Amount", "Tax"},       // 0: header
		{"01/01", "A", "123-456-789", "1000.00", "120"}, // 1: data
		{"01/02", "B", "234-567-890", "2000.00", "240"}, // 2: data
		{"01/03", "C", "345-678-901", "3000.00", "360"}, // 3: data
		{"", "Sub Total", "", "6000.00", "720"},          // 4: subtotal
	}

	cls := rc.ClassifyRow(allRows[4], 4, allRows, 1)
	if cls.Type != RowTypeSubtotal && cls.Type != RowTypeGrandTotal {
		t.Errorf("expected subtotal/grand_total for sum-check row, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRows_Mixed(t *testing.T) {
	rc := newTestClassifier()

	rows := CellGrid{
		{"Date", "Supplier Name", "TIN", "Gross Purchase", "Input Tax"}, // 0: header
		{"(1)", "(2)", "(3)", "(4)", "(5)"},                              // 1: numbering
		{"01/15/2024", "Vendor A", "111-222-333-000", "5000.00", "600.00"}, // 2: data
		{"01/20/2024", "Vendor B", "444-555-666-000", "3000.00", "360.00"}, // 3: data
		{"", "", "", "", ""},                                              // 4: blank
		{"01/25/2024", "Total Trading Corp.", "777-888-999-000", "2000.00", "240.00"}, // 5: data (false positive)
		{"", "Total Purchases", "", "10000.00", "1200.00"},               // 6: subtotal
		{"", "Grand Total", "", "10000.00", "1200.00"},                   // 7: grand total
		{"", "Prepared by: Accountant", "", "", ""},                      // 8: note
	}

	results := rc.ClassifyRows(rows, 2, 8)
	if len(results) != 7 {
		t.Fatalf("expected 7 classifications, got %d", len(results))
	}

	// Row 2: data
	if results[0].Type != RowTypeData {
		t.Errorf("row 2: expected data, got %s", results[0].Type)
	}
	// Row 3: data
	if results[1].Type != RowTypeData {
		t.Errorf("row 3: expected data, got %s", results[1].Type)
	}
	// Row 4: blank
	if results[2].Type != RowTypeBlank {
		t.Errorf("row 4: expected blank, got %s", results[2].Type)
	}
	// Row 5: "Total Trading Corp." should be data (false positive protection)
	if results[3].Type != RowTypeData {
		t.Errorf("row 5 'Total Trading Corp.': expected data, got %s (reason: %s)", results[3].Type, results[3].Reason)
	}
	// Row 7: grand total
	if results[5].Type != RowTypeGrandTotal {
		t.Errorf("row 7: expected grand_total, got %s (reason: %s)", results[5].Type, results[5].Reason)
	}
	// Row 8: note
	if results[6].Type != RowTypeNote {
		t.Errorf("row 8: expected note, got %s (reason: %s)", results[6].Type, results[6].Reason)
	}
}

func TestLooksLikeDate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"01/15/2024", true},
		{"2024-01-15", true},
		{"12-31-2024", true},
		{"Hello", false},
		{"123", false},
		{"01-15-2024", true},
		{"1/5/24", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := looksLikeDate(tt.input); got != tt.want {
				t.Errorf("looksLikeDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLooksLikeTINOrDocNo(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123-456-789-000", true},
		{"123-456-789", true},
		{"Hello", false},
		{"12345", false},
		{"99-1234567", false}, // only 1 dash, but let's see
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := looksLikeTINOrDocNo(tt.input); got != tt.want {
				t.Errorf("looksLikeTINOrDocNo(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyRow_StandaloneTotal(t *testing.T) {
	rc := newTestClassifier()
	// Just the word "Total" in a sparse row
	row := []string{"", "Total", "", "50000.00", ""}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeGrandTotal {
		t.Errorf("standalone 'Total' should be grand_total, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}

func TestClassifyRow_HighFillTotalInDescription(t *testing.T) {
	// A highly-filled row with "total" in a description field should be data
	// if it has date + TIN
	rc := NewRowClassifier(
		[]string{"Date", "Description", "TIN", "Debit", "Credit", "Balance", "Ref"},
		7,
		DefaultClassifierConfig(),
	)
	row := []string{"01/15/2024", "Total invoice for services", "123-456-789-000", "5000", "0", "15000", "INV-001"}
	cls := rc.ClassifyRow(row, 10, nil, 5)
	if cls.Type != RowTypeData {
		t.Errorf("high-fill row with 'total' in description should be data, got %s (reason: %s)", cls.Type, cls.Reason)
	}
}
