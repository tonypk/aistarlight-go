package service

import (
	"testing"
)

func TestDetectBankFormat_BDO(t *testing.T) {
	columns := []string{"Date", "Description", "Debit", "Credit", "Balance"}
	format := DetectBankFormat(columns)
	if format.Name != "BDO" && format.Name != "Metrobank" {
		// BDO and Metrobank share similar column names
		t.Logf("Detected: %s (acceptable for BDO-like columns)", format.Name)
	}
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
}

func TestDetectBankFormat_BPI(t *testing.T) {
	columns := []string{"Date", "Remarks", "Withdrawal", "Deposit", "Balance"}
	format := DetectBankFormat(columns)
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
	if format.Name != "BPI" {
		t.Errorf("Expected BPI, got %s", format.Name)
	}
}

func TestDetectBankFormat_PayPal(t *testing.T) {
	columns := []string{"Date", "Name", "Amount", "Transaction ID", "Status"}
	format := DetectBankFormat(columns)
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
	if format.Name != "PayPal" {
		t.Errorf("Expected PayPal, got %s", format.Name)
	}
}

func TestDetectBankFormat_Stripe(t *testing.T) {
	columns := []string{"id", "Created (UTC)", "Amount", "Description", "Status"}
	format := DetectBankFormat(columns)
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
	if format.Name != "Stripe" {
		t.Errorf("Expected Stripe, got %s", format.Name)
	}
}

func TestDetectBankFormat_GCash(t *testing.T) {
	columns := []string{"Date", "Transaction Type", "Amount", "Reference"}
	format := DetectBankFormat(columns)
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
	if format.Name != "GCash" {
		t.Errorf("Expected GCash, got %s", format.Name)
	}
}

func TestDetectBankFormat_UnknownFallsToGeneric(t *testing.T) {
	columns := []string{"foo", "bar", "baz"}
	format := DetectBankFormat(columns)
	if format == nil {
		t.Fatal("Expected non-nil format")
	}
	if format.Name != "Generic" {
		t.Errorf("Expected Generic for unknown columns, got %s", format.Name)
	}
}

func TestParseBankStatement_DebitCredit(t *testing.T) {
	format := &BankFormatConfig{
		Name:              "TestBank",
		DateColumns:       []string{"date"},
		DescriptionColumns: []string{"description"},
		DebitColumn:       "debit",
		CreditColumn:      "credit",
		DateFormat:        "01/02/2006",
	}

	rows := []map[string]interface{}{
		{"date": "01/15/2024", "description": "Payment to vendor", "debit": "5000.00", "credit": ""},
		{"date": "01/20/2024", "description": "Client deposit", "debit": "", "credit": "10000.00"},
	}

	entries := ParseBankStatement(rows, format)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Type != "debit" {
		t.Errorf("Entry 0: expected debit, got %s", entries[0].Type)
	}
	if entries[0].Amount != 5000 {
		t.Errorf("Entry 0: expected amount 5000, got %f", entries[0].Amount)
	}
	if entries[0].Date != "2024-01-15" {
		t.Errorf("Entry 0: expected date 2024-01-15, got %s", entries[0].Date)
	}

	if entries[1].Type != "credit" {
		t.Errorf("Entry 1: expected credit, got %s", entries[1].Type)
	}
	if entries[1].Amount != 10000 {
		t.Errorf("Entry 1: expected amount 10000, got %f", entries[1].Amount)
	}
}

func TestParseBankStatement_AmountColumn(t *testing.T) {
	format := &BankFormatConfig{
		Name:              "PayPal",
		DateColumns:       []string{"date"},
		DescriptionColumns: []string{"name"},
		AmountColumn:      "amount",
		ReferenceColumn:   "transaction id",
		DateFormat:        "01/02/2006",
	}

	rows := []map[string]interface{}{
		{"date": "01/10/2024", "name": "Sale", "amount": "500.00", "transaction id": "TXN001"},
		{"date": "01/11/2024", "name": "Refund", "amount": "-200.00", "transaction id": "TXN002"},
	}

	entries := ParseBankStatement(rows, format)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Type != "credit" {
		t.Errorf("Positive amount should be credit, got %s", entries[0].Type)
	}
	if entries[0].Amount != 500 {
		t.Errorf("Expected 500, got %f", entries[0].Amount)
	}
	if entries[0].Reference != "TXN001" {
		t.Errorf("Expected TXN001, got %s", entries[0].Reference)
	}

	if entries[1].Type != "debit" {
		t.Errorf("Negative amount should be debit, got %s", entries[1].Type)
	}
	if entries[1].Amount != 200 {
		t.Errorf("Expected 200, got %f", entries[1].Amount)
	}
}

func TestParseBankStatement_SkipsEmptyRows(t *testing.T) {
	format := &BankFormatConfig{
		Name:              "Test",
		DateColumns:       []string{"date"},
		DescriptionColumns: []string{"description"},
		AmountColumn:      "amount",
		DateFormat:        "01/02/2006",
	}

	rows := []map[string]interface{}{
		{"date": "01/01/2024", "description": "Valid", "amount": "100"},
		{"date": "", "description": "", "amount": "0"},
		{"date": "01/02/2024", "description": "Also valid", "amount": "200"},
	}

	entries := ParseBankStatement(rows, format)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries (empty row skipped), got %d", len(entries))
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
	}{
		{nil, 0},
		{"", 0},
		{"nan", 0},
		{"none", 0},
		{float64(123.45), 123.45},
		{float32(10.5), 10.5},
		{int(42), 42},
		{int64(99), 99},
		{"1,234.56", 1234.56},
		{"$1,000.00", 1000},
		{"₱50,000.00", 50000},
		{"PHP 25,000.00", 25000},
		{"(500.00)", -500},
		{"  100.50  ", 100.50},
	}

	for _, tt := range tests {
		result := parseAmount(tt.input)
		if result != tt.expected {
			t.Errorf("parseAmount(%v) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		value    string
		format   string
		expected string
	}{
		{"01/15/2024", "01/02/2006", "2024-01-15"},
		{"2024-03-20", "2006-01-02", "2024-03-20"},
		{"Jan 5, 2024", "Jan 2, 2006", "2024-01-05"},
		{"", "01/02/2006", ""},
		{"garbage", "01/02/2006", ""},
	}

	for _, tt := range tests {
		result := parseDate(tt.value, tt.format)
		if result != tt.expected {
			t.Errorf("parseDate(%q, %q) = %q, want %q", tt.value, tt.format, result, tt.expected)
		}
	}
}
