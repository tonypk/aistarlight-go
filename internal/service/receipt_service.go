package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// ReceiptService orchestrates receipt OCR processing.
type ReceiptService struct {
	q      *sqlc.Queries
	ocr    OCRClient
	vendor *VendorService
	ai     AIClient
}

// OCRClient is the interface for the OCR microservice.
type OCRClient interface {
	ExtractText(ctx context.Context, imagePath string) (*OCRResult, error)
}

// AIClient is the interface for LLM-based extraction (subset of openai.Client).
type AIClient interface {
	ChatCompletion(ctx context.Context, messages []oai.ChatCompletionMessage, opts ...openai.RequestOption) (oai.ChatCompletionResponse, error)
}

// OCRResult holds the raw OCR output.
type OCRResult struct {
	Text      string   `json:"text"`
	Lines     []string `json:"lines"`
	LineCount int      `json:"line_count"`
}

// NewReceiptService creates a ReceiptService.
func NewReceiptService(q *sqlc.Queries, ocr OCRClient, vendor *VendorService, ai ...AIClient) *ReceiptService {
	s := &ReceiptService{q: q, ocr: ocr, vendor: vendor}
	if len(ai) > 0 {
		s.ai = ai[0]
	}
	return s
}

// ParsedField holds a parsed value with confidence.
type ParsedField struct {
	Value      interface{} `json:"value"`
	Confidence float64     `json:"confidence"`
}

// DetectedAmount represents a labeled amount found on a receipt.
type DetectedAmount struct {
	Label        string  `json:"label"`
	Amount       float64 `json:"amount"`
	IsLikelyTotal bool   `json:"is_likely_total"`
}

// LineItem represents a single item on a receipt.
type LineItem struct {
	Name   string  `json:"name"`
	Qty    float64 `json:"qty,omitempty"`
	Amount float64 `json:"amount,omitempty"`
}

// ParsedReceipt holds all fields extracted from a receipt.
type ParsedReceipt struct {
	VendorName      ParsedField      `json:"vendor_name"`
	TIN             ParsedField      `json:"tin"`
	Date            ParsedField      `json:"date"`
	TotalAmount     ParsedField      `json:"total_amount"`
	VatableSales    ParsedField      `json:"vatable_sales"`
	VATAmount       ParsedField      `json:"vat_amount"`
	VATType         ParsedField      `json:"vat_type"`
	Category        ParsedField      `json:"category"`
	ReceiptNumber   ParsedField      `json:"receipt_number"`
	DetectedAmounts []DetectedAmount `json:"detected_amounts,omitempty"`
	LineItems       []LineItem       `json:"line_items,omitempty"`
}

// ReceiptResult holds a single receipt processing result.
type ReceiptResult struct {
	Filename          string       `json:"filename"`
	Parsed            ParsedReceipt `json:"parsed"`
	OverallConfidence float64      `json:"overall_confidence"`
	Error             string       `json:"error,omitempty"`
}

// ProcessBatch processes a batch of receipt images.
// jurisdictionCode determines country-specific parsing rules (defaults to "PH").
// hint is an optional user instruction/caption that can trigger app screenshot detection.
func (s *ReceiptService) ProcessBatch(ctx context.Context, companyID, userID uuid.UUID, imagePaths []string, period, reportType, jurisdictionCode, hint string) (*sqlc.ReceiptBatch, []ReceiptResult, error) {
	jCfg := jurisdiction.Get(jurisdictionCode)
	if reportType == "" {
		reportType = jCfg.DefaultReport
	}

	batch, err := s.q.CreateReceiptBatch(ctx, sqlc.CreateReceiptBatchParams{
		ID:             uuid.New(),
		CompanyID:      companyID,
		UserID:         userID,
		Status:         "processing",
		TotalImages:    int32(len(imagePaths)),
		ProcessedCount: 0,
		ReportType:     reportType,
		Period:         period,
		Results:        []byte("[]"),
		ImagePath:      nil,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create batch: %w", err)
	}

	var results []ReceiptResult
	processedCount := 0

	for _, path := range imagePaths {
		imageResults := s.processOneImage(ctx, path, jCfg, hint)
		results = append(results, imageResults...)
		for _, r := range imageResults {
			if r.Error == "" {
				processedCount++
			}
		}
	}

	resultsJSON, _ := json.Marshal(results)
	status := "completed"
	if processedCount == 0 && len(imagePaths) > 0 {
		status = "failed"
	}

	_ = s.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:             batch.ID,
		Status:         status,
		ProcessedCount: int32(processedCount),
		Results:        resultsJSON,
		ImagePath:      nil,
	})

	// Auto-discover vendors from parsed receipts
	if s.vendor != nil {
		for _, r := range results {
			tin, _ := r.Parsed.TIN.Value.(string)
			vendorName, _ := r.Parsed.VendorName.Value.(string)
			if tin != "" && vendorName != "" {
				if _, err := s.vendor.FindOrCreate(ctx, companyID, tin, vendorName); err != nil {
					slog.Warn("auto-create vendor from receipt failed",
						"tin", tin, "vendor", vendorName, "error", err)
				}
			}
		}
	}

	slog.Info("receipt batch processed",
		"batch_id", batch.ID,
		"total", len(imagePaths),
		"processed", processedCount,
	)

	return &batch, results, nil
}

