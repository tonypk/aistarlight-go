package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestGenerateVATSummary_BasicSales(t *testing.T) {
	transactions := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 100000.0, "vat_amount": 0.0, "vat_type": "vatable", "category": "sale"},
		{"source_type": "sales_record", "amount": 50000.0, "vat_amount": 0.0, "vat_type": "government", "category": "sale"},
		{"source_type": "sales_record", "amount": 20000.0, "vat_amount": 0.0, "vat_type": "zero_rated", "category": "sale"},
		{"source_type": "sales_record", "amount": 10000.0, "vat_amount": 0.0, "vat_type": "exempt", "category": "sale"},
	}

	s := GenerateVATSummary(transactions, "2024-01")

	if s.Period != "2024-01" {
		t.Errorf("Period = %q, want 2024-01", s.Period)
	}
	if s.TransactionCount != 4 {
		t.Errorf("TransactionCount = %d, want 4", s.TransactionCount)
	}

	assertDecimalEqual(t, "VatableSales", s.VatableSales, "100000")
	assertDecimalEqual(t, "SalesToGovernment", s.SalesToGovernment, "50000")
	assertDecimalEqual(t, "ZeroRatedSales", s.ZeroRatedSales, "20000")
	assertDecimalEqual(t, "VATExemptSales", s.VATExemptSales, "10000")
	assertDecimalEqual(t, "TotalSales", s.TotalSales, "180000")

	// Output VAT: 100000 * 0.12 = 12000
	assertDecimalEqual(t, "OutputVAT", s.OutputVAT, "12000")
	// Govt VAT: 50000 * 0.05 = 2500
	assertDecimalEqual(t, "OutputVATGovernment", s.OutputVATGovernment, "2500")
	assertDecimalEqual(t, "TotalOutputVAT", s.TotalOutputVAT, "14500")
}

func TestGenerateVATSummary_Purchases(t *testing.T) {
	transactions := []map[string]interface{}{
		{"source_type": "purchase", "amount": 30000.0, "vat_amount": 3600.0, "vat_type": "", "category": "goods"},
		{"source_type": "purchase", "amount": 10000.0, "vat_amount": 1200.0, "vat_type": "", "category": "services"},
		{"source_type": "purchase", "amount": 50000.0, "vat_amount": 6000.0, "vat_type": "", "category": "capital"},
		{"source_type": "purchase", "amount": 5000.0, "vat_amount": 600.0, "vat_type": "", "category": "imports"},
	}

	s := GenerateVATSummary(transactions, "2024-01")

	assertDecimalEqual(t, "InputVATGoods", s.InputVATGoods, "3600")
	assertDecimalEqual(t, "InputVATServices", s.InputVATServices, "1200")
	assertDecimalEqual(t, "InputVATCapital", s.InputVATCapital, "6000")
	assertDecimalEqual(t, "InputVATImports", s.InputVATImports, "600")
	assertDecimalEqual(t, "TotalInputVAT", s.TotalInputVAT, "11400")
}

func TestGenerateVATSummary_NetVAT(t *testing.T) {
	transactions := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 100000.0, "vat_amount": 0.0, "vat_type": "vatable", "category": "sale"},
		{"source_type": "purchase", "amount": 30000.0, "vat_amount": 3600.0, "vat_type": "", "category": "goods"},
	}

	s := GenerateVATSummary(transactions, "2024-01")

	// Output: 100000 * 0.12 = 12000
	// Input: 3600
	// Net: 12000 - 3600 = 8400
	assertDecimalEqual(t, "NetVAT", s.NetVAT, "8400")
}

func TestGenerateVATSummary_ExplicitOutputTax(t *testing.T) {
	// When vat_amount (output_tax) is explicitly provided, use it instead of amount * 12%
	transactions := []map[string]interface{}{
		// Row with explicit output_tax: vatable_sales=5,743,199.49, output_tax=689,183.94
		{"source_type": "sales_record", "amount": 5743199.49, "vat_amount": 689183.94, "vat_type": "vatable", "category": "sale"},
		// Row with explicit output_tax: vatable_sales=3,410,061.39, output_tax=409,207.37
		{"source_type": "sales_record", "amount": 3410061.39, "vat_amount": 409207.37, "vat_type": "vatable", "category": "sale"},
		// Zero-rated row (no VAT)
		{"source_type": "sales_record", "amount": 4681523.36, "vat_amount": 0.0, "vat_type": "zero_rated", "category": "sale"},
	}

	s := GenerateVATSummary(transactions, "2024-12")

	// Vatable sales = 5,743,199.49 + 3,410,061.39 = 9,153,260.88
	assertDecimalEqual(t, "VatableSales", s.VatableSales, "9153260.88")
	// Zero-rated sales = 4,681,523.36
	assertDecimalEqual(t, "ZeroRatedSales", s.ZeroRatedSales, "4681523.36")
	// Output VAT should use explicit values: 689,183.94 + 409,207.37 = 1,098,391.31
	// NOT computed: 9,153,260.88 * 0.12 = 1,098,391.3056
	assertDecimalEqual(t, "OutputVAT", s.OutputVAT, "1098391.31")
	assertDecimalEqual(t, "TotalOutputVAT", s.TotalOutputVAT, "1098391.31")
}

