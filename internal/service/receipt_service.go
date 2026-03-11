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
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// ReceiptService orchestrates receipt OCR processing.
type ReceiptService struct {
	q        *sqlc.Queries
	ocr      OCRClient
	supplier *SupplierService
}

// OCRClient is the interface for the OCR microservice.
type OCRClient interface {
	ExtractText(ctx context.Context, imagePath string) (*OCRResult, error)
}

// OCRResult holds the raw OCR output.
type OCRResult struct {
	Text      string   `json:"text"`
	Lines     []string `json:"lines"`
	LineCount int      `json:"line_count"`
}

// NewReceiptService creates a ReceiptService.
func NewReceiptService(q *sqlc.Queries, ocr OCRClient, supplier *SupplierService) *ReceiptService {
	return &ReceiptService{q: q, ocr: ocr, supplier: supplier}
}

// ParsedField holds a parsed value with confidence.
type ParsedField struct {
	Value      interface{} `json:"value"`
	Confidence float64     `json:"confidence"`
}

// ParsedReceipt holds all fields extracted from a receipt.
type ParsedReceipt struct {
	VendorName    ParsedField `json:"vendor_name"`
	TIN           ParsedField `json:"tin"`
	Date          ParsedField `json:"date"`
	TotalAmount   ParsedField `json:"total_amount"`
	VatableSales  ParsedField `json:"vatable_sales"`
	VATAmount     ParsedField `json:"vat_amount"`
	VATType       ParsedField `json:"vat_type"`
	Category      ParsedField `json:"category"`
	ReceiptNumber ParsedField `json:"receipt_number"`
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
func (s *ReceiptService) ProcessBatch(ctx context.Context, companyID, userID uuid.UUID, imagePaths []string, period, reportType, jurisdictionCode string) (*sqlc.ReceiptBatch, []ReceiptResult, error) {
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
		result := s.processOneReceipt(ctx, path, jCfg)
		results = append(results, result)
		if result.Error == "" {
			processedCount++
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

	// Auto-discover suppliers from parsed receipts
	if s.supplier != nil {
		for _, r := range results {
			tin, _ := r.Parsed.TIN.Value.(string)
			vendorName, _ := r.Parsed.VendorName.Value.(string)
			if tin != "" && vendorName != "" {
				if _, err := s.supplier.FindOrCreate(ctx, companyID, tin, vendorName); err != nil {
					slog.Warn("auto-create supplier from receipt failed",
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

func (s *ReceiptService) processOneReceipt(ctx context.Context, imagePath string, jCfg jurisdiction.Config) ReceiptResult {
	result := ReceiptResult{Filename: imagePath}

	if s.ocr == nil {
		result.Error = "OCR service not configured"
		return result
	}

	ocrResult, err := s.ocr.ExtractText(ctx, imagePath)
	if err != nil {
		result.Error = fmt.Sprintf("OCR failed: %v", err)
		return result
	}

	parsed := parseReceipt(ocrResult.Text, ocrResult.Lines, jCfg)
	result.Parsed = parsed
	result.OverallConfidence = AverageConfidence(parsed)

	return result
}

// Receipt parsing — rule-based extraction

var receiptNoRe = regexp.MustCompile(`(?i)(?:OR|Invoice|SI|Receipt)\s*(?:No\.?|#)\s*[:\s]*([\w-]+)`)

func parseReceipt(text string, lines []string, jCfg jurisdiction.Config) ParsedReceipt {
	p := ParsedReceipt{}
	upperText := strings.ToUpper(text)

	// TIN — use jurisdiction-specific pattern
	tinRe := regexp.MustCompile(jCfg.TINPattern)
	if match := tinRe.FindString(text); match != "" {
		normalized := normalizeTIN(match)
		p.TIN = ParsedField{Value: normalized, Confidence: 0.95}
	}

	// Build currency-aware amount regex from jurisdiction config
	var amountRe *regexp.Regexp
	if len(jCfg.AmountPatterns) > 0 {
		amountRe = regexp.MustCompile(jCfg.AmountPatterns[0])
	}

	// Date
	p.Date = extractDate(text)

	// Amounts — use jurisdiction-specific labels
	p.TotalAmount = extractLabeledAmount(lines, jCfg.TotalLabels, 0.85, amountRe)
	p.VatableSales = extractLabeledAmount(lines, jCfg.VatableLabels, 0.85, amountRe)
	p.VATAmount = extractLabeledAmount(lines, jCfg.VATLabels, 0.85, amountRe)

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

func normalizeTIN(raw string) string {
	digits := regexp.MustCompile(`\d`).FindAllString(raw, -1)
	joined := strings.Join(digits, "")
	if len(joined) >= 12 {
		return fmt.Sprintf("%s-%s-%s-%s", joined[:3], joined[3:6], joined[6:9], joined[9:])
	}
	return raw
}

var datePatterns = []struct {
	re     *regexp.Regexp
	layout string
}{
	{regexp.MustCompile(`\d{2}/\d{2}/\d{4}`), "01/02/2006"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "2006-01-02"},
	{regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`), "1/2/2006"},
}

func extractDate(text string) ParsedField {
	for _, dp := range datePatterns {
		if match := dp.re.FindString(text); match != "" {
			return ParsedField{Value: match, Confidence: 0.90}
		}
	}
	return ParsedField{}
}

func extractLabeledAmount(lines []string, labels []string, confidence float64, amountRe *regexp.Regexp) ParsedField {
	for _, line := range lines {
		upper := strings.ToUpper(line)
		for _, label := range labels {
			if strings.Contains(upper, label) {
				if amt := extractAmountFromLine(line, amountRe); amt > 0 {
					return ParsedField{Value: amt, Confidence: confidence}
				}
			}
		}
	}
	return ParsedField{}
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

func isHeaderLine(line string) bool {
	headers := []string{"OFFICIAL RECEIPT", "SALES INVOICE", "DELIVERY RECEIPT", "CHARGE INVOICE"}
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