// GetBatch retrieves a receipt batch by ID.
func (s *ReceiptService) GetBatch(ctx context.Context, id uuid.UUID) (*sqlc.ReceiptBatch, error) {
	batch, err := s.q.GetReceiptBatchByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %w", err)
	}
	return &batch, nil
}

// ListBatches lists receipt batches for a company.
func (s *ReceiptService) ListBatches(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]sqlc.ReceiptBatch, int64, error) {
	batches, err := s.q.ListReceiptBatchesByCompany(ctx, sqlc.ListReceiptBatchesByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.q.CountReceiptBatchesByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	return batches, total, nil
}

func (s *ReceiptService) processOneImage(ctx context.Context, imagePath string, jCfg jurisdiction.Config, hint string) []ReceiptResult {
	if s.ocr == nil {
		return []ReceiptResult{{Filename: imagePath, Error: "OCR service not configured"}}
	}

	ocrResult, err := s.ocr.ExtractText(ctx, imagePath)
	if err != nil {
		return []ReceiptResult{{Filename: imagePath, Error: fmt.Sprintf("OCR failed: %v", err)}}
	}

	// Check for ride-hailing app screenshot (Uber/Grab multi-trip).
	if appType := isAppScreenshot(ocrResult.Text, hint); appType != "" {
		trips := parseAppTrips(ocrResult.Lines, appType, jCfg)
		if len(trips) > 0 {
			// Set filename on all trip results.
			for i := range trips {
				trips[i].Filename = imagePath
			}
			slog.Info("app screenshot detected",
				"app", appType,
				"trips", len(trips),
				"image", imagePath,
			)
			return trips
		}
	}

	// Normal receipt processing.
	parsed := parseReceipt(ocrResult.Text, ocrResult.Lines, jCfg)

	// AI line-item extraction (non-blocking — best effort).
	if s.ai != nil && len(ocrResult.Lines) >= 3 {
		items, err := s.extractLineItems(ctx, ocrResult.Lines)
		if err != nil {
			slog.Warn("AI line item extraction failed", "error", err, "image", imagePath)
		} else {
			parsed.LineItems = items
		}
	}

	return []ReceiptResult{{
		Filename:          imagePath,
		Parsed:            parsed,
		OverallConfidence: AverageConfidence(parsed),
	}}
}

// extractLineItems uses AI to extract individual purchased items from OCR text.
func (s *ReceiptService) extractLineItems(ctx context.Context, lines []string) ([]LineItem, error) {
	// Build compact OCR text (limit to avoid token waste)
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
		if sb.Len() > 2000 {
			break
		}
	}

	resp, err := s.ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{
			Role:    oai.ChatMessageRoleSystem,
			Content: lineItemExtractionPrompt,
		},
		{
			Role:    oai.ChatMessageRoleUser,
			Content: sb.String(),
		},
	}, openai.WithTemperature(0.1), openai.WithMaxTokens(1024), openai.WithJSONResponse())
	if err != nil {
		return nil, fmt.Errorf("AI extraction: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no AI response")
	}

	var result struct {
		Items []LineItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	// Filter out empty or summary items
	var filtered []LineItem
	for _, item := range result.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		filtered = append(filtered, LineItem{
			Name:   name,
			Qty:    item.Qty,
			Amount: item.Amount,
		})
	}

	return filtered, nil
}