func TestGenerateVATSummary_Empty(t *testing.T) {
	s := GenerateVATSummary(nil, "2024-01")

	if s.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0", s.TransactionCount)
	}
	assertDecimalEqual(t, "TotalSales", s.TotalSales, "0")
	assertDecimalEqual(t, "TotalInputVAT", s.TotalInputVAT, "0")
	assertDecimalEqual(t, "NetVAT", s.NetVAT, "0")
}

func TestMatchTransactions_ExactMatch(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "R1", "amount": 1000.0, "date": "2024-01-15", "description": "Invoice 001"},
		{"id": "R2", "amount": 2500.0, "date": "2024-01-20", "description": "Invoice 002"},
	}

	bankEntries := []map[string]interface{}{
		{"id": "B1", "amount": 1000.0, "date": "2024-01-15", "description": "Transfer"},
		{"id": "B2", "amount": 2500.0, "date": "2024-01-20", "description": "Deposit"},
	}

	result := MatchTransactions(records, bankEntries, 0.01, 3)

	if len(result.MatchedPairs) != 2 {
		t.Fatalf("Expected 2 matches, got %d", len(result.MatchedPairs))
	}
	if len(result.UnmatchedRecords) != 0 {
		t.Errorf("Expected 0 unmatched records, got %d", len(result.UnmatchedRecords))
	}
	if len(result.UnmatchedBank) != 0 {
		t.Errorf("Expected 0 unmatched bank entries, got %d", len(result.UnmatchedBank))
	}
	if result.MatchRate != 1.0 {
		t.Errorf("Expected match rate 1.0, got %f", result.MatchRate)
	}
}

func TestMatchTransactions_WithTolerance(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "R1", "amount": 1000.0, "date": "2024-01-15"},
	}

	bankEntries := []map[string]interface{}{
		{"id": "B1", "amount": 1000.50, "date": "2024-01-16"}, // small amount diff, 1 day diff
	}

	result := MatchTransactions(records, bankEntries, 1.0, 3)

	if len(result.MatchedPairs) != 1 {
		t.Fatalf("Expected 1 match with tolerance, got %d", len(result.MatchedPairs))
	}
	if result.MatchedPairs[0].DateDiffDays != 1 {
		t.Errorf("Expected date diff 1, got %d", result.MatchedPairs[0].DateDiffDays)
	}
}

func TestMatchTransactions_NoMatch(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "R1", "amount": 1000.0, "date": "2024-01-15"},
	}

	bankEntries := []map[string]interface{}{
		{"id": "B1", "amount": 5000.0, "date": "2024-03-01"}, // too different
	}

	result := MatchTransactions(records, bankEntries, 0.01, 3)

	if len(result.MatchedPairs) != 0 {
		t.Errorf("Expected 0 matches, got %d", len(result.MatchedPairs))
	}
	if len(result.UnmatchedRecords) != 1 {
		t.Errorf("Expected 1 unmatched record, got %d", len(result.UnmatchedRecords))
	}
	if len(result.UnmatchedBank) != 1 {
		t.Errorf("Expected 1 unmatched bank entry, got %d", len(result.UnmatchedBank))
	}
}

func TestMatchTransactions_DefaultTolerance(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "R1", "amount": 1000.0, "date": "2024-01-15"},
	}

	bankEntries := []map[string]interface{}{
		{"id": "B1", "amount": 1000.0, "date": "2024-01-15"},
	}

	// Pass 0/0 to use defaults
	result := MatchTransactions(records, bankEntries, 0, 0)

	if len(result.MatchedPairs) != 1 {
		t.Errorf("Expected 1 match with defaults, got %d", len(result.MatchedPairs))
	}
}

func TestMatchTransactions_Empty(t *testing.T) {
	result := MatchTransactions(nil, nil, 0.01, 3)
	if len(result.MatchedPairs) != 0 {
		t.Errorf("Expected 0 matches for empty input, got %d", len(result.MatchedPairs))
	}
	if result.MatchRate != 0 {
		t.Errorf("Expected match rate 0 for empty, got %f", result.MatchRate)
	}
}

