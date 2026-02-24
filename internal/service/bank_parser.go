package service

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BankFormatConfig defines how to parse a specific bank's statement format.
type BankFormatConfig struct {
	Name              string
	DateColumns       []string
	DescriptionColumns []string
	DebitColumn       string
	CreditColumn      string
	AmountColumn      string
	ReferenceColumn   string
	DateFormat        string
}

// ParsedBankEntry is a standardized bank statement entry.
type ParsedBankEntry struct {
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"` // debit or credit
	Reference   string  `json:"reference,omitempty"`
	ID          string  `json:"id,omitempty"`
}

var bankFormats = map[string]BankFormatConfig{
	"BDO": {
		Name:              "BDO",
		DateColumns:       []string{"date", "posting date", "transaction date"},
		DescriptionColumns: []string{"description", "particulars", "remarks"},
		DebitColumn:       "debit",
		CreditColumn:      "credit",
		DateFormat:        "01/02/2006",
	},
	"BPI": {
		Name:              "BPI",
		DateColumns:       []string{"date", "transaction date", "posting date"},
		DescriptionColumns: []string{"remarks", "description", "particulars"},
		DebitColumn:       "withdrawal",
		CreditColumn:      "deposit",
		DateFormat:        "01/02/2006",
	},
	"Metrobank": {
		Name:              "Metrobank",
		DateColumns:       []string{"date", "transaction date"},
		DescriptionColumns: []string{"description", "remarks"},
		DebitColumn:       "debit",
		CreditColumn:      "credit",
		DateFormat:        "01/02/2006",
	},
	"PayPal": {
		Name:              "PayPal",
		DateColumns:       []string{"date", "date/time"},
		DescriptionColumns: []string{"name", "subject", "description", "item title"},
		AmountColumn:      "amount",
		ReferenceColumn:   "transaction id",
		DateFormat:        "01/02/2006",
	},
	"Stripe": {
		Name:              "Stripe",
		DateColumns:       []string{"created (utc)", "created", "date"},
		DescriptionColumns: []string{"description", "customer description"},
		AmountColumn:      "amount",
		ReferenceColumn:   "id",
		DateFormat:        "2006-01-02",
	},
	"GCash": {
		Name:              "GCash",
		DateColumns:       []string{"date", "transaction date"},
		DescriptionColumns: []string{"transaction type", "description", "reference"},
		AmountColumn:      "amount",
		DateFormat:        "01/02/2006",
	},
	"Generic": {
		Name:              "Generic",
		DateColumns:       []string{"date", "transaction date", "posting date", "value date"},
		DescriptionColumns: []string{"description", "remarks", "particulars", "memo", "narrative"},
		DebitColumn:       "debit",
		CreditColumn:      "credit",
		AmountColumn:      "amount",
		ReferenceColumn:   "reference",
		DateFormat:        "01/02/2006",
	},
}

// DetectBankFormat scores each format against the given columns and returns the best match.
func DetectBankFormat(columns []string) *BankFormatConfig {
	colSet := make(map[string]bool, len(columns))
	for _, c := range columns {
		colSet[strings.ToLower(strings.TrimSpace(c))] = true
	}

	var bestFormat *BankFormatConfig
	bestScore := 0

	for name, fmt := range bankFormats {
		if name == "Generic" {
			continue
		}
		score := 0

		for _, dc := range fmt.DateColumns {
			if colSet[dc] {
				score += 2
				break
			}
		}
		for _, dc := range fmt.DescriptionColumns {
			if colSet[dc] {
				score += 2
				break
			}
		}
		if fmt.DebitColumn != "" && colSet[fmt.DebitColumn] {
			score++
		}
		if fmt.CreditColumn != "" && colSet[fmt.CreditColumn] {
			score++
		}
		if fmt.AmountColumn != "" && colSet[fmt.AmountColumn] {
			score++
		}

		if score > bestScore {
			bestScore = score
			f := fmt // copy
			bestFormat = &f
		}
	}

	if bestScore >= 3 {
		return bestFormat
	}

	generic := bankFormats["Generic"]
	return &generic
}

