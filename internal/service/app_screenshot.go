package service

import (
	"regexp"
	"strings"

	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// isAppScreenshot detects whether OCR text is from a ride-hailing app screenshot.
// Returns "uber", "grab", or "" (not a ride-hailing screenshot).
// hint is an optional user-provided caption/instruction (e.g., "uber", "打车").
func isAppScreenshot(text string, hint string) string {
	lower := strings.ToLower(text)
	hintLower := strings.ToLower(hint)

	// User explicitly said "uber" or "grab" in caption — trust them.
	if strings.Contains(hintLower, "uber") || strings.Contains(hintLower, "打车") {
		return "uber"
	}
	if strings.Contains(hintLower, "grab") {
		return "grab"
	}

	// Uber keywords: English app text + Chinese localized UI.
	uberKeywords := []string{
		"uber", "uberx", "uber comfort", "uber black", "trip fare",
		"uber pool", "uber xl", "uber moto",
		// Chinese Uber app UI elements.
		"重新预约", // "Rebook" button (very Uber-specific)
		"行程费用",  // "Trip fare"
	}

	// Grab keywords.
	grabKeywords := []string{
		"grabcar", "justgrab", "grabpay", "grab car", "grab ride",
		"grabbike", "grabfood",
	}

	uberScore := 0
	for _, kw := range uberKeywords {
		if strings.Contains(lower, kw) {
			uberScore++
		}
	}
	grabScore := 0
	for _, kw := range grabKeywords {
		if strings.Contains(lower, kw) {
			grabScore++
		}
	}

	// Bare name check (+1 each).
	if strings.Contains(lower, "uber") {
		uberScore++
	}
	if strings.Contains(lower, "grab") {
		grabScore++
	}

	// Chinese activity page heuristic: "活动" (Activity tab) + "重新预约" is strong Uber signal.
	// Also match pattern: "活动" header + multiple LKR/₱ amounts = trip list.
	if strings.Contains(lower, "活动") {
		// "活动" alone is generic, but combined with ride-hailing UI elements it's strong.
		if strings.Contains(lower, "重新预约") || strings.Contains(lower, "主页") {
			uberScore += 2
		}
	}

	// Need at least 2 keyword hits to be confident.
	if uberScore >= 2 && uberScore >= grabScore {
		return "uber"
	}
	if grabScore >= 2 {
		return "grab"
	}

	return ""
}

// reDateTrip matches date patterns common in ride-hailing trip history.
// Supports: ISO, slash, text month, and Chinese date formats like "2月5日".
var reDateTrip = regexp.MustCompile(`(?i)(?:` +
	`\d{4}-\d{2}-\d{2}` + // ISO: 2025-03-12
	`|\d{1,2}/\d{1,2}/\d{2,4}` + // Slash: 3/12/2025
	`|\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\w*(?:\s+\d{2,4})?` + // 12 Mar 2025
	`|(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\w*\s+\d{1,2}(?:,?\s+\d{2,4})?` + // Mar 12, 2025
	`|\d{1,2}月\d{1,2}日` + // Chinese: 2月5日
	`)`)

// reAmountWithCurrency matches currency-prefixed amounts (LKR660.35, ₱234.00, etc.).
var reAmountWithCurrency = regexp.MustCompile(`(?i)(?:LKR|₱|PHP|P|S\$|SGD|Rs\.?|රු)\s*[\d,]+(?:\.\d{2})?`)

// parseAppTrips parses multiple trip entries from a ride-hailing app screenshot.
// It scans OCR lines for currency-amount anchors and extracts date + route for each.
func parseAppTrips(lines []string, appType string, jCfg jurisdiction.Config) []ReceiptResult {
	// Build amount regex from jurisdiction config.
	var amtRe *regexp.Regexp
	if len(jCfg.AmountPatterns) > 0 {
		amtRe = regexp.MustCompile(jCfg.AmountPatterns[0])
	}

	vendor := capitalize(appType) // "Uber" or "Grab"

	type tripAnchor struct {
		lineIdx int
		amount  float64
	}

	// Phase 1: find all amount-bearing lines (must have currency prefix to avoid noise).
	var anchors []tripAnchor
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Require currency-prefixed amount to filter out random numbers (times, battery %).
		if !reAmountWithCurrency.MatchString(trimmed) {
			continue
		}
		amt := extractAmountFromLine(trimmed, amtRe)
		if amt <= 0 {
			amt = extractAmountFromLine(trimmed, nil)
		}
		if amt > 0 {
			anchors = append(anchors, tripAnchor{lineIdx: i, amount: amt})
		}
	}

	if len(anchors) == 0 {
		return nil
	}

	// Phase 2: for each anchor, look backward for date and route description.
	var results []ReceiptResult
	for idx, anchor := range anchors {
		startLine := 0
		if idx > 0 {
			startLine = anchors[idx-1].lineIdx + 1
		}

		date := ""
		route := ""
		for j := anchor.lineIdx; j >= startLine; j-- {
			line := strings.TrimSpace(lines[j])
			if line == "" {
				continue
			}
			// Skip UI noise (navigation labels, buttons).
			if isUINoiseTrip(line) {
				continue
			}
			// Try to extract a date (including Chinese format "2月5日").
			if date == "" {
				if m := reDateTrip.FindString(line); m != "" {
					date = m
					continue
				}
			}
			// Non-amount, non-date lines above the anchor = route/destination.
			if route == "" && j != anchor.lineIdx {
				if len(line) > 3 && !reAmountWithCurrency.MatchString(line) {
					route = line
				}
			}
		}

		parsed := ParsedReceipt{
			VendorName:  ParsedField{Value: vendor, Confidence: 0.95},
			TotalAmount: ParsedField{Value: anchor.amount, Confidence: 0.90},
			Category:    ParsedField{Value: "services", Confidence: 0.85},
			VATType:     ParsedField{Value: defaultVATType(jCfg), Confidence: 0.70},
		}
		if date != "" {
			parsed.Date = ParsedField{Value: date, Confidence: 0.85}
		}
		if route != "" {
			parsed.VendorName = ParsedField{
				Value:      vendor + " — " + route,
				Confidence: 0.90,
			}
		}

		results = append(results, ReceiptResult{
			Parsed:            parsed,
			OverallConfidence: AverageConfidence(parsed),
		})
	}

	return results
}

// isUINoiseTrip returns true for ride-hailing app UI chrome that should be ignored.
var uiNoisePatterns = []string{
	"重新预约", "rebook", "主页", "服务", "活动", "账号",
	"home", "services", "activity", "account",
}

func isUINoiseTrip(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	for _, p := range uiNoisePatterns {
		if lower == p || lower == strings.ToLower(p) {
			return true
		}
	}
	return false
}

// defaultVATType returns the first VAT type from jurisdiction config.
func defaultVATType(jCfg jurisdiction.Config) string {
	if len(jCfg.VATTypes) > 0 {
		return jCfg.VATTypes[0]
	}
	return "vatable"
}

// capitalize capitalizes the first letter of a string.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
