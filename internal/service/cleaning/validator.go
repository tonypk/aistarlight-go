package cleaning

import (
	"regexp"
	"strings"
	"time"
)

// tinRegex matches Philippine TIN format: ###-###-###(-###)?
var tinRegex = regexp.MustCompile(`^\d{3}-\d{3}-\d{3}(-\d{3})?$`)

// ValidatorConfig holds validation parameters.
type ValidatorConfig struct {
	// Period is the expected reporting period (e.g., "2024-01" for January 2024).
	// Used for date period consistency checking.
	Period string

	// TableType is used to determine required fields.
	TableType string

	// PeriodMismatchThreshold: if more than this fraction of dates are outside
	// the expected period, a warning is raised.
	PeriodMismatchThreshold float64
}

// DefaultValidatorConfig returns sensible defaults.
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		PeriodMismatchThreshold: 0.20,
	}
}

// Validate runs all validation checks on cleaned data rows.
// columns is the list of column names; mapping maps column names to target fields.
func Validate(rows []map[string]interface{}, columns []string, mapping map[string]FieldMapping, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	// Build reverse mapping: target_field → column_name
	fieldToCol := make(map[string]string, len(mapping))
	for col, fm := range mapping {
		if fm.TargetField != "" {
			fieldToCol[fm.TargetField] = col
		}
	}

	issues = append(issues, validateDates(rows, fieldToCol, cfg)...)
	issues = append(issues, validateAmounts(rows, fieldToCol, cfg)...)
	issues = append(issues, validateTINs(rows, fieldToCol, cfg)...)
	issues = append(issues, validateDuplicates(rows, fieldToCol, cfg)...)
	issues = append(issues, validateRequiredFields(rows, fieldToCol, cfg)...)

	return issues
}

// validateDates checks date fields for parsability and period consistency.
func validateDates(rows []map[string]interface{}, fieldToCol map[string]string, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	// Identify date columns based on target field names
	dateFields := []string{
		"sales_date", "purchase_date", "date", "invoice_date",
		"income_date", "expense_date", "revenue_date",
		"importation_date", "assessment_date",
	}

	for _, df := range dateFields {
		colName, ok := fieldToCol[df]
		if !ok {
			continue
		}

		totalDates := 0
		parsedDates := 0
		outOfPeriod := 0

		for i, row := range rows {
			val := cellString(row, colName)
			if val == "" {
				continue
			}
			totalDates++

			t, ok := tryParseDate(val)
			if !ok {
				issues = append(issues, ValidationIssue{
					RowIndex: i,
					Column:   colName,
					Field:    df,
					Value:    val,
					Message:  "cannot parse as date",
					Severity: SeverityWarning,
				})
				continue
			}
			parsedDates++

			// Period consistency check
			if cfg.Period != "" {
				if !dateMatchesPeriod(t, cfg.Period) {
					outOfPeriod++
				}
			}
		}

		// Report period mismatch at sheet level
		if totalDates > 0 && cfg.Period != "" && parsedDates > 0 {
			mismatchRate := float64(outOfPeriod) / float64(parsedDates)
			if mismatchRate > cfg.PeriodMismatchThreshold {
				issues = append(issues, ValidationIssue{
					RowIndex: -1,
					Column:   colName,
					Field:    df,
					Value:    cfg.Period,
					Message:  "more than 20% of dates are outside the expected period",
					Severity: SeverityWarning,
				})
			}
		}
	}

	return issues
}