// ParseBankStatement standardizes bank statement rows using the given format.
func ParseBankStatement(rows []map[string]interface{}, format *BankFormatConfig) []ParsedBankEntry {
	columns := make([]string, 0)
	if len(rows) > 0 {
		for k := range rows[0] {
			columns = append(columns, k)
		}
	}

	dateCol := findColumn(columns, format.DateColumns)
	descCol := findColumn(columns, format.DescriptionColumns)
	refCol := ""
	if format.ReferenceColumn != "" {
		refCol = findColumn(columns, []string{format.ReferenceColumn})
	}

	var entries []ParsedBankEntry

	for _, row := range rows {
		dateStr := parseDate(toString(row[dateCol]), format.DateFormat)
		desc := toString(row[descCol])

		var amount float64
		entryType := "debit"

		if format.DebitColumn != "" && format.CreditColumn != "" {
			debitCol := findColumn(columns, []string{format.DebitColumn})
			creditCol := findColumn(columns, []string{format.CreditColumn})
			debit := parseAmount(row[debitCol])
			credit := parseAmount(row[creditCol])

			if credit != 0 {
				amount = math.Abs(credit)
				entryType = "credit"
			} else {
				amount = math.Abs(debit)
				entryType = "debit"
			}
		} else if format.AmountColumn != "" {
			amtCol := findColumn(columns, []string{format.AmountColumn})
			amt := parseAmount(row[amtCol])
			if amt < 0 {
				amount = math.Abs(amt)
				entryType = "debit"
			} else {
				amount = amt
				entryType = "credit"
			}
		}

		if amount == 0 && desc == "" {
			continue
		}

		entry := ParsedBankEntry{
			Date:        dateStr,
			Description: desc,
			Amount:      amount,
			Type:        entryType,
		}
		if refCol != "" {
			entry.Reference = toString(row[refCol])
		}

		entries = append(entries, entry)
	}

	return entries
}

// BankParseResult holds the output of auto-detection and parsing.
type BankParseResult struct {
	BankName       string            `json:"bank_name"`
	FormatDetected bool              `json:"format_detected"`
	Transactions   []ParsedBankEntry `json:"transactions"`
	RowCount       int               `json:"row_count"`
	FileType       string            `json:"file_type"`
}

func findColumn(columns []string, candidates []string) string {
	colMap := make(map[string]string, len(columns))
	for _, c := range columns {
		colMap[strings.ToLower(strings.TrimSpace(c))] = c
	}
	for _, candidate := range candidates {
		if original, ok := colMap[candidate]; ok {
			return original
		}
	}
	return ""
}

var dateFormats = []string{
	"01/02/2006",
	"2006-01-02",
	"1/2/2006",
	"02/01/2006",
	"2006-01-02T15:04:05",
	"Jan 2, 2006",
	"2 Jan 2006",
}

var datePattern = regexp.MustCompile(`\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\d{4}[/-]\d{1,2}[/-]\d{1,2}`)

func parseDate(value string, expectedFormat string) string {
	if value == "" {
		return ""
	}
	value = strings.TrimSpace(value)

	// Try expected format first
	formats := append([]string{expectedFormat}, dateFormats...)
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, value); err == nil {
			return t.Format("2006-01-02")
		}
	}

	// Check if already ISO-like
	if len(value) >= 10 && datePattern.MatchString(value[:10]) {
		return value[:10]
	}

	return ""
}

func parseAmount(v interface{}) float64 {
	if v == nil {
		return 0
	}

	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		s := strings.TrimSpace(val)
		if s == "" || strings.EqualFold(s, "nan") || strings.EqualFold(s, "none") {
			return 0
		}
		// Handle parenthetical negatives: "(1234.56)" → -1234.56
		if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
			s = "-" + s[1:len(s)-1]
		}
		// Strip currency symbols and commas
		s = strings.NewReplacer("$", "", "₱", "", "PHP", "", ",", "", " ", "").Replace(s)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}
