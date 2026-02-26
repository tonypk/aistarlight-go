package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// SessionService manages reconciliation sessions.
type SessionService struct {
	q          *sqlc.Queries
	classifier *ClassifierService
}

// NewSessionService creates a SessionService.
func NewSessionService(q *sqlc.Queries, classifier *ClassifierService) *SessionService {
	return &SessionService{q: q, classifier: classifier}
}

// SessionResponse is the API response for a session.
type SessionResponse struct {
	ID                   uuid.UUID   `json:"id"`
	Period               string      `json:"period"`
	Status               string      `json:"status"`
	ReportID             *string     `json:"report_id"`
	SourceFiles          interface{} `json:"source_files"`
	Summary              interface{} `json:"summary"`
	ReconciliationResult interface{} `json:"reconciliation_result"`
	CompletedAt          *string     `json:"completed_at"`
	CreatedAt            string      `json:"created_at"`
	UpdatedAt            string      `json:"updated_at"`
}

// TransactionResponse is the API response for a transaction.
type TransactionResponse struct {
	ID                   string   `json:"id"`
	SourceType           string   `json:"source_type"`
	SourceFileID         string   `json:"source_file_id"`
	RowIndex             int      `json:"row_index"`
	Date                 *string  `json:"date"`
	Description          *string  `json:"description"`
	Amount               float64  `json:"amount"`
	VATAmount            float64  `json:"vat_amount"`
	VATType              string   `json:"vat_type"`
	Category             string   `json:"category"`
	TIN                  *string  `json:"tin"`
	Confidence           float64  `json:"confidence"`
	ClassificationSource string   `json:"classification_source"`
	MatchGroupID         *string  `json:"match_group_id"`
	MatchStatus          string   `json:"match_status"`
	EWTRate              *float64 `json:"ewt_rate"`
	EWTAmount            *float64 `json:"ewt_amount"`
	ATCCode              *string  `json:"atc_code"`
	SupplierID           *string  `json:"supplier_id"`
}

// AnomalyResponse is the API response for an anomaly.
type AnomalyResponse struct {
	ID             string      `json:"id"`
	TransactionID  *string     `json:"transaction_id"`
	AnomalyType    string      `json:"anomaly_type"`
	Severity       string      `json:"severity"`
	Description    string      `json:"description"`
	Details        interface{} `json:"details"`
	Status         string      `json:"status"`
	ResolvedBy     *string     `json:"resolved_by"`
	ResolvedAt     *string     `json:"resolved_at"`
	ResolutionNote *string     `json:"resolution_note"`
	CreatedAt      string      `json:"created_at"`
}