const lineItemExtractionPrompt = `You are a receipt OCR parser. Extract ONLY the purchased line items (products/services) from the receipt text below.

Rules:
- Extract individual items the customer purchased
- Include item name, quantity (default 1 if not shown), and line amount if visible
- Do NOT include: totals, subtotals, VAT/tax lines, change, payment method, receipt headers, store info, cashier info, date/time
- Keep item names concise but recognizable (e.g. "Coca-Cola 1.5L", "Rice 5kg", "Haircut")
- If the receipt has no identifiable line items (e.g. only totals), return empty items array
- For service receipts (taxi, utilities, etc.), extract the service description as a single item

Respond with JSON only:
{"items": [{"name": "item name", "qty": 1, "amount": 123.45}, ...]}`

// Receipt parsing — rule-based extraction

var receiptNoRe = regexp.MustCompile(`(?i)(?:OR|Invoice|SI|Receipt)\s*(?:No\.?|#)\s*[:\s]*([\w-]+)`)

func parseReceipt(text string, lines []string, jCfg jurisdiction.Config) ParsedReceipt {
	p := ParsedReceipt{}
	upperText := strings.ToUpper(text)

	// TIN — use jurisdiction-specific pattern and normalization
	tinRe := regexp.MustCompile(jCfg.TINPattern)
	if match := tinRe.FindString(text); match != "" {
		normalized := normalizeTIN(match, jCfg.Code)
		p.TIN = ParsedField{Value: normalized, Confidence: 0.95}
	}

	// Build currency-aware amount regex from jurisdiction config
	var amountRe *regexp.Regexp
	if len(jCfg.AmountPatterns) > 0 {
		amountRe = regexp.MustCompile(jCfg.AmountPatterns[0])
	}

	// Date — jurisdiction determines DD/MM vs MM/DD interpretation
	p.Date = extractDate(text, jCfg.Code)

	// Amounts — use jurisdiction-specific labels
	p.TotalAmount = extractLabeledAmount(lines, jCfg.TotalLabels, 0.85, amountRe)
	p.VatableSales = extractLabeledAmount(lines, jCfg.VatableLabels, 0.85, amountRe)
	p.VATAmount = extractLabeledAmount(lines, jCfg.VATLabels, 0.85, amountRe)

	// Extract all labeled amounts for multi-amount selection (Approach C).
	p.DetectedAmounts = extractAllLabeledAmounts(lines, amountRe)

	// If no labeled total, use largest amount
	if p.TotalAmount.Value == nil {
		p.TotalAmount = extractLargestAmount(text, 0.75)
	}

	// Cross-validate: vatable + vat = total
	if p.VatableSales.Value != nil && p.VATAmount.Value != nil && p.TotalAmount.Value != nil {
		vatable, _ := p.VatableSales.Value.(float64)
		vat, _ := p.VATAmount.Value.(float64)
		total, _ := p.TotalAmount.Value.(float64)
		if math.Abs((vatable+vat)-total) < 1.0 {
			p.TotalAmount.Confidence = 0.95
		}
	}

	// VAT type — use jurisdiction-specific labels
	if containsAny(upperText, jCfg.ExemptLabels) {
		p.VATType = ParsedField{Value: "exempt", Confidence: 0.90}
	} else if jCfg.Code == "LK" && containsAnyCI(text, []string{"SVAT", "SUSPENDED VAT"}) {
		// LK SVAT is a distinct type, not zero_rated.
		p.VATType = ParsedField{Value: "svat", Confidence: 0.90}
	} else if containsAny(upperText, jCfg.ZeroRatedLabels) {
		p.VATType = ParsedField{Value: "zero_rated", Confidence: 0.90}
	} else if containsAny(upperText, jCfg.VatableLabels) || p.VATAmount.Value != nil {
		defaultVATType := "vatable"
		if len(jCfg.VATTypes) > 0 {
			defaultVATType = jCfg.VATTypes[0] // first type is the standard one
		}
		p.VATType = ParsedField{Value: defaultVATType, Confidence: 0.90}
	} else {
		defaultVATType := "vatable"
		if len(jCfg.VATTypes) > 0 {
			defaultVATType = jCfg.VATTypes[0]
		}
		p.VATType = ParsedField{Value: defaultVATType, Confidence: 0.50}
	}

	// Category default
	p.Category = ParsedField{Value: "goods", Confidence: 0.50}

	// Receipt number
	if match := receiptNoRe.FindStringSubmatch(text); len(match) > 1 {
		p.ReceiptNumber = ParsedField{Value: match[1], Confidence: 0.90}
	}

	// Vendor name — first non-header line
	if len(lines) > 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 3 && !isHeaderLine(trimmed) {
				p.VendorName = ParsedField{Value: trimmed, Confidence: 0.70}
				break
			}
		}
	}

	return p
}

