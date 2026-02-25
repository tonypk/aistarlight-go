package cleaning

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

// IsEmptyRow returns true if every cell is blank.
func IsEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// IsSequentialNumbers returns true if the non-empty cells form a sequential
// integer series like 1,2,3 or (1),(2),(3). BIR forms often have such rows.
func IsSequentialNumbers(row []string) bool {
	var nums []int
	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			continue
		}
		v = strings.TrimPrefix(v, "(")
		v = strings.TrimSuffix(v, ")")
		v = strings.TrimSpace(v)
		n := 0
		isInt := true
		for _, c := range v {
			if c < '0' || c > '9' {
				isInt = false
				break
			}
			n = n*10 + int(c-'0')
		}
		if !isInt || n < 0 {
			return false
		}
		nums = append(nums, n)
	}
	if len(nums) < 3 {
		return false
	}
	for i := 1; i < len(nums); i++ {
		if nums[i] != nums[i-1]+1 {
			return false
		}
	}
	return true
}

// LooksNumeric returns true if the string looks like a number, date, or
// TIN-like pattern (more digits/dots than text).
func LooksNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	cleaned := strings.NewReplacer(",", "", "$", "", "₱", "", "PHP", "", "%", "", " ", "").Replace(s)
	digitDot := 0
	for _, c := range cleaned {
		if (c >= '0' && c <= '9') || c == '.' || c == '-' {
			digitDot++
		}
	}
	if len(cleaned) > 0 && float64(digitDot)/float64(len(cleaned)) > 0.7 {
		return true
	}
	return false
}

// FillRatio returns the fraction of non-empty cells in a row.
func FillRatio(row []string, totalCols int) float64 {
	if totalCols <= 0 {
		return 0
	}
	nonEmpty := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			nonEmpty++
		}
	}
	return float64(nonEmpty) / float64(totalCols)
}

// NonEmptyCount returns the number of non-empty cells in a row.
func NonEmptyCount(row []string) int {
	n := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			n++
		}
	}
	return n
}

// TextRatio returns the fraction of non-empty cells that are NOT numeric.
func TextRatio(row []string) float64 {
	nonEmpty := 0
	numericCount := 0
	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			continue
		}
		nonEmpty++
		if LooksNumeric(v) {
			numericCount++
		}
	}
	if nonEmpty == 0 {
		return 0
	}
	return 1.0 - float64(numericCount)/float64(nonEmpty)
}

// JaccardSimilarity computes the Jaccard index between two string sets.
// Both inputs are lowercased before comparison.
func JaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		v := strings.ToLower(strings.TrimSpace(s))
		if v != "" {
			setA[v] = struct{}{}
		}
	}

	setB := make(map[string]struct{}, len(b))
	for _, s := range b {
		v := strings.ToLower(strings.TrimSpace(s))
		if v != "" {
			setB[v] = struct{}{}
		}
	}

	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// HeaderSignature extracts a normalized, sorted list of non-empty, lowercased
// column names for template matching.
func HeaderSignature(columns []string) []string {
	var sig []string
	for _, c := range columns {
		v := strings.ToLower(strings.TrimSpace(c))
		if v != "" && !strings.HasPrefix(v, "column_") {
			sig = append(sig, v)
		}
	}
	sort.Strings(sig)
	return sig
}

// HeaderSignatureHash returns a SHA-256 hex digest of the header signature.
func HeaderSignatureHash(columns []string) string {
	sig := HeaderSignature(columns)
	h := sha256.Sum256([]byte(strings.Join(sig, "|")))
	return hex.EncodeToString(h[:])
}

// FormatRowForAI formats a row as a JSON-like array string for AI prompts.
func FormatRowForAI(row []string) string {
	parts := make([]string, len(row))
	for i, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			parts[i] = `""`
		} else {
			parts[i] = fmt.Sprintf("%q", v)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// MaxColWidth returns the maximum number of columns across all rows.
func MaxColWidth(rows CellGrid) int {
	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}
	return maxCols
}

// ClampInt clamps v to [lo, hi].
func ClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// TruncateString truncates s to maxLen characters, appending "..." if needed.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// RowPreview returns a compact string preview of a row (up to 100 chars).
func RowPreview(row []string) string {
	var parts []string
	for _, cell := range row {
		v := strings.TrimSpace(cell)
		if v != "" {
			parts = append(parts, v)
		}
	}
	return TruncateString(strings.Join(parts, " | "), 100)
}

// ParseFloat attempts to parse a string as a float64, stripping common
// formatting characters. Returns 0 and false if parsing fails.
func ParseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	// Handle parenthetical negatives: (1,234.56) → -1234.56
	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
		negative = true
	}

	// Strip currency and formatting
	s = strings.NewReplacer(
		",", "", "$", "", "₱", "", "PHP", "", "php", "",
		" ", "", "\u00a0", "", // non-breaking space
	).Replace(s)

	if s == "" || s == "-" || s == "." {
		return 0, false
	}

	// Handle explicit negative
	if strings.HasPrefix(s, "-") {
		negative = !negative
		s = s[1:]
	}

	// Parse digits and decimal point
	var intPart, fracPart int64
	var fracDigits int
	inFrac := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			if inFrac {
				fracPart = fracPart*10 + int64(c-'0')
				fracDigits++
			} else {
				intPart = intPart*10 + int64(c-'0')
			}
		} else if c == '.' && !inFrac {
			inFrac = true
		} else {
			return 0, false
		}
	}

	result := float64(intPart)
	if fracDigits > 0 {
		result += float64(fracPart) / math.Pow(10, float64(fracDigits))
	}
	if negative {
		result = -result
	}
	return result, true
}