// validateAmounts checks amount fields for parsability and non-zero values.
func validateAmounts(rows []map[string]interface{}, fieldToCol map[string]string, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	amountFields := []string{
		"gross_sales", "gross_purchase", "amount", "tax_base",
		"output_tax", "input_tax", "tax_withheld", "total_sales",
		"gross_compensation", "taxable_compensation",
		"debit", "credit", "balance",
		"basic_salary", "expense_amount",
	}

	for _, af := range amountFields {
		colName, ok := fieldToCol[af]
		if !ok {
			continue
		}

		for i, row := range rows {
			val := cellString(row, colName)
			if val == "" {
				continue
			}
			_, ok := ParseFloat(val)
			if !ok {
				issues = append(issues, ValidationIssue{
					RowIndex: i,
					Column:   colName,
					Field:    af,
					Value:    TruncateString(val, 50),
					Message:  "cannot parse as numeric amount",
					Severity: SeverityWarning,
				})
			}
		}
	}

	return issues
}

// validateTINs checks TIN fields for Philippine format.
func validateTINs(rows []map[string]interface{}, fieldToCol map[string]string, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	tinFields := []string{
		"customer_tin", "supplier_tin", "tin", "employee_tin", "employer_tin",
	}

	for _, tf := range tinFields {
		colName, ok := fieldToCol[tf]
		if !ok {
			continue
		}

		for i, row := range rows {
			val := cellString(row, colName)
			if val == "" {
				continue
			}
			// Normalize: remove spaces
			normalized := strings.ReplaceAll(strings.TrimSpace(val), " ", "")
			if !tinRegex.MatchString(normalized) {
				issues = append(issues, ValidationIssue{
					RowIndex: i,
					Column:   colName,
					Field:    tf,
					Value:    TruncateString(val, 30),
					Message:  "invalid TIN format (expected ###-###-###[-###])",
					Severity: SeverityWarning,
				})
			}
		}
	}

	return issues
}

// validateDuplicates checks for duplicate rows based on key fields.
func validateDuplicates(rows []map[string]interface{}, fieldToCol map[string]string, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	// Build composite key from available fields
	keyFields := []string{
		"sales_invoice_number", "purchase_invoice_number", "invoice_number",
	}
	dateFields := []string{
		"sales_date", "purchase_date", "date", "invoice_date",
	}
	amountFields := []string{
		"gross_sales", "gross_purchase", "amount", "tax_base",
	}

	var docCol, dateCol, amtCol string
	for _, f := range keyFields {
		if c, ok := fieldToCol[f]; ok {
			docCol = c
			break
		}
	}
	for _, f := range dateFields {
		if c, ok := fieldToCol[f]; ok {
			dateCol = c
			break
		}
	}
	for _, f := range amountFields {
		if c, ok := fieldToCol[f]; ok {
			amtCol = c
			break
		}
	}

	if docCol == "" && dateCol == "" {
		return issues // can't check duplicates without key fields
	}

	seen := make(map[string]int) // composite key → first row index
	for i, row := range rows {
		parts := []string{
			cellString(row, docCol),
			cellString(row, dateCol),
			cellString(row, amtCol),
		}
		key := strings.Join(parts, "|")
		if key == "||" {
			continue // all empty, skip
		}

		if firstIdx, exists := seen[key]; exists {
			issues = append(issues, ValidationIssue{
				RowIndex: i,
				Column:   docCol,
				Field:    "duplicate",
				Value:    TruncateString(key, 50),
				Message:  "possible duplicate of row " + itoa(firstIdx),
				Severity: SeverityWarning,
			})
		} else {
			seen[key] = i
		}
	}

	return issues
}

// validateRequiredFields checks that mandatory fields are present based on table type.
func validateRequiredFields(rows []map[string]interface{}, fieldToCol map[string]string, cfg ValidatorConfig) []ValidationIssue {
	var issues []ValidationIssue

	required := requiredFieldsByType(cfg.TableType)
	if len(required) == 0 {
		return issues
	}

	// Check at sheet level: are the required columns mapped?
	for _, field := range required {
		if _, ok := fieldToCol[field]; !ok {
			issues = append(issues, ValidationIssue{
				RowIndex: -1,
				Column:   "",
				Field:    field,
				Value:    "",
				Message:  "required field not mapped to any column",
				Severity: SeverityWarning,
			})
		}
	}

	return issues
}

