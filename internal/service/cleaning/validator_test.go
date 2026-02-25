package cleaning

import (
	"testing"
	"time"
)

func TestValidateDates_InvalidFormat(t *testing.T) {
	rows := []map[string]interface{}{
		{"Date": "01/15/2024"},
		{"Date": "not-a-date"},
		{"Date": "2024-02-28"},
	}
	mapping := map[string]FieldMapping{
		"Date": {TargetField: "purchase_date", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()
	cfg.TableType = "purchase"

	issues := Validate(rows, []string{"Date"}, mapping, cfg)

	dateIssues := 0
	for _, iss := range issues {
		if iss.Field == "purchase_date" && iss.Message == "cannot parse as date" {
			dateIssues++
		}
	}
	if dateIssues != 1 {
		t.Errorf("expected 1 unparseable date issue, got %d", dateIssues)
	}
}

func TestValidateDates_PeriodMismatch(t *testing.T) {
	// 3 out of 4 dates are outside the expected period
	rows := []map[string]interface{}{
		{"Date": "01/15/2024"}, // Jan
		{"Date": "03/01/2024"}, // Mar
		{"Date": "04/01/2024"}, // Apr
		{"Date": "05/01/2024"}, // May
	}
	mapping := map[string]FieldMapping{
		"Date": {TargetField: "sales_date", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()
	cfg.Period = "2024-01" // January 2024
	cfg.TableType = "sales"

	issues := Validate(rows, []string{"Date"}, mapping, cfg)

	hasPeriodWarning := false
	for _, iss := range issues {
		if iss.RowIndex == -1 && iss.Message == "more than 20% of dates are outside the expected period" {
			hasPeriodWarning = true
		}
	}
	if !hasPeriodWarning {
		t.Error("expected period mismatch warning")
	}
}

func TestValidateAmounts_InvalidFormat(t *testing.T) {
	rows := []map[string]interface{}{
		{"Amount": "1,234.56"},
		{"Amount": "abc"},
		{"Amount": "₱5,000.00"},
	}
	mapping := map[string]FieldMapping{
		"Amount": {TargetField: "gross_purchase", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()

	issues := Validate(rows, []string{"Amount"}, mapping, cfg)

	amountIssues := 0
	for _, iss := range issues {
		if iss.Field == "gross_purchase" && iss.Message == "cannot parse as numeric amount" {
			amountIssues++
		}
	}
	if amountIssues != 1 {
		t.Errorf("expected 1 unparseable amount issue, got %d", amountIssues)
	}
}

func TestValidateTINs(t *testing.T) {
	rows := []map[string]interface{}{
		{"TIN": "123-456-789-000"}, // valid 12-digit
		{"TIN": "123-456-789"},     // valid 9-digit
		{"TIN": "12345"},           // invalid
		{"TIN": "abc-def-ghi"},     // invalid
	}
	mapping := map[string]FieldMapping{
		"TIN": {TargetField: "supplier_tin", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()

	issues := Validate(rows, []string{"TIN"}, mapping, cfg)

	tinIssues := 0
	for _, iss := range issues {
		if iss.Field == "supplier_tin" {
			tinIssues++
		}
	}
	if tinIssues != 2 {
		t.Errorf("expected 2 TIN issues, got %d", tinIssues)
	}
}

func TestValidateDuplicates(t *testing.T) {
	rows := []map[string]interface{}{
		{"InvNo": "INV-001", "Date": "01/15/2024", "Amount": "1000"},
		{"InvNo": "INV-002", "Date": "01/16/2024", "Amount": "2000"},
		{"InvNo": "INV-001", "Date": "01/15/2024", "Amount": "1000"}, // duplicate
	}
	mapping := map[string]FieldMapping{
		"InvNo":  {TargetField: "purchase_invoice_number", Confidence: 0.9},
		"Date":   {TargetField: "purchase_date", Confidence: 0.9},
		"Amount": {TargetField: "gross_purchase", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()

	issues := Validate(rows, []string{"InvNo", "Date", "Amount"}, mapping, cfg)

	dupIssues := 0
	for _, iss := range issues {
		if iss.Field == "duplicate" {
			dupIssues++
		}
	}
	if dupIssues != 1 {
		t.Errorf("expected 1 duplicate issue, got %d", dupIssues)
	}
}

func TestValidateRequiredFields_Missing(t *testing.T) {
	rows := []map[string]interface{}{
		{"Date": "01/15/2024", "Amount": "1000"},
	}
	// Only date and amount mapped, but purchase requires supplier_name
	mapping := map[string]FieldMapping{
		"Date":   {TargetField: "purchase_date", Confidence: 0.9},
		"Amount": {TargetField: "gross_purchase", Confidence: 0.9},
	}
	cfg := DefaultValidatorConfig()
	cfg.TableType = "purchase"

	issues := Validate(rows, []string{"Date", "Amount"}, mapping, cfg)

	hasRequired := false
	for _, iss := range issues {
		if iss.Field == "supplier_name" && iss.Message == "required field not mapped to any column" {
			hasRequired = true
		}
	}
	if !hasRequired {
		t.Error("expected required field warning for supplier_name")
	}
}

func TestTryParseDate(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
		month time.Month
		day   int
	}{
		{"01/15/2024", true, time.January, 15},
		{"2024-01-15", true, time.January, 15},
		{"1/5/2024", true, time.January, 5},
		{"not-a-date", false, 0, 0},
		{"", false, 0, 0},
		{"13/02/2024", true, time.February, 13}, // DD/MM/YYYY (day>12 disambiguation)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed, ok := tryParseDate(tt.input)
			if ok != tt.ok {
				t.Errorf("tryParseDate(%q) ok = %v, want %v", tt.input, ok, tt.ok)
				return
			}
			if ok {
				if parsed.Month() != tt.month {
					t.Errorf("month = %v, want %v", parsed.Month(), tt.month)
				}
				if parsed.Day() != tt.day {
					t.Errorf("day = %d, want %d", parsed.Day(), tt.day)
				}
			}
		})
	}
}

func TestDateMatchesPeriod(t *testing.T) {
	jan15 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	mar15 := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		date   time.Time
		period string
		want   bool
	}{
		{"monthly match", jan15, "2024-01", true},
		{"monthly mismatch", mar15, "2024-01", false},
		{"annual match", jan15, "2024", true},
		{"quarterly match Q1", jan15, "2024-Q1", true},
		{"quarterly mismatch", mar15, "2024-Q2", false},
		{"quarterly match Q1 march", mar15, "2024-Q1", true},
		{"empty period", jan15, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dateMatchesPeriod(tt.date, tt.period); got != tt.want {
				t.Errorf("dateMatchesPeriod(%v, %q) = %v, want %v", tt.date, tt.period, got, tt.want)
			}
		})
	}
}

func TestTINRegex(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"123-456-789", true},
		{"123-456-789-000", true},
		{"000-000-000-000", true},
		{"12-345-678", false},
		{"1234-567-890", false},
		{"abc-def-ghi", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := tinRegex.MatchString(tt.input); got != tt.valid {
				t.Errorf("TIN %q: got %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}
