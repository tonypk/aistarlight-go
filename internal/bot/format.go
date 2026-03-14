package bot

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func formatReceiptReply(result service.ReceiptResult, txnCount int, classResults []service.ClassificationResult, journalEntries []*domain.JournalEntry, currencySymbol string, refNumbers []int32) string {
	parsed := result.Parsed

	// Amount
	amount := decimal.Zero
	if parsed.TotalAmount.Value != nil {
		switch v := parsed.TotalAmount.Value.(type) {
		case float64:
			amount = decimal.NewFromFloat(v)
		case string:
			amount, _ = decimal.NewFromString(v)
		}
	}

	// Category: prefer classification result (user-selected) over OCR-parsed.
	category := "Goods"
	if len(classResults) > 0 && classResults[0].Category != "" {
		category = capitalize(classResults[0].Category)
	} else if parsed.Category.Value != nil {
		if v, ok := parsed.Category.Value.(string); ok && v != "" {
			category = capitalize(v)
		}
	}

	// Build ref number label.
	refLabel := ""
	if len(refNumbers) == 1 {
		refLabel = fmt.Sprintf(" #TXN-%d", refNumbers[0])
	} else if len(refNumbers) > 1 {
		refs := make([]string, len(refNumbers))
		for i, r := range refNumbers {
			refs[i] = fmt.Sprintf("#TXN-%d", r)
		}
		refLabel = fmt.Sprintf(" (%s)", strings.Join(refs, ", "))
	}

	var lines []string
	line1Parts := []string{
		fmt.Sprintf("Receipt Recorded%s\n%s%s %s", refLabel, currencySymbol, addCommas(amount.StringFixed(2)), category),
	}

	// Date
	if parsed.Date.Value != nil {
		if v, ok := parsed.Date.Value.(string); ok && v != "" {
			line1Parts = append(line1Parts, v)
		}
	}

	// Vendor
	if parsed.VendorName.Value != nil {
		if v, ok := parsed.VendorName.Value.(string); ok && v != "" {
			line1Parts = append(line1Parts, v)
		}
	}

	lines = append(lines, strings.Join(line1Parts, " | "))

	// VAT
	if parsed.VATAmount.Value != nil {
		vatAmount := decimal.Zero
		switch v := parsed.VATAmount.Value.(type) {
		case float64:
			vatAmount = decimal.NewFromFloat(v)
		case string:
			vatAmount, _ = decimal.NewFromString(v)
		}
		if !vatAmount.IsZero() {
			lines = append(lines, fmt.Sprintf("VAT: %s%s", currencySymbol, addCommas(vatAmount.StringFixed(2))))
		}
	}

	// Classification results
	if len(classResults) > 0 {
		cr := classResults[0]
		confPct := int(cr.Confidence * 100)
		lines = append(lines, fmt.Sprintf("\nClassification: %s / %s (%d%%)", cr.VATType, cr.Category, confPct))
	}

	// Journal entries
	if len(journalEntries) > 0 {
		je := journalEntries[0]
		if len(je.Lines) >= 2 {
			dr := je.Lines[0]
			cr := je.Lines[1]
			lines = append(lines, fmt.Sprintf("Journal: DR %s %s%s / CR %s %s%s",
				dr.AccountName, currencySymbol, addCommas(dr.Debit.StringFixed(2)),
				cr.AccountName, currencySymbol, addCommas(cr.Credit.StringFixed(2)),
			))
		} else if len(je.Lines) == 1 {
			l := je.Lines[0]
			if !l.Debit.IsZero() {
				lines = append(lines, fmt.Sprintf("Journal: DR %s %s%s", l.AccountName, currencySymbol, addCommas(l.Debit.StringFixed(2))))
			} else {
				lines = append(lines, fmt.Sprintf("Journal: CR %s %s%s", l.AccountName, currencySymbol, addCommas(l.Credit.StringFixed(2))))
			}
		}
	}

	if txnCount > 1 && len(refNumbers) > 1 {
		refs := make([]string, len(refNumbers))
		for i, r := range refNumbers {
			refs[i] = fmt.Sprintf("#TXN-%d", r)
		}
		lines = append(lines, fmt.Sprintf("\n(%d transactions: %s)", txnCount, strings.Join(refs, ", ")))
	} else if txnCount > 1 {
		lines = append(lines, fmt.Sprintf("\n(%d transactions recorded)", txnCount))
	}

	return strings.Join(lines, "\n")
}