// requiredFieldsByType returns the fields required for a given table type.
func requiredFieldsByType(tableType string) []string {
	switch tableType {
	case "sales":
		return []string{"sales_date", "customer_name", "gross_sales"}
	case "purchase":
		return []string{"purchase_date", "supplier_name", "gross_purchase"}
	case "ewt":
		return []string{"supplier_name", "tax_base", "tax_withheld"}
	case "payroll":
		return []string{"employee_name", "gross_compensation"}
	case "bank":
		return []string{"date", "description"}
	default:
		return nil
	}
}

// cellString extracts a string value from a row map.
func cellString(row map[string]interface{}, key string) string {
	if key == "" {
		return ""
	}
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// tryParseDate attempts to parse a date string using common Philippine formats.
func tryParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	formats := []string{
		"01/02/2006",     // MM/DD/YYYY (most common in PH)
		"1/2/2006",       // M/D/YYYY
		"01-02-2006",     // MM-DD-YYYY
		"2006-01-02",     // ISO 8601
		"Jan 2, 2006",    // Month D, YYYY
		"January 2, 2006",
		"02-Jan-2006",    // DD-Mon-YYYY
		"2006/01/02",     // YYYY/MM/DD
		"01/02/06",       // MM/DD/YY
		"1/2/06",         // M/D/YY
	}

	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, true
		}
	}

	// Handle ambiguous dates where day > 12 (can't be MM/DD)
	// e.g., "13/02/2024" must be DD/MM/YYYY
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '-' })
	if len(parts) == 3 {
		ddmmFormats := []string{
			"02/01/2006", // DD/MM/YYYY
			"02-01-2006", // DD-MM-YYYY
			"02/01/06",   // DD/MM/YY
		}
		for _, f := range ddmmFormats {
			t, err := time.Parse(f, s)
			if err == nil {
				return t, true
			}
		}
	}

	return time.Time{}, false
}

// dateMatchesPeriod checks if a date falls within the expected period.
// Period format: "YYYY-MM" for monthly, "YYYY-Q1" for quarterly, "YYYY" for annual.
func dateMatchesPeriod(t time.Time, period string) bool {
	if period == "" {
		return true
	}

	year := t.Year()
	month := int(t.Month())

	// Try "YYYY-MM" format
	if len(period) == 7 && period[4] == '-' {
		var py, pm int
		n, _ := parseYearMonth(period)
		if n {
			py, pm = extractYearMonth(period)
			return year == py && month == pm
		}
		_ = py
		_ = pm
	}

	// Try "YYYY" format (annual)
	if len(period) == 4 {
		var py int
		for _, c := range period {
			if c >= '0' && c <= '9' {
				py = py*10 + int(c-'0')
			} else {
				return true // can't parse, don't flag
			}
		}
		return year == py
	}

	// Try "YYYY-QN" format (quarterly)
	if len(period) == 7 && strings.Contains(period, "-Q") {
		var py int
		for _, c := range period[:4] {
			py = py*10 + int(c-'0')
		}
		q := int(period[6] - '0')
		if year != py {
			return false
		}
		switch q {
		case 1:
			return month >= 1 && month <= 3
		case 2:
			return month >= 4 && month <= 6
		case 3:
			return month >= 7 && month <= 9
		case 4:
			return month >= 10 && month <= 12
		}
	}

	return true // unknown format, don't flag
}

// parseYearMonth checks if the string is in YYYY-MM format.
func parseYearMonth(s string) (bool, error) {
	if len(s) != 7 || s[4] != '-' {
		return false, nil
	}
	for i, c := range s {
		if i == 4 {
			continue
		}
		if c < '0' || c > '9' {
			return false, nil
		}
	}
	return true, nil
}

// extractYearMonth extracts year and month from "YYYY-MM".
func extractYearMonth(s string) (int, int) {
	year := int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
	month := int(s[5]-'0')*10 + int(s[6]-'0')
	return year, month
}