func sessionToResponse(s sqlc.ReconciliationSession) SessionResponse {
	resp := SessionResponse{
		ID:        s.ID,
		Period:    s.Period,
		Status:    s.Status,
		CreatedAt: s.CreatedAt.Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	}
	if s.ReportID.Valid {
		id := uuid.UUID(s.ReportID.Bytes).String()
		resp.ReportID = &id
	}
	if len(s.SourceFiles) > 0 {
		var sf interface{}
		_ = json.Unmarshal(s.SourceFiles, &sf)
		resp.SourceFiles = sf
	} else {
		resp.SourceFiles = []interface{}{}
	}
	if len(s.Summary) > 0 {
		var sm interface{}
		_ = json.Unmarshal(s.Summary, &sm)
		resp.Summary = sm
	}
	if len(s.ReconciliationResult) > 0 {
		var rr interface{}
		_ = json.Unmarshal(s.ReconciliationResult, &rr)
		resp.ReconciliationResult = rr
	}
	if s.CompletedAt.Valid {
		t := s.CompletedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	return resp
}

func txnToResponse(t sqlc.Transaction) TransactionResponse {
	resp := TransactionResponse{
		ID:                   t.ID.String(),
		SourceType:           t.SourceType,
		SourceFileID:         t.SourceFileID,
		RowIndex:             int(t.RowIndex),
		Description:          t.Description,
		VATType:              t.VatType,
		Category:             t.Category,
		TIN:                  t.Tin,
		ClassificationSource: t.ClassificationSource,
		MatchStatus:          t.MatchStatus,
		ATCCode:              t.AtcCode,
	}
	if t.Date.Valid {
		d := t.Date.Time.Format("2006-01-02")
		resp.Date = &d
	}
	if f, err := t.Amount.Float64Value(); err == nil {
		resp.Amount = f.Float64
	}
	if f, err := t.VatAmount.Float64Value(); err == nil {
		resp.VATAmount = f.Float64
	}
	if f, err := t.Confidence.Float64Value(); err == nil {
		resp.Confidence = f.Float64
	}
	if t.MatchGroupID.Valid {
		id := uuid.UUID(t.MatchGroupID.Bytes).String()
		resp.MatchGroupID = &id
	}
	if t.EwtRate.Valid {
		if f, err := t.EwtRate.Float64Value(); err == nil {
			resp.EWTRate = &f.Float64
		}
	}
	if t.EwtAmount.Valid {
		if f, err := t.EwtAmount.Float64Value(); err == nil {
			resp.EWTAmount = &f.Float64
		}
	}
	if t.SupplierID.Valid {
		id := uuid.UUID(t.SupplierID.Bytes).String()
		resp.SupplierID = &id
	}
	return resp
}

func anomalyToResponse(a sqlc.Anomaly) AnomalyResponse {
	resp := AnomalyResponse{
		ID:             a.ID.String(),
		AnomalyType:    a.AnomalyType,
		Severity:       a.Severity,
		Description:    a.Description,
		Status:         a.Status,
		ResolutionNote: a.ResolutionNote,
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
	}
	if a.TransactionID.Valid {
		id := uuid.UUID(a.TransactionID.Bytes).String()
		resp.TransactionID = &id
	}
	if len(a.Details) > 0 {
		var d interface{}
		_ = json.Unmarshal(a.Details, &d)
		resp.Details = d
	}
	if a.ResolvedBy.Valid {
		id := uuid.UUID(a.ResolvedBy.Bytes).String()
		resp.ResolvedBy = &id
	}
	if a.ResolvedAt.Valid {
		t := a.ResolvedAt.Time.Format(time.RFC3339)
		resp.ResolvedAt = &t
	}
	return resp
}

// CreateSession creates a new reconciliation session.
func (s *SessionService) CreateSession(ctx context.Context, companyID, userID uuid.UUID, period string, reportID *uuid.UUID) (*SessionResponse, error) {
	reportUUID := pgtype.UUID{}
	if reportID != nil {
		reportUUID = pgtype.UUID{Bytes: *reportID, Valid: true}
	}

	session, err := s.q.CreateReconciliationSession(ctx, sqlc.CreateReconciliationSessionParams{
		ID:          uuid.New(),
		CompanyID:   companyID,
		CreatedBy:   userID,
		Period:      period,
		Status:      "draft",
		ReportID:    reportUUID,
		SourceFiles: []byte("[]"),
	})
	if err != nil {
		slog.Error("create session failed", "error", err, "company_id", companyID, "user_id", userID, "period", period)
		return nil, fmt.Errorf("create session: %w", err)
	}

	resp := sessionToResponse(session)
	return &resp, nil
}

// GetSession retrieves a session by ID.
func (s *SessionService) GetSession(ctx context.Context, id, companyID uuid.UUID) (*SessionResponse, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("session not found")
	}
	if session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}
	resp := sessionToResponse(session)
	return &resp, nil
}

// ListSessions lists sessions for a company.
func (s *SessionService) ListSessions(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]SessionResponse, int64, error) {
	sessions, err := s.q.ListReconciliationSessionsByCompany(ctx, sqlc.ListReconciliationSessionsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list sessions: %w", err)
	}
	total, err := s.q.CountReconciliationSessionsByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	result := make([]SessionResponse, len(sessions))
	for i, sess := range sessions {
		result[i] = sessionToResponse(sess)
	}
	return result, total, nil
}

