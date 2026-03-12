package service

import (
	"regexp"
	"strings"

	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// isAppScreenshot detects whether OCR text is from a ride-hailing app screenshot.
// Returns "uber", "grab", or "" (not a ride-hailing screenshot).
func isAppScreenshot(text string) string {
	lower := strings.ToLower(text)

	uberKeywords := []string{"uber", "uberx", "uber comfort", "uber black", "trip fare", "uber pool", "uber xl"}
	grabKeywords := []string{"grabcar", "justgrab", "grabpay", "grab car", "grab ride", "grabbike"}

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

	// Also check the bare "grab" and "uber" names, but only score +1
	// to avoid false positives from random text containing these words.
	if strings.Contains(lower, "grab") {
		grabScore++
	}
	if strings.Contains(lower, "uber") {
		uberScore++
	}

	// Need at least 2 keyword hits to be confident it's a ride-hailing screenshot.
	if uberScore >= 2 && uberScore >= grabScore {
		return "uber"
	}
	if grabScore >= 2 {
		return "grab"
	}

	return ""
}

// reAmount matches currency amounts like "₱234.00", "P1,500.00", "234.00", "1,500".
var reAmount = regexp.MustCompile(`(?i)(?:₱|PHP|PhP|Php|P|S\$|SGD|Rs\.?|LKR|රු)?\s*([\d,]+(?:\.\d{2})?)`)

// reDateLine matches common date patterns in trip history.
var reDateLine = regexp.MustCompile(`(?i)(?:` +
	`\d{4}-\d{2}-\d{2}` + // ISO: 2025-03-12
	`|\d{1,2}/\d{1,2}/\d{2,4}` + // Slash: 3/12/2025 or 03/12/25
	`|\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\w*(?:\s+\d{2,4})?` + // 12 Mar 2025, 12 March
	`|(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\w*\s+\d{1,2}(?:,?\s+\d{2,4})?` + // Mar 12, 2025
	`)`)

// parseAppTrips parses multiple trip entries from a ride-hailing app screenshot.
// It scans OCR lines for amount anchors and extracts date + route description for each.
func parseAppTrips(lines []string, appType string, jCfg jurisdiction.Config) []ReceiptResult {
	// Build amount regex from jurisdiction config, with fallback to generic pattern.
	var amtRe *regexp.Regexp
	if len(jCfg.AmountPatterns) > 0 {
		amtRe = regexp.MustCompile(jCfg.AmountPatterns[0])
	}

	vendor := capitalize(appType) // "Uber" or "Grab"

	type tripAnchor struct {
		lineIdx int
		amount  float64
	}

	// Phase 1: find all amount-bearing lines.
	var anchors []tripAnchor
	for i, line := range lines {
		amt := extractAmountFromLine(line, amtRe)
		if amt <= 0 {
			// Try generic pattern.
			amt = extractAmountFromLine(line, nil)
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
		// Determine search window: from previous anchor (or start) to current anchor.
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
			// Try to extract a date.
			if date == "" {
				if m := reDateLine.FindString(line); m != "" {
					date = m
					continue
				}
			}
			// Non-amount, non-date lines before the anchor are route descriptions.
			if route == "" && j != anchor.lineIdx {
				// Skip lines that are just amounts or very short.
				if len(line) > 3 && extractAmountFromLine(line, amtRe) <= 0 {
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
			// Use route as a description hint — store in vendor name suffix.
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