func normalizeTIN(raw, jurisdictionCode string) string {
	digits := regexp.MustCompile(`\d`).FindAllString(raw, -1)
	joined := strings.Join(digits, "")
	switch jurisdictionCode {
	case "PH":
		// PH TIN: XXX-XXX-XXX-XXXX
		if len(joined) >= 12 {
			return fmt.Sprintf("%s-%s-%s-%s", joined[:3], joined[3:6], joined[6:9], joined[9:])
		}
	case "LK":
		// LK TIN: 9-digit numeric, return as-is
		return strings.TrimSpace(raw)
	case "SG":
		// SG UEN: alphanumeric, return as-is
		return strings.TrimSpace(raw)
	}
	return raw
}

var (
	reDateSlash2  = regexp.MustCompile(`\d{2}/\d{2}/\d{4}`)
	reDateISO     = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	reDateSlash1  = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)
	reDateDash    = regexp.MustCompile(`\d{2}-\d{2}-\d{4}`)
	reDateTextMon = regexp.MustCompile(`(?i)\d{1,2}\s+(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+\d{4}`)
)

// useDDMMFirst returns true for jurisdictions using DD/MM date order.
func useDDMMFirst(code string) bool {
	return code == "LK" || code == "SG"
}

func extractDate(text string, jurisdictionCode string) ParsedField {
	ddmm := useDDMMFirst(jurisdictionCode)

	// ISO dates are unambiguous — try first.
	if match := reDateISO.FindString(text); match != "" {
		return ParsedField{Value: match, Confidence: 0.95}
	}

	// "15 January 2025" style — unambiguous.
	if match := reDateTextMon.FindString(text); match != "" {
		return ParsedField{Value: match, Confidence: 0.95}
	}

	// DD/MM/YYYY vs MM/DD/YYYY — jurisdiction determines layout.
	if match := reDateSlash2.FindString(text); match != "" {
		if ddmm {
			return ParsedField{Value: match, Confidence: 0.90}
		}
		return ParsedField{Value: match, Confidence: 0.90}
	}

	// DD-MM-YYYY — common in LK.
	if match := reDateDash.FindString(text); match != "" {
		return ParsedField{Value: match, Confidence: 0.90}
	}

	// Flexible single-digit day/month.
	if match := reDateSlash1.FindString(text); match != "" {
		return ParsedField{Value: match, Confidence: 0.85}
	}

	return ParsedField{}
}

// nonTotalPrefixes are line fragments that indicate the line is NOT the real total,
// even if the word "TOTAL" appears in it.
var nonTotalPrefixes = []string{
	"SERVICE CHARGE", "DELIVERY FEE", "DELIVERY CHARGE",
	"SUBTOTAL", "SUB TOTAL", "SUB-TOTAL",
	"DISCOUNT", "CASH", "CHANGE", "TENDERED",
	"NO. OF ITEMS", "NUMBER OF ITEMS", "QTY",
	// Chinese non-total terms (cashier / change / received).
	"实收", "找零", "已收", "收银", "找赎",
}

// isNonTotalLine returns true when the line contains a label like "TOTAL" but is
// actually a non-total line (e.g. "SERVICE CHARGE TOTAL").
// genericTotalLabels are labels that are short / generic enough to warrant
// non-total prefix exclusion (e.g. a line "实收 TOTAL 500" should be rejected).
var genericTotalLabels = map[string]bool{
	"TOTAL": true, "NET AMOUNT": true,
	// Chinese generic labels (short, easily confused).
	"支付": true, "付款": true, "应付": true,
	"合计": true, "总计": true, "消费": true,
}