// DeleteSession deletes a draft session.
func (s *SessionService) DeleteSession(ctx context.Context, id, companyID uuid.UUID) error {
	session, err := s.q.GetReconciliationSessionByID(ctx, id)
	if err != nil || session.CompanyID != companyID {
		return fmt.Errorf("session not found")
	}
	if session.Status != "draft" {
		return fmt.Errorf("can only delete draft sessions")
	}
	return s.q.DeleteReconciliationSession(ctx, sqlc.DeleteReconciliationSessionParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// AddFileInput holds input for adding a file to a session.
type AddFileInput struct {
	FileID         string                 `json:"file_id"`
	Filename       string                 `json:"filename"`
	SourceType     string                 `json:"source_type"`
	SheetName      string                 `json:"sheet_name,omitempty"`
	ColumnMappings map[string]string      `json:"column_mappings,omitempty"`
	Rows           []map[string]interface{} `json:"rows"`
}

// AddFile adds a file to the session and imports transactions.
func (s *SessionService) AddFile(ctx context.Context, sessionID, companyID uuid.UUID, input AddFileInput) (map[string]interface{}, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	imported := 0
	var warnings []map[string]interface{}
	for i, row := range input.Rows {
		// Apply column mappings: translate raw column names to standardized field names
		if len(input.ColumnMappings) > 0 {
			mapped := make(map[string]interface{}, len(row))
			for rawCol, val := range row {
				if fieldName, ok := input.ColumnMappings[rawCol]; ok {
					mapped[fieldName] = val
				} else {
					mapped[rawCol] = val // preserve unmapped keys (e.g. _source)
				}
			}
			row = mapped
		}

		// Normalize keys: lowercase + spaces→underscores so that raw headers
		// like "TAXABLE MONTH" match expected keys like "taxable_month".
		row = normalizeRowKeys(row)

		sourceType := input.SourceType
		if st := toString(row["_source"]); st == "sales" {
			sourceType = "sales_record"
		} else if st == "purchases" {
			sourceType = "purchase_record"
		}

		// Extract amount: try BIR-specific fields first, then generic fallbacks
		amtKeys := []string{
			"gross_sales", "vatable_sales", "total_sales", "gross_purchase",
			"gross_amount", "amount", "landed_cost", "tax_base",
			"gross_sales_receipts", "net_sales", "gross_income", "total_gross_income",
			"expense_amount", "gross_compensation", "basic_salary",
			"taxable_income", "taxable_compensation",
			"purchase_domestic_goods", "purchase_importation", "purchase_domestic_services",
		}
		amt := firstNonZeroAmount(row, amtKeys...)
		amountNum := pgtype.Numeric{}
		scanNumeric(&amountNum, amt)

		// Extract VAT/tax amount: output_tax for sales, input_tax for purchases, tax_withheld for EWT/ITR
		vat := firstNonZeroAmount(row,
			"output_tax", "input_tax", "vat_amount", "tax_withheld",
			"income_tax_due", "total_tax_withheld",
			"creditable_withholding_tax", "tax_payable",
			"input_tax_capital_goods", "input_tax_domestic_goods",
			"input_tax_importation", "input_tax_domestic_services",
			"vat_paid_imports",
		)
		vatNum := pgtype.Numeric{}
		scanNumeric(&vatNum, vat)
		confNum := pgtype.Numeric{}
		_ = confNum.Scan("0")

		// Extract date: BIR fields first, then generic
		var txDate pgtype.Date
		var rawDateStr string
		for _, key := range []string{
			"sales_date", "purchase_date", "invoice_date", "income_date",
			"expense_date", "revenue_date", "date_hired", "date",
			"taxable_month", "taxable_year", "importation_date", "assessment_date",
		} {
			if d := toString(row[key]); d != "" {
				txDate = parseFlexDate(d)
				if txDate.Valid {
					rawDateStr = ""
					break
				}
				if rawDateStr == "" {
					rawDateStr = d // remember first non-empty date string that failed to parse
				}
			}
		}
		// If we found a date string but couldn't parse it, record a warning
		if !txDate.Valid && rawDateStr != "" {
			descStr := ""
			if d := firstNonEmptyStr(row, "description", "supplier_name", "customer_name", "registered_name"); d != nil {
				descStr = *d
			}
			warnings = append(warnings, map[string]interface{}{
				"row":         i + 1,
				"description": descStr,
				"raw_date":    rawDateStr,
				"message":     fmt.Sprintf("Row %d: invalid date \"%s\" — please correct this date value", i+1, rawDateStr),
			})
		}

		// Extract description: try multiple name fields
		desc := firstNonEmptyStr(row,
			"description", "expense_description", "expense_category",
			"supplier_name", "customer_name", "income_source", "revenue_source",
			"registered_name", "employee_name", "employer_name",
		)

		// Extract TIN: try specific then generic
		tin := firstNonEmptyStr(row,
			"supplier_tin", "customer_tin", "employee_tin", "employer_tin", "tin",
		)

		// invoice_number is preserved in rawData JSON for report generation
		rawData, _ := json.Marshal(row)

		vatType := toString(row["vat_type"])
		if vatType == "" {
			vatType = "vatable"
		}
		category := toString(row["category"])
		if category == "" {
			category = "goods"
		}

		// Extract EWT and ATC fields if mapped
		ewtRateNum := pgtype.Numeric{}
		if v := parseAmount(row["ewt_rate"]); v != 0 {
			scanNumeric(&ewtRateNum, v)
		}
		ewtAmountNum := pgtype.Numeric{}
		if v := parseAmount(row["ewt_amount"]); v != 0 {
			scanNumeric(&ewtAmountNum, v)
		}
		atcCode := toStringPtr(row["atc_code"])

		_, err := s.q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
			ID:                   uuid.New(),
			CompanyID:            companyID,
			SessionID:            sessionID,
			SourceType:           sourceType,
			SourceFileID:         input.FileID,
			RowIndex:             int32(i),
			Date:                 txDate,
			Description:          desc,
			Amount:               amountNum,
			VatAmount:            vatNum,
			VatType:              vatType,
			Category:             category,
			Tin:                  tin,
			Confidence:           confNum,
			ClassificationSource: "ai",
			RawData:              rawData,
			MatchStatus:          "unmatched",
			EwtRate:              ewtRateNum,
			EwtAmount:            ewtAmountNum,
			AtcCode:              atcCode,
		})
		if err != nil {
			slog.Warn("failed to create transaction", "error", err, "row", i)
			continue
		}
		imported++
	}

	// Update source_files on session
	var existingFiles []interface{}
	if len(session.SourceFiles) > 0 {
		_ = json.Unmarshal(session.SourceFiles, &existingFiles)
	}
	fileInfo := map[string]interface{}{
		"file_id":    input.FileID,
		"filename":   input.Filename,
		"file_type":  input.SourceType,
		"sheet_name": input.SheetName,
		"row_count":  imported,
	}
	existingFiles = append(existingFiles, fileInfo)
	newFilesJSON, _ := json.Marshal(existingFiles)

	_ = s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               session.Status,
		ReportID:             session.ReportID,
		SourceFiles:          newFilesJSON,
		Summary:              session.Summary,
		ReconciliationResult: session.ReconciliationResult,
		CompletedAt:          session.CompletedAt,
	})

	result := map[string]interface{}{
		"file":                  fileInfo,
		"transactions_imported": imported,
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	return result, nil
}

