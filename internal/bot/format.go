package bot

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func formatReceiptReply(result service.ReceiptResult, txnCount int) string {
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

	// Category
	category := "Goods"
	if parsed.Category.Value != nil {
		if v, ok := parsed.Category.Value.(string); ok && v != "" {
			category = capitalize(v)
		}
	}

	parts := []string{
		fmt.Sprintf("Recorded P%s %s", addCommas(amount.StringFixed(2)), category),
	}

	// Date
	if parsed.Date.Value != nil {
		if v, ok := parsed.Date.Value.(string); ok && v != "" {
			parts = append(parts, v)
		}
	}

	// Vendor
	if parsed.VendorName.Value != nil {
		if v, ok := parsed.VendorName.Value.(string); ok && v != "" {
			parts = append(parts, v)
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
			parts = append(parts, fmt.Sprintf("VAT: P%s", addCommas(vatAmount.StringFixed(2))))
		}
	}

	return strings.Join(parts, " | ")
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