func isNonTotalLine(upper, matchedLabel string) bool {
	// Only apply the exclusion when the matched label is generic (e.g. "TOTAL").
	// Specific labels like "GRAND TOTAL" or "TOTAL AMOUNT" are safe.
	if !genericTotalLabels[matchedLabel] {
		return false
	}
	for _, prefix := range nonTotalPrefixes {
		if strings.Contains(upper, prefix) {
			return true
		}
	}
	return false
}

func extractLabeledAmount(lines []string, labels []string, confidence float64, amountRe *regexp.Regexp) ParsedField {
	// Two passes: first try specific labels (len>5), then generic ones.
	// This ensures "GRAND TOTAL" or "TOTAL AMOUNT" is found before bare "TOTAL".
	for pass := 0; pass < 2; pass++ {
		for i, line := range lines {
			upper := strings.ToUpper(line)
			for _, label := range labels {
				isSpecific := len(label) > 5
				if (pass == 0) != isSpecific {
					continue // pass 0 = specific only, pass 1 = generic only
				}
				if !containsLabelWord(upper, label) {
					continue
				}
				if isNonTotalLine(upper, label) {
					continue
				}
				if amt := extractAmountFromLine(line, amountRe); amt > 0 {
					return ParsedField{Value: amt, Confidence: confidence}
				}
				// Look-ahead: PaddleOCR may put the amount on the next line.
				if i+1 < len(lines) {
					if amt := extractAmountFromLine(lines[i+1], amountRe); amt > 0 {
						return ParsedField{Value: amt, Confidence: confidence * 0.95}
					}
				}
			}
		}
	}
	return ParsedField{}
}

// allAmountLabels defines all possible receipt amount labels across jurisdictions.
// The order defines priority: earlier entries are more likely to be the "correct" total.
var allAmountLabels = []struct {
	Label         string
	IsLikelyTotal bool
}{
	// English total labels (high priority).
	{"NET TOTAL", true},
	{"NET AMOUNT", true},
	{"TOTAL AMOUNT", true},
	{"GRAND TOTAL", true},
	{"TOTAL DUE", true},
	{"AMOUNT DUE", true},
	{"TOTAL PAYABLE", true},
	{"BALANCE DUE", true},
	{"BILL TOTAL", true},
	{"TOTAL (SGD)", true},
	{"TOTAL (LKR)", true},
	{"TOTAL", true},
	// Chinese total labels — payment / total terms.
	{"总金额", true},
	{"总数", true},
	{"支付", true},
	{"付款", true},
	{"应付", true},
	{"合计", true},
	{"总计", true},
	{"消费", true},
	// English non-total labels.
	{"SUBTOTAL", false},
	{"SUB TOTAL", false},
	{"SUB-TOTAL", false},
	{"VATABLE SALES", false},
	{"TAXABLE AMOUNT", false},
	{"AMOUNT BEFORE GST", false},
	{"AMOUNT BEFORE VAT", false},
	{"NET", false},
	{"CASH", false},
	{"CHANGE", false},
	{"DISCOUNT", false},
	{"VAT AMOUNT", false},
	{"GST AMOUNT", false},
	{"12% VAT", false},
	{"9% GST", false},
	{"18% VAT", false},
	{"SERVICE CHARGE", false},
	{"DELIVERY FEE", false},
	// Chinese non-total labels — received / change / subtotal.
	{"实收", false},
	{"找零", false},
	{"已收", false},
	{"小计", false},
	{"折扣", false},
	{"找赎", false},
}

// containsLabelWord checks if text contains label as a word (not as a substring of another word).
// E.g., "SUBTOTAL" does NOT contain "TOTAL" as a word, but "TOTAL   326.00" does.
func containsLabelWord(text, label string) bool {
	idx := strings.Index(text, label)
	if idx < 0 {
		return false
	}
	// Check character before the match — must be start of string or non-alpha.
	if idx > 0 {
		before := text[idx-1]
		if (before >= 'A' && before <= 'Z') || (before >= 'a' && before <= 'z') {
			return false
		}
	}
	// Check character after the match — must be end of string or non-alpha.
	end := idx + len(label)
	if end < len(text) {
		after := text[end]
		if (after >= 'A' && after <= 'Z') || (after >= 'a' && after <= 'z') {
			return false
		}
	}
	return true
}