// ClassifySession runs AI classification on session transactions.
func (s *SessionService) ClassifySession(ctx context.Context, sessionID, companyID uuid.UUID, force bool) (map[string]interface{}, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	allTxns, err := s.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil || len(allTxns) == 0 {
		return nil, fmt.Errorf("no transactions to classify")
	}

	// Filter to unclassified unless force
	var toClassify []sqlc.Transaction
	if force {
		toClassify = allTxns
	} else {
		for _, t := range allTxns {
			if t.ClassificationSource == "ai" {
				if f, err := t.Confidence.Float64Value(); err == nil && f.Float64 == 0 {
					toClassify = append(toClassify, t)
				}
			}
		}
		if len(toClassify) == 0 {
			return map[string]interface{}{"message": "All transactions already classified", "count": 0}, nil
		}
	}

	// Build input dicts
	txnDicts := make([]map[string]interface{}, len(toClassify))
	for i, t := range toClassify {
		m := map[string]interface{}{
			"amount": 0.0,
			"tin":    "",
		}
		if t.Description != nil {
			m["description"] = *t.Description
		}
		if t.Date.Valid {
			m["date"] = t.Date.Time.Format("2006-01-02")
		}
		if f, err := t.Amount.Float64Value(); err == nil {
			m["amount"] = f.Float64
		}
		if t.Tin != nil {
			m["tin"] = *t.Tin
		}
		txnDicts[i] = m
	}

	// Update status to classifying
	_ = s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               "classifying",
		ReportID:             session.ReportID,
		SourceFiles:          session.SourceFiles,
		Summary:              session.Summary,
		ReconciliationResult: session.ReconciliationResult,
		CompletedAt:          session.CompletedAt,
	})

	// Run classification
	results, err := s.classifier.ClassifyTransactions(ctx, txnDicts, companyID, "")
	if err != nil {
		return nil, fmt.Errorf("classification failed: %w", err)
	}

	classifiedCount := 0
	for i, t := range toClassify {
		if i >= len(results) {
			break
		}
		r := results[i]
		confNum := pgtype.Numeric{}
		_ = confNum.Scan(fmt.Sprintf("%.2f", r.Confidence))

		_ = s.q.BulkUpdateTransactionClassification(ctx, sqlc.BulkUpdateTransactionClassificationParams{
			ID:                   t.ID,
			VatType:              r.VATType,
			Category:             r.Category,
			Confidence:           confNum,
			ClassificationSource: r.ClassificationSource,
		})
		classifiedCount++
	}

	// Update status to reviewing
	_ = s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               "reviewing",
		ReportID:             session.ReportID,
		SourceFiles:          session.SourceFiles,
		Summary:              session.Summary,
		ReconciliationResult: session.ReconciliationResult,
		CompletedAt:          session.CompletedAt,
	})

	return map[string]interface{}{
		"classified": classifiedCount,
		"total":      len(allTxns),
	}, nil
}

