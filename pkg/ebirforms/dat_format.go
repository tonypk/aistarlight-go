package ebirforms

import (
	"fmt"
	"strings"
)

// DATField defines a single field in a BIR DAT record.
type DATField struct {
	Name   string
	Width  int
	Align  string // "left" or "right"
	Pad    byte   // padding character
}

// DATRecord is an ordered list of fields making up a DAT line.
type DATRecord []DATField

// FormatField formats a single value according to field spec.
func FormatField(value string, field DATField) string {
	if len(value) > field.Width {
		value = value[:field.Width]
	}

	padChar := field.Pad
	if padChar == 0 {
		padChar = ' '
	}

	padding := strings.Repeat(string(padChar), field.Width-len(value))

	if field.Align == "right" {
		return padding + value
	}
	return value + padding
}

// FormatRecord formats a complete DAT record from field values.
func FormatRecord(record DATRecord, values map[string]string) string {
	var parts []string
	for _, field := range record {
		val := values[field.Name]
		parts = append(parts, FormatField(val, field))
	}
	return strings.Join(parts, "")
}

// FormatAmount formats a decimal string for DAT output (no decimal point, right-aligned).
func FormatAmount(amountStr string, width int) string {
	// Remove decimal point: "1234.56" → "123456"
	amount := strings.ReplaceAll(amountStr, ".", "")
	amount = strings.ReplaceAll(amount, ",", "")
	amount = strings.ReplaceAll(amount, "-", "")
	if amount == "" || amount == "0" {
		amount = "0"
	}
	return fmt.Sprintf("%0*s", width, amount)
}

// FormatTIN normalizes a TIN for DAT output (digits only, padded to 12).
func FormatTIN(tin string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, tin)
	if len(digits) > 12 {
		digits = digits[:12]
	}
	return fmt.Sprintf("%-12s", digits)
}

// --- BIR 2550M DAT format ---

// Header2550M defines the header record for BIR 2550M DAT file.
var Header2550M = DATRecord{
	{Name: "record_type", Width: 1, Align: "left"},   // "H"
	{Name: "form_type", Width: 7, Align: "left"},      // "2550M  "
	{Name: "tin", Width: 12, Align: "left"},
	{Name: "branch_code", Width: 5, Align: "left"},
	{Name: "registered_name", Width: 50, Align: "left"},
	{Name: "return_period", Width: 6, Align: "left"},  // MMYYYY
	{Name: "rdo_code", Width: 3, Align: "left"},
	{Name: "amended_return", Width: 1, Align: "left"}, // "N"
}

// Detail2550M defines the detail record for BIR 2550M DAT file.
var Detail2550M = DATRecord{
	{Name: "record_type", Width: 1, Align: "left"},           // "D"
	{Name: "line_number", Width: 4, Align: "right", Pad: '0'},
	{Name: "amount", Width: 15, Align: "right", Pad: '0'},
}