// extractAllLabeledAmounts extracts all labeled amounts from OCR lines.
// Returns unique amounts with their labels, deduplicating amounts that appear on multiple matching labels.
func extractAllLabeledAmounts(lines []string, amountRe *regexp.Regexp) []DetectedAmount {
	var detected []DetectedAmount
	seen := make(map[float64]bool)

	for _, def := range allAmountLabels {
		for i, line := range lines {
			upper := strings.ToUpper(line)
			if !containsLabelWord(upper, def.Label) {
				continue
			}
			amt := extractAmountFromLine(line, amountRe)
			// Look-ahead: PaddleOCR may put the amount on the next line.
			if amt <= 0 && i+1 < len(lines) {
				amt = extractAmountFromLine(lines[i+1], amountRe)
			}
			if amt <= 0 {
				continue
			}
			if seen[amt] {
				continue
			}
			seen[amt] = true
			detected = append(detected, DetectedAmount{
				Label:         def.Label,
				Amount:        amt,
				IsLikelyTotal: def.IsLikelyTotal,
			})
			break // only take first match per label
		}
	}

	return detected
}

// SelectAmountByInstruction matches a user instruction against detected amounts.
// Returns the matched amount and true if found, or zero and false.
func SelectAmountByInstruction(amounts []DetectedAmount, instruction string) (DetectedAmount, bool) {
	if instruction == "" || len(amounts) == 0 {
		return DetectedAmount{}, false
	}
	lower := strings.ToLower(instruction)

	// Direct match: instruction mentions a label name (word boundary).
	// Sort by label length descending to match longer labels first (e.g., "subtotal" before "total").
	for _, da := range amounts {
		labelLower := strings.ToLower(da.Label)
		if containsLabelWord(lower, labelLower) {
			return da, true
		}
	}

	// Fuzzy keywords from instruction.
	keywordMap := map[string][]string{
		"net":       {"NET TOTAL", "NET AMOUNT", "NET"},
		"subtotal":  {"SUBTOTAL", "SUB TOTAL", "SUB-TOTAL"},
		"sub total": {"SUBTOTAL", "SUB TOTAL", "SUB-TOTAL"},
		"grand":     {"GRAND TOTAL"},
		"due":       {"AMOUNT DUE", "TOTAL DUE", "BALANCE DUE"},
		"balance":   {"BALANCE DUE"},
		"cash":      {"CASH"},
		"vat":       {"VAT AMOUNT"},
		"gst":       {"GST AMOUNT"},
		"discount":  {"DISCOUNT"},
		"总额":       {"TOTAL", "TOTAL AMOUNT", "GRAND TOTAL"},
		"净额":       {"NET TOTAL", "NET AMOUNT", "NET"},
		"小计":       {"SUBTOTAL", "SUB TOTAL"},
		"税额":       {"VAT AMOUNT", "GST AMOUNT"},
		"应付":       {"AMOUNT DUE", "TOTAL DUE", "TOTAL PAYABLE"},
		"现金":       {"CASH"},
		"找零":       {"CHANGE"},
		"折扣":       {"DISCOUNT"},
	}

	for kw, targetLabels := range keywordMap {
		if !strings.Contains(lower, kw) {
			continue
		}
		for _, da := range amounts {
			for _, tl := range targetLabels {
				if da.Label == tl {
					return da, true
				}
			}
		}
	}

	return DetectedAmount{}, false
}

// SelectBestAmount picks the best total amount from detected amounts using priority rules.
// Priority: NET TOTAL > TOTAL AMOUNT > GRAND TOTAL > TOTAL > AMOUNT DUE > others.
func SelectBestAmount(amounts []DetectedAmount) (DetectedAmount, bool) {
	if len(amounts) == 0 {
		return DetectedAmount{}, false
	}

	// If only one amount, use it.
	if len(amounts) == 1 {
		return amounts[0], true
	}

	// If only one is_likely_total, use it.
	var totals []DetectedAmount
	for _, da := range amounts {
		if da.IsLikelyTotal {
			totals = append(totals, da)
		}
	}
	if len(totals) == 1 {
		return totals[0], true
	}

	// Multiple totals: return the first one (allAmountLabels order = priority).
	if len(totals) > 0 {
		return totals[0], true
	}

	// No totals: return the largest amount.
	best := amounts[0]
	for _, da := range amounts[1:] {
		if da.Amount > best.Amount {
			best = da
		}
	}
	return best, true
}