// ListTransactions lists transactions for a session with filters.
func (s *SessionService) ListTransactions(ctx context.Context, sessionID, companyID uuid.UUID, limit, offset int, filters map[string]string) ([]TransactionResponse, int64, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, 0, fmt.Errorf("session not found")
	}

	vatType := filters["vat_type"]
	category := filters["category"]
	sourceType := filters["source_type"]
	matchStatus := filters["match_status"]
	search := filters["search"]

	var minConf pgtype.Numeric
	if mc := filters["min_confidence"]; mc != "" {
		_ = minConf.Scan(mc)
	}

	txns, err := s.q.ListTransactionsBySessionFiltered(ctx, sqlc.ListTransactionsBySessionFilteredParams{
		SessionID: sessionID,
		Limit:     int32(limit),
		Offset:    int32(offset),
		Column4:   vatType,
		Column5:   category,
		Column6:   sourceType,
		Column7:   matchStatus,
		Column8:   minConf,
		Column9:   search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list transactions: %w", err)
	}

	total, err := s.q.CountTransactionsBySessionFiltered(ctx, sqlc.CountTransactionsBySessionFilteredParams{
		SessionID: sessionID,
		Column2:   vatType,
		Column3:   category,
		Column4:   sourceType,
		Column5:   matchStatus,
		Column6:   minConf,
		Column7:   search,
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]TransactionResponse, len(txns))
	for i, t := range txns {
		result[i] = txnToResponse(t)
	}
	return result, total, nil
}

// UpdateTransaction updates a transaction's classification.
func (s *SessionService) UpdateTransaction(ctx context.Context, txnID, sessionID, companyID uuid.UUID, updates map[string]interface{}) (*TransactionResponse, error) {
	txn, err := s.q.GetTransactionByID(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found")
	}
	if txn.CompanyID != companyID || txn.SessionID != sessionID {
		return nil, fmt.Errorf("transaction not found")
	}

	vatType := txn.VatType
	if v, ok := updates["vat_type"].(string); ok && v != "" {
		vatType = v
	}
	category := txn.Category
	if v, ok := updates["category"].(string); ok && v != "" {
		category = v
	}

	confNum := pgtype.Numeric{}
	_ = confNum.Scan("1.00")

	_ = s.q.BulkUpdateTransactionClassification(ctx, sqlc.BulkUpdateTransactionClassificationParams{
		ID:                   txnID,
		VatType:              vatType,
		Category:             category,
		Confidence:           confNum,
		ClassificationSource: "user_override",
	})

	// Re-fetch
	txn, err = s.q.GetTransactionByID(ctx, txnID)
	if err != nil {
		return nil, err
	}
	resp := txnToResponse(txn)
	return &resp, nil
}

// DetectAnomalies runs anomaly detection on a session.
func (s *SessionService) DetectAnomalies(ctx context.Context, sessionID, companyID, userID uuid.UUID) (map[string]interface{}, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	allTxns, err := s.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil || len(allTxns) == 0 {
		return nil, fmt.Errorf("no transactions to analyze")
	}

	txnDicts := make([]map[string]interface{}, len(allTxns))
	for i, t := range allTxns {
		txnDicts[i] = txnToMap(t)
	}

	detected := RunAnomalyDetection(txnDicts)

	// Clear previous anomalies
	_ = s.q.DeleteAnomaliesBySession(ctx, sessionID)

	// Insert new anomalies
	for _, a := range detected {
		txnIDPg := pgtype.UUID{}
		if a.TransactionID != nil {
			txnIDPg = pgtype.UUID{Bytes: *a.TransactionID, Valid: true}
		}
		detailsJSON, _ := json.Marshal(a.Details)

		_, _ = s.q.CreateAnomaly(ctx, sqlc.CreateAnomalyParams{
			ID:            uuid.New(),
			CompanyID:     companyID,
			SessionID:     sessionID,
			TransactionID: txnIDPg,
			AnomalyType:   a.AnomalyType,
			Severity:      a.Severity,
			Description:   a.Description,
			Details:       detailsJSON,
			Status:        "open",
		})
	}

	return map[string]interface{}{"anomalies_found": len(detected)}, nil
}