func formatReceiptPreview(result service.ReceiptResult, currencySymbol, uploaderName, projectTag string) string {
	parsed := result.Parsed

	var lines []string
	lines = append(lines, "Receipt Preview\n")

	// Amount
	if parsed.TotalAmount.Value != nil {
		amount := decimal.Zero
		switch v := parsed.TotalAmount.Value.(type) {
		case float64:
			amount = decimal.NewFromFloat(v)
		case string:
			amount, _ = decimal.NewFromString(v)
		}
		lines = append(lines, fmt.Sprintf("Amount: %s%s", currencySymbol, addCommas(amount.StringFixed(2))))
	}

	// Vendor
	if parsed.VendorName.Value != nil {
		if v, ok := parsed.VendorName.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("Vendor: %s", v))
		}
	}

	// Date
	if parsed.Date.Value != nil {
		if v, ok := parsed.Date.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("Date: %s", v))
		}
	}

	// Description (from line items or caption)
	desc := formatLineItemsDescription(parsed)
	if desc != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", desc))
	}

	// Receipt number
	if parsed.ReceiptNumber.Value != nil {
		if v, ok := parsed.ReceiptNumber.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("Receipt #: %s", v))
		}
	}

	// TIN
	if parsed.TIN.Value != nil {
		if v, ok := parsed.TIN.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("TIN: %s", v))
		}
	}

	// VAT
	if parsed.VATAmount.Value != nil {
		vatAmount := decimal.Zero
		switch v := parsed.VATAmount.Value.(type) {
		case float64:
			vatAmount = decimal.NewFromFloat(v)
		case string:
			vatAmount, _ = decimal.NewFromString(v)
		}
		if !vatAmount.IsZero() {
			lines = append(lines, fmt.Sprintf("VAT: %s%s", currencySymbol, addCommas(vatAmount.StringFixed(2))))
		}
	}

	if projectTag != "" {
		lines = append(lines, fmt.Sprintf("Project: %s", projectTag))
	}

	// Classification preview
	lines = append(lines, "\nRecommended Classification:")
	if parsed.VATType.Value != nil {
		vatType, _ := parsed.VATType.Value.(string)
		confPct := int(parsed.VATType.Confidence * 100)
		lines = append(lines, fmt.Sprintf("  VAT Type: %s (%d%%)", vatType, confPct))
	}
	if parsed.Category.Value != nil {
		category, _ := parsed.Category.Value.(string)
		confPct := int(parsed.Category.Confidence * 100)
		lines = append(lines, fmt.Sprintf("  Category: %s (%d%%)", category, confPct))
	}

	lines = append(lines, fmt.Sprintf("\nUploaded by: %s", uploaderName))

	return strings.Join(lines, "\n")
}

// formatLineItemsDescription builds a short description from line items.
// Returns empty string if no line items.
func formatLineItemsDescription(parsed service.ParsedReceipt) string {
	if len(parsed.LineItems) == 0 {
		return ""
	}
	var parts []string
	for _, item := range parsed.LineItems {
		s := item.Name
		if item.Qty > 1 {
			if item.Qty == float64(int(item.Qty)) {
				s += fmt.Sprintf(" x%d", int(item.Qty))
			} else {
				s += fmt.Sprintf(" x%.1f", item.Qty)
			}
		}
		parts = append(parts, s)
	}
	desc := strings.Join(parts, ", ")
	if len(desc) > 120 {
		desc = desc[:117] + "..."
	}
	return desc
}

// formatAmountSelectionPreview shows a receipt preview with all detected amounts,
// asking the user to select which amount to record.
func formatAmountSelectionPreview(result service.ReceiptResult, currencySymbol, uploaderName string) string {
	parsed := result.Parsed

	var lines []string
	lines = append(lines, "Receipt Preview — Multiple amounts detected\n")

	// Vendor
	if parsed.VendorName.Value != nil {
		if v, ok := parsed.VendorName.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("Vendor: %s", v))
		}
	}

	// Date
	if parsed.Date.Value != nil {
		if v, ok := parsed.Date.Value.(string); ok && v != "" {
			lines = append(lines, fmt.Sprintf("Date: %s", v))
		}
	}

	lines = append(lines, "\nDetected amounts:")
	for _, da := range parsed.DetectedAmounts {
		d := decimal.NewFromFloat(da.Amount)
		marker := ""
		if da.IsLikelyTotal {
			marker = " *"
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s%s", da.Label, currencySymbol, addCommas(d.StringFixed(2)), marker))
	}

	lines = append(lines, fmt.Sprintf("\nUploaded by: %s", uploaderName))
	lines = append(lines, "\nWhich amount should be recorded?")

	return strings.Join(lines, "\n")
}