// NeedsAmountSelection returns true if multiple distinct total-like amounts are detected
// and user should choose which one to use.
func NeedsAmountSelection(amounts []DetectedAmount) bool {
	if len(amounts) <= 1 {
		return false
	}
	// Count distinct total-like amounts (amounts where IsLikelyTotal=true).
	var totalCount int
	for _, da := range amounts {
		if da.IsLikelyTotal {
			totalCount++
		}
	}
	// If 2+ total-like amounts with different values, user should choose.
	if totalCount >= 2 {
		return true
	}
	// If many amounts (>=3) even including non-total ones, show selection.
	return len(amounts) >= 3
}

func extractAmountFromLine(line string, amountRe *regexp.Regexp) float64 {
	if amountRe != nil {
		if match := amountRe.FindStringSubmatch(line); len(match) > 1 {
			s := strings.ReplaceAll(match[1], ",", "")
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return f
			}
		}
	}
	// Try plain number
	re := regexp.MustCompile(`[\d,]+\.\d{2}`)
	if match := re.FindString(line); match != "" {
		s := strings.ReplaceAll(match, ",", "")
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return f
		}
	}
	return 0
}

func extractLargestAmount(text string, confidence float64) ParsedField {
	re := regexp.MustCompile(`[\d,]+\.\d{2}`)
	matches := re.FindAllString(text, -1)

	var largest float64
	for _, m := range matches {
		s := strings.ReplaceAll(m, ",", "")
		f, err := strconv.ParseFloat(s, 64)
		if err == nil && f > largest {
			largest = f
		}
	}
	if largest > 0 {
		return ParsedField{Value: largest, Confidence: confidence}
	}
	return ParsedField{}
}

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func containsAnyCI(text string, keywords []string) bool {
	upper := strings.ToUpper(text)
	for _, kw := range keywords {
		if strings.Contains(upper, strings.ToUpper(kw)) {
			return true
		}
	}
	return false
}

func isHeaderLine(line string) bool {
	headers := []string{
		// PH
		"OFFICIAL RECEIPT", "SALES INVOICE", "DELIVERY RECEIPT", "CHARGE INVOICE",
		// LK / SG / General
		"TAX INVOICE", "VAT INVOICE", "SIMPLIFIED VAT", "FISCAL INVOICE",
		"RETAIL INVOICE", "CASH BILL", "CREDIT NOTE", "DEBIT NOTE",
		"PROFORMA INVOICE", "COMMERCIAL INVOICE",
	}
	upper := strings.ToUpper(line)
	for _, h := range headers {
		if strings.Contains(upper, h) {
			return true
		}
	}
	return false
}

// AverageConfidence computes the average confidence of non-zero fields.
func AverageConfidence(p ParsedReceipt) float64 {
	fields := []float64{
		p.VendorName.Confidence, p.TIN.Confidence, p.Date.Confidence,
		p.TotalAmount.Confidence, p.VATType.Confidence, p.Category.Confidence,
	}
	var sum float64
	count := 0
	for _, c := range fields {
		if c > 0 {
			sum += c
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// NeedsLLM returns fields below confidence thresholds that need AI resolution.
func NeedsLLM(p ParsedReceipt) map[string]float64 {
	ambiguous := make(map[string]float64)
	if p.Category.Confidence < 0.85 {
		ambiguous["category"] = p.Category.Confidence
	}
	if p.VATType.Confidence < 0.85 {
		ambiguous["vat_type"] = p.VATType.Confidence
	}
	if p.TotalAmount.Confidence < 0.70 {
		ambiguous["total_amount"] = p.TotalAmount.Confidence
	}
	if p.VendorName.Confidence < 0.60 {
		ambiguous["vendor_name"] = p.VendorName.Confidence
	}
	return ambiguous
}

// Ensure pgtype imports are available for batch operations
var _ pgtype.UUID