// ListAnomalies lists anomalies for a session.
func (s *SessionService) ListAnomalies(ctx context.Context, sessionID, companyID uuid.UUID, limit, offset int, statusFilter *string) ([]AnomalyResponse, int64, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, 0, fmt.Errorf("session not found")
	}

	sf := ""
	if statusFilter != nil {
		sf = *statusFilter
	}

	anomalies, err := s.q.ListAnomaliesBySessionFiltered(ctx, sqlc.ListAnomaliesBySessionFilteredParams{
		SessionID: sessionID,
		Limit:     int32(limit),
		Offset:    int32(offset),
		Column4:   sf,
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.q.CountAnomaliesBySessionFiltered(ctx, sqlc.CountAnomaliesBySessionFilteredParams{
		SessionID: sessionID,
		Column2:   sf,
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]AnomalyResponse, len(anomalies))
	for i, a := range anomalies {
		result[i] = anomalyToResponse(a)
	}
	return result, total, nil
}

// ResolveAnomaly resolves an anomaly.
func (s *SessionService) ResolveAnomaly(ctx context.Context, anomalyID, sessionID, companyID, userID uuid.UUID, status string, note *string) (*AnomalyResponse, error) {
	anomaly, err := s.q.GetAnomalyByID(ctx, anomalyID)
	if err != nil {
		return nil, fmt.Errorf("anomaly not found")
	}
	if anomaly.CompanyID != companyID || anomaly.SessionID != sessionID {
		return nil, fmt.Errorf("anomaly not found")
	}

	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	_ = s.q.UpdateAnomaly(ctx, sqlc.UpdateAnomalyParams{
		ID:             anomalyID,
		Status:         status,
		ResolvedBy:     pgtype.UUID{Bytes: userID, Valid: true},
		ResolvedAt:     now,
		ResolutionNote: note,
	})

	anomaly, _ = s.q.GetAnomalyByID(ctx, anomalyID)
	resp := anomalyToResponse(anomaly)
	return &resp, nil
}

// GetVATSummary generates a VAT summary for a session.
func (s *SessionService) GetVATSummary(ctx context.Context, sessionID, companyID uuid.UUID) (*VATSummary, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	allTxns, err := s.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	txnMaps := make([]map[string]interface{}, len(allTxns))
	for i, t := range allTxns {
		txnMaps[i] = txnToMap(t)
	}

	summary := GenerateVATSummary(txnMaps, session.Period)

	// Cache summary on session
	summaryJSON, _ := json.Marshal(summary)
	_ = s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               session.Status,
		ReportID:             session.ReportID,
		SourceFiles:          session.SourceFiles,
		Summary:              summaryJSON,
		ReconciliationResult: session.ReconciliationResult,
		CompletedAt:          session.CompletedAt,
	})

	return &summary, nil
}