// formatMultiTripPreview formats a preview for multiple trips detected from a single screenshot.
func formatMultiTripPreview(results []service.ReceiptResult, currencySymbol, uploaderName string) string {
	if len(results) == 0 {
		return "No trips detected."
	}

	// Detect app name from vendor of first result.
	appName := "Ride"
	if v, ok := results[0].Parsed.VendorName.Value.(string); ok && v != "" {
		if strings.Contains(strings.ToLower(v), "uber") {
			appName = "Uber"
		} else if strings.Contains(strings.ToLower(v), "grab") {
			appName = "Grab"
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Receipt Preview — %d %s trips\n", len(results), appName))

	total := decimal.Zero
	for i, r := range results {
		amount := decimal.Zero
		if r.Parsed.TotalAmount.Value != nil {
			switch v := r.Parsed.TotalAmount.Value.(type) {
			case float64:
				amount = decimal.NewFromFloat(v)
			case string:
				amount, _ = decimal.NewFromString(v)
			}
		}
		total = total.Add(amount)

		dateStr := ""
		if r.Parsed.Date.Value != nil {
			if v, ok := r.Parsed.Date.Value.(string); ok && v != "" {
				dateStr = v
			}
		}

		// Extract route from vendor name (format: "Uber — BGC to Makati").
		route := ""
		if v, ok := r.Parsed.VendorName.Value.(string); ok {
			if idx := strings.Index(v, " — "); idx >= 0 {
				route = v[idx+len(" — "):]
			}
		}

		entry := fmt.Sprintf("%d. %s%s", i+1, currencySymbol, addCommas(amount.StringFixed(2)))
		if dateStr != "" {
			entry += " | " + dateStr
		}
		if route != "" {
			entry += " | " + route
		}
		lines = append(lines, entry)
	}

	lines = append(lines, fmt.Sprintf("\nTotal: %s%s", currencySymbol, addCommas(total.StringFixed(2))))
	lines = append(lines, fmt.Sprintf("Uploaded by: %s", uploaderName))

	return strings.Join(lines, "\n")
}

// formatMultiTripReply formats the confirmation reply for multiple trips.
func formatMultiTripReply(results []service.ReceiptResult, currencySymbol string, refNumbers []int32) string {
	if len(results) == 0 {
		return "No trips recorded."
	}

	appName := "Ride"
	if v, ok := results[0].Parsed.VendorName.Value.(string); ok && v != "" {
		if strings.Contains(strings.ToLower(v), "uber") {
			appName = "Uber"
		} else if strings.Contains(strings.ToLower(v), "grab") {
			appName = "Grab"
		}
	}

	var lines []string
	total := decimal.Zero

	refs := make([]string, 0, len(refNumbers))
	for _, r := range refNumbers {
		refs = append(refs, fmt.Sprintf("#TXN-%d", r))
	}
	refLabel := ""
	if len(refs) > 0 {
		refLabel = " (" + strings.Join(refs, ", ") + ")"
	}

	lines = append(lines, fmt.Sprintf("%d %s Trips Recorded%s\n", len(results), appName, refLabel))

	for i, r := range results {
		amount := decimal.Zero
		if r.Parsed.TotalAmount.Value != nil {
			switch v := r.Parsed.TotalAmount.Value.(type) {
			case float64:
				amount = decimal.NewFromFloat(v)
			case string:
				amount, _ = decimal.NewFromString(v)
			}
		}
		total = total.Add(amount)

		ref := ""
		if i < len(refNumbers) {
			ref = fmt.Sprintf(" #TXN-%d", refNumbers[i])
		}

		entry := fmt.Sprintf("%d. %s%s%s", i+1, currencySymbol, addCommas(amount.StringFixed(2)), ref)
		lines = append(lines, entry)
	}

	lines = append(lines, fmt.Sprintf("\nTotal: %s%s", currencySymbol, addCommas(total.StringFixed(2))))

	return strings.Join(lines, "\n")
}

// addCommas adds thousands separators to a decimal string like "5200.00" -> "5,200.00".
func addCommas(s string) string {
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]

	n := len(intPart)
	if n <= 3 {
		if len(parts) == 2 {
			return intPart + "." + parts[1]
		}
		return intPart
	}

	var result strings.Builder
	remainder := n % 3
	if remainder > 0 {
		result.WriteString(intPart[:remainder])
	}
	for i := remainder; i < n; i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(intPart[i : i+3])
	}

	if len(parts) == 2 {
		result.WriteByte('.')
		result.WriteString(parts[1])
	}
	return result.String()
}

// formatInterface converts an interface{} (from sqlc COALESCE) to a formatted amount string.
func formatInterface(v interface{}) string {
	if v == nil {
		return "0.00"
	}
	switch val := v.(type) {
	case float64:
		return addCommas(decimal.NewFromFloat(val).StringFixed(2))
	case int64:
		return addCommas(decimal.NewFromInt(val).StringFixed(2))
	case string:
		if d, err := decimal.NewFromString(val); err == nil {
			return addCommas(d.StringFixed(2))
		}
	}
	slog.Warn("formatInterface: unexpected type", "type", fmt.Sprintf("%T", v), "value", v)
	return fmt.Sprintf("%v", v)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