func TestCompareWithDeclared_FullMatch(t *testing.T) {
	summary := VATSummary{
		VatableSales:       decimal.NewFromInt(100000),
		SalesToGovernment:  decimal.NewFromInt(50000),
		ZeroRatedSales:     decimal.NewFromInt(20000),
		VATExemptSales:     decimal.NewFromInt(10000),
		TotalSales:         decimal.NewFromInt(180000),
		OutputVAT:          decimal.NewFromInt(12000),
		OutputVATGovernment: decimal.NewFromInt(2500),
		TotalOutputVAT:     decimal.NewFromInt(14500),
		InputVATGoods:      decimal.NewFromInt(3600),
		InputVATCapital:    decimal.Zero,
		InputVATServices:   decimal.NewFromInt(1200),
		InputVATImports:    decimal.Zero,
		TotalInputVAT:      decimal.NewFromInt(4800),
	}

	declared := map[string]string{
		"vatable_sales":       "100000",
		"sales_to_government": "50000",
		"zero_rated_sales":    "20000",
		"exempt_sales":        "10000",
		"total_sales":         "180000",
		"output_vat":          "12000",
		"output_vat_government": "2500",
		"total_output_vat":    "14500",
		"input_vat_goods":     "3600",
		"input_vat_capital":   "0",
		"input_vat_services":  "1200",
		"input_vat_imports":   "0",
		"total_input_vat":     "4800",
	}

	result := CompareWithDeclared(summary, declared)

	if !result.FullyMatched {
		t.Error("Expected fully matched")
	}
	if result.MatchedLines != result.TotalLines {
		t.Errorf("Expected %d matched lines, got %d", result.TotalLines, result.MatchedLines)
	}
	if result.TotalDifference != "0" {
		t.Errorf("Expected total difference 0, got %s", result.TotalDifference)
	}
}

func TestCompareWithDeclared_Mismatch(t *testing.T) {
	summary := VATSummary{
		VatableSales:  decimal.NewFromInt(100000),
		TotalSales:    decimal.NewFromInt(100000),
		OutputVAT:     decimal.NewFromInt(12000),
		TotalOutputVAT: decimal.NewFromInt(12000),
		TotalInputVAT: decimal.Zero,
	}

	declared := map[string]string{
		"vatable_sales": "90000",  // 10000 difference
		"total_sales":   "90000",
		"output_vat":    "10800",  // different
	}

	result := CompareWithDeclared(summary, declared)

	if result.FullyMatched {
		t.Error("Should not be fully matched with mismatches")
	}
	if result.MatchedLines == result.TotalLines {
		t.Error("Not all lines should match")
	}
}

func TestCompareWithDeclared_EmptyDeclared(t *testing.T) {
	summary := VATSummary{
		VatableSales: decimal.NewFromInt(100000),
	}

	result := CompareWithDeclared(summary, map[string]string{})

	if result.FullyMatched {
		t.Error("Should not fully match with empty declared values")
	}
}

func TestDateDiffDays(t *testing.T) {
	tests := []struct {
		dateA    string
		dateB    string
		expected int
	}{
		{"2024-01-15", "2024-01-15", 0},
		{"2024-01-15", "2024-01-18", 3},
		{"2024-01-18", "2024-01-15", 3}, // absolute
		{"2024-01-01", "2024-02-01", 31},
		{"", "2024-01-15", 0},
		{"2024-01-15", "", 0},
		{"invalid", "2024-01-15", 0},
	}

	for _, tt := range tests {
		got := dateDiffDays(tt.dateA, tt.dateB)
		if got != tt.expected {
			t.Errorf("dateDiffDays(%q, %q) = %d, want %d", tt.dateA, tt.dateB, got, tt.expected)
		}
	}
}

func TestInferRowSourceType(t *testing.T) {
	tests := []struct {
		name     string
		row      map[string]interface{}
		fallback string
		want     string
	}{
		{
			name:     "sales row with gross_sales",
			row:      map[string]interface{}{"gross_sales": "100000", "customer_name": "ABC Corp"},
			fallback: "purchase_record",
			want:     "sales_record",
		},
		{
			name:     "purchase row with gross_purchase",
			row:      map[string]interface{}{"gross_purchase": "50000", "supplier_name": "XYZ Inc"},
			fallback: "sales_record",
			want:     "purchase_record",
		},
		{
			name:     "sales row with vatable_sales and output_tax",
			row:      map[string]interface{}{"vatable_sales": "5743199.49", "output_tax": "689183.94"},
			fallback: "purchase_record",
			want:     "sales_record",
		},
		{
			name:     "purchase row with input_tax only",
			row:      map[string]interface{}{"input_tax": "5467.33", "supplier_tin": "123-456-789"},
			fallback: "sales_record",
			want:     "purchase_record",
		},
		{
			name:     "ambiguous row falls back to default",
			row:      map[string]interface{}{"amount": "10000", "description": "Payment"},
			fallback: "purchase_record",
			want:     "purchase_record",
		},
		{
			name:     "combined row with both signals uses fallback",
			row:      map[string]interface{}{"gross_sales": "100000", "gross_purchase": "50000"},
			fallback: "sales_record",
			want:     "sales_record",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferRowSourceType(tt.row, tt.fallback)
			if got != tt.want {
				t.Errorf("inferRowSourceType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// assertDecimalEqual is a test helper for decimal comparisons.
func assertDecimalEqual(t *testing.T, name string, got decimal.Decimal, wantStr string) {
	t.Helper()
	want := decimal.RequireFromString(wantStr)
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}