// ReconcileSession runs the full reconciliation pipeline.
func (s *SessionService) ReconcileSession(ctx context.Context, sessionID, companyID uuid.UUID, reportID *uuid.UUID, amountTolerance float64, dateToleranceDays int) (map[string]interface{}, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	allTxns, err := s.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil || len(allTxns) == 0 {
		return nil, fmt.Errorf("no transactions to reconcile")
	}

	var sales, purchases, bank []map[string]interface{}
	for _, t := range allTxns {
		m := txnToMap(t)
		switch t.SourceType {
		case "sales_record":
			sales = append(sales, m)
		case "purchase_record":
			purchases = append(purchases, m)
		case "bank_statement":
			bank = append(bank, m)
		}
	}

	txnMaps := make([]map[string]interface{}, len(allTxns))
	for i, t := range allTxns {
		txnMaps[i] = txnToMap(t)
	}

	// Generate summary
	summary := GenerateVATSummary(txnMaps, session.Period)

	result := map[string]interface{}{
		"summary": summary,
	}

	// Match if we have bank entries — using upgraded multi-signal scoring engine
	if len(bank) > 0 {
		records := append(sales, purchases...)
		matchResult := MatchTransactions(records, bank, amountTolerance, dateToleranceDays)

		result["match_stats"] = map[string]interface{}{
			"pairs":             len(matchResult.MatchedPairs),
			"unmatched_records": len(matchResult.UnmatchedRecords),
			"unmatched_bank":    len(matchResult.UnmatchedBank),
			"match_rate":        matchResult.MatchRate,
		}

		// Unified bank reconciliation view — all data in one place
		result["bank_reconciliation"] = map[string]interface{}{
			"matched_pairs":     matchResult.MatchedPairs,
			"unmatched_records": matchResult.UnmatchedRecords,
			"unmatched_bank":    matchResult.UnmatchedBank,
			"split_matches":     matchResult.SplitMatches,
			"match_rate":        matchResult.MatchRate,
		}

		// Update match statuses for 1:1 matches
		for _, pair := range matchResult.MatchedPairs {
			mgID := pgtype.UUID{Bytes: pair.MatchGroupID, Valid: true}
			if recID, err := uuid.Parse(pair.RecordID); err == nil {
				_ = s.q.UpdateTransactionMatch(ctx, sqlc.UpdateTransactionMatchParams{
					ID:           recID,
					MatchGroupID: mgID,
					MatchStatus:  "matched",
				})
			}
			if bankID, err := uuid.Parse(pair.BankID); err == nil {
				_ = s.q.UpdateTransactionMatch(ctx, sqlc.UpdateTransactionMatchParams{
					ID:           bankID,
					MatchGroupID: mgID,
					MatchStatus:  "matched",
				})
			}
		}

		// Update match statuses for split matches
		for _, split := range matchResult.SplitMatches {
			mgID := pgtype.UUID{Bytes: split.MatchGroupID, Valid: true}
			for _, recIDStr := range split.RecordIDs {
				if recID, err := uuid.Parse(recIDStr); err == nil {
					_ = s.q.UpdateTransactionMatch(ctx, sqlc.UpdateTransactionMatchParams{
						ID:           recID,
						MatchGroupID: mgID,
						MatchStatus:  "split_matched",
					})
				}
			}
			if bankID, err := uuid.Parse(split.BankID); err == nil {
				_ = s.q.UpdateTransactionMatch(ctx, sqlc.UpdateTransactionMatchParams{
					ID:           bankID,
					MatchGroupID: mgID,
					MatchStatus:  "split_matched",
				})
			}
		}
	}

	// Compare with declared report
	effectiveReportID := reportID
	if effectiveReportID == nil && session.ReportID.Valid {
		id := uuid.UUID(session.ReportID.Bytes)
		effectiveReportID = &id
	}
	if effectiveReportID != nil {
		report, err := s.q.GetReportByID(ctx, *effectiveReportID)
		if err == nil && report.CompanyID == companyID && len(report.CalculatedData) > 0 {
			var declared map[string]string
			_ = json.Unmarshal(report.CalculatedData, &declared)
			if declared != nil {
				comparison := CompareWithDeclared(summary, declared)
				result["comparison"] = comparison
			}
		}
	}

	// Count anomalies
	anomalyCount, _ := s.q.CountAnomaliesBySession(ctx, sessionID)
	result["anomaly_count"] = anomalyCount

	// Cache result
	resultJSON, _ := json.Marshal(result)
	summaryJSON, _ := json.Marshal(summary)
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	_ = s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               "completed",
		ReportID:             session.ReportID,
		SourceFiles:          session.SourceFiles,
		Summary:              summaryJSON,
		ReconciliationResult: resultJSON,
		CompletedAt:          now,
	})

	return result, nil
}

// GetCompanyInfo retrieves company info for PDF generation.
func (s *SessionService) GetCompanyInfo(ctx context.Context, companyID uuid.UUID) (CompanyInfo, error) {
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return CompanyInfo{}, fmt.Errorf("company not found: %w", err)
	}
	info := CompanyInfo{CompanyName: company.CompanyName}
	if company.TinNumber != nil {
		info.TINNumber = *company.TinNumber
	}
	if company.RdoCode != nil {
		info.RDOCode = *company.RdoCode
	}
	return info, nil
}

// ExportTransactionsCSV exports session transactions as CSV rows.
func (s *SessionService) ExportTransactionsCSV(ctx context.Context, sessionID, companyID uuid.UUID) ([][]string, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	txns, err := s.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	rows := [][]string{
		{"Date", "Description", "Amount", "VAT Amount", "VAT Type", "Category", "TIN", "Confidence", "Source", "Match Status"},
	}
	for _, t := range txns {
		resp := txnToResponse(t)
		date := ""
		if resp.Date != nil {
			date = *resp.Date
		}
		desc := ""
		if resp.Description != nil {
			desc = *resp.Description
		}
		tin := ""
		if resp.TIN != nil {
			tin = *resp.TIN
		}
		rows = append(rows, []string{
			date, desc,
			fmt.Sprintf("%.2f", resp.Amount),
			fmt.Sprintf("%.2f", resp.VATAmount),
			resp.VATType, resp.Category, tin,
			fmt.Sprintf("%.2f", resp.Confidence),
			resp.ClassificationSource,
			resp.MatchStatus,
		})
	}
	return rows, nil
}

// helper to convert sqlc Transaction to map for engine functions
func txnToMap(t sqlc.Transaction) map[string]interface{} {
	m := map[string]interface{}{
		"id":            t.ID.String(),
		"source_type":   t.SourceType,
		"vat_type":      t.VatType,
		"category":      t.Category,
		"match_status":  t.MatchStatus,
	}
	if t.Description != nil {
		m["description"] = *t.Description
	}
	if t.Date.Valid {
		m["date"] = t.Date.Time.Format("2006-01-02")
	}
	if f, err := t.Amount.Float64Value(); err == nil {
		m["amount"] = f.Float64
	}
	if f, err := t.VatAmount.Float64Value(); err == nil {
		m["vat_amount"] = f.Float64
	}
	if f, err := t.Confidence.Float64Value(); err == nil {
		m["confidence"] = f.Float64
	}
	if t.Tin != nil {
		m["tin"] = *t.Tin
	}
	return m
}

// parseFlexDate tries multiple date formats common in Philippine business documents.
// It also handles invalid day-of-month values (e.g. 11/31/2024 → 11/30/2024) by
// clamping to the last day of the month.
func parseFlexDate(s string) pgtype.Date {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Date{}
	}
	formats := []string{
		"2006-01-02",   // ISO
		"01-02-06",     // MM-DD-YY
		"01/02/06",     // MM/DD/YY
		"01-02-2006",   // MM-DD-YYYY
		"01/02/2006",   // MM/DD/YYYY
		"1/2/2006",     // M/D/YYYY
		"1/2/06",       // M/D/YY
		"Jan 2, 2006",  // Mon D, YYYY
		"2 Jan 2006",   // D Mon YYYY
		"2006/01/02",   // YYYY/MM/DD
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			// Fix 2-digit year: 00-49 → 2000s, 50-99 → 1900s (Go default)
			if t.Year() < 100 {
				t = t.AddDate(2000, 0, 0)
			}
			return pgtype.Date{Time: t, Valid: true}
		}
	}
	// Try parsing just the first 10 chars for ISO format with trailing time
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return pgtype.Date{Time: t, Valid: true}
		}
	}
	return pgtype.Date{}
}

// firstNonZeroAmount returns the first non-zero amount from the given row keys.
func firstNonZeroAmount(row map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v := parseAmount(row[k]); v != 0 {
			return v
		}
	}
	return 0
}

// firstNonEmptyStr returns the first non-empty string pointer from the given row keys.
func firstNonEmptyStr(row map[string]interface{}, keys ...string) *string {
	for _, k := range keys {
		if p := toStringPtr(row[k]); p != nil {
			return p
		}
	}
	return nil
}

func toStringPtr(v interface{}) *string {
	if v == nil {
		return nil
	}
	s := fmt.Sprintf("%v", v)
	if s == "" || s == "<nil>" {
		return nil
	}
	return &s
}

// normalizeRowKeys lowercases keys and replaces spaces/hyphens with underscores.
// This ensures raw column headers like "TAXABLE MONTH" or "Gross Purchase"
// match expected field names like "taxable_month" or "gross_purchase".
func normalizeRowKeys(row map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(row))
	for k, v := range row {
		nk := strings.ToLower(strings.TrimSpace(k))
		nk = strings.ReplaceAll(nk, " ", "_")
		nk = strings.ReplaceAll(nk, "-", "_")
		// Keep the first occurrence if there are duplicates after normalization.
		if _, exists := out[nk]; !exists {
			out[nk] = v
		}
	}
	return out
}

// scanNumeric safely scans a float64 into pgtype.Numeric.
// Uses strconv.FormatFloat to avoid scientific notation (e.g. 1.43e+07)
// which pgtype.Numeric.Scan cannot parse.
func scanNumeric(n *pgtype.Numeric, f float64) {
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))
}

