package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrBatchNotFound = errors.New("bank reconciliation batch not found")
)

// BankReconService orchestrates the bank reconciliation pipeline.
type BankReconService struct {
	q        *sqlc.Queries
	analyzer *MatchAnalyzer
}

// NewBankReconService creates a BankReconService.
func NewBankReconService(q *sqlc.Queries, analyzer *MatchAnalyzer) *BankReconService {
	return &BankReconService{q: q, analyzer: analyzer}
}

// CreateBatchInput holds parameters for creating a new reconciliation batch.
type CreateBatchInput struct {
	CompanyID         uuid.UUID
	CreatedBy         uuid.UUID
	SessionID         *uuid.UUID
	Period            string
	AmountTolerance   float64
	DateToleranceDays int
	SourceFiles       []string
	Records           []map[string]interface{}
	BankColumns       []string
	BankRows          []map[string]interface{}
}

// BatchResult holds the complete result of a reconciliation run.
type BatchResult struct {
	BatchID      uuid.UUID        `json:"batch_id"`
	Status       string           `json:"status"`
	ParseSummary *ParseSummary    `json:"parse_summary"`
	MatchResult  *MatchResult     `json:"match_result"`
	Suggestions  []AISuggestion   `json:"ai_suggestions,omitempty"`
	Explanations []AIExplanation  `json:"ai_explanations,omitempty"`
	Error        string           `json:"error,omitempty"`
}

// ParseSummary summarizes the bank statement parsing step.
type ParseSummary struct {
	BankName       string `json:"bank_name"`
	FormatDetected bool   `json:"format_detected"`
	TotalEntries   int    `json:"total_entries"`
	RecordCount    int    `json:"record_count"`
	BankEntryCount int    `json:"bank_entry_count"`
}

// RunReconciliation executes the full pipeline: parse → match → AI analyze → persist.
func (s *BankReconService) RunReconciliation(ctx context.Context, input CreateBatchInput) (*BatchResult, error) {
	batchID := uuid.New()

	sourceFilesJSON, _ := json.Marshal(input.SourceFiles)

	sessionID := pgtype.UUID{}
	if input.SessionID != nil {
		sessionID = pgtype.UUID{Bytes: *input.SessionID, Valid: true}
	}

	toleranceNumeric := pgtype.Numeric{}
	if input.AmountTolerance > 0 {
		_ = toleranceNumeric.Scan(fmt.Sprintf("%f", input.AmountTolerance))
	}

	batch, err := s.q.CreateBankReconBatch(ctx, sqlc.CreateBankReconBatchParams{
		ID:                batchID,
		CompanyID:         input.CompanyID,
		CreatedBy:         input.CreatedBy,
		SessionID:         sessionID,
		Status:            "pending",
		SourceFiles:       sourceFilesJSON,
		TotalEntries:      0,
		AmountTolerance:   toleranceNumeric,
		DateToleranceDays: int32(input.DateToleranceDays),
		Period:            input.Period,
	})
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}

	result := &BatchResult{BatchID: batch.ID, Status: "pending"}

	// Step 1: Parse bank statement
	result.Status = "parsing"
	s.updateStatus(ctx, batchID, "parsing")

	format := DetectBankFormat(input.BankColumns)
	bankEntries := ParseBankStatement(input.BankRows, format)

	parseSummary := &ParseSummary{
		BankName:       format.Name,
		FormatDetected: format.Name != "Generic",
		TotalEntries:   len(bankEntries) + len(input.Records),
		RecordCount:    len(input.Records),
		BankEntryCount: len(bankEntries),
	}
	result.ParseSummary = parseSummary

	parseSummaryJSON, _ := json.Marshal(parseSummary)
	_ = s.q.UpdateBankReconBatch(ctx, sqlc.UpdateBankReconBatchParams{
		ID:           batchID,
		Status:       "parsing",
		ParseSummary: parseSummaryJSON,
		TotalEntries: int32(parseSummary.TotalEntries),
	})

	// Step 2: Match transactions
	result.Status = "matching"
	s.updateStatus(ctx, batchID, "matching")

	bankMaps := bankEntriesToMaps(bankEntries)
	matchResult := MatchTransactions(input.Records, bankMaps, input.AmountTolerance, input.DateToleranceDays)
	result.MatchResult = &matchResult

	matchResultJSON, _ := json.Marshal(matchResult)
	_ = s.q.UpdateBankReconBatch(ctx, sqlc.UpdateBankReconBatchParams{
		ID:          batchID,
		Status:      "matching",
		MatchResult: matchResultJSON,
	})

	// Step 3: AI analysis of unmatched entries
	result.Status = "analyzing"
	s.updateStatus(ctx, batchID, "analyzing")

	suggestions, explanations, aiErr := s.analyzer.AnalyzeUnmatched(
		ctx,
		matchResult.UnmatchedRecords,
		matchResult.UnmatchedBank,
	)
	if aiErr != nil {
		slog.Warn("AI analysis failed, continuing without suggestions", "error", aiErr)
	}
	result.Suggestions = suggestions
	result.Explanations = explanations

	suggestionsJSON, _ := json.Marshal(suggestions)
	explanationsJSON, _ := json.Marshal(explanations)

	// Step 4: Persist final results
	result.Status = "completed"
	_ = s.q.UpdateBankReconBatch(ctx, sqlc.UpdateBankReconBatchParams{
		ID:             batchID,
		Status:         "completed",
		ParseSummary:   parseSummaryJSON,
		MatchResult:    matchResultJSON,
		AiSuggestions:  suggestionsJSON,
		AiExplanations: explanationsJSON,
		TotalEntries:   int32(parseSummary.TotalEntries),
	})

	slog.Info("reconciliation completed",
		"batch_id", batchID,
		"matched", len(matchResult.MatchedPairs),
		"unmatched_records", len(matchResult.UnmatchedRecords),
		"unmatched_bank", len(matchResult.UnmatchedBank),
		"match_rate", matchResult.MatchRate,
		"ai_suggestions", len(suggestions),
	)

	return result, nil
}

// GetBatch retrieves a reconciliation batch by ID.
func (s *BankReconService) GetBatch(ctx context.Context, id uuid.UUID) (*BatchResult, error) {
	batch, err := s.q.GetBankReconBatchByID(ctx, id)
	if err != nil {
		return nil, ErrBatchNotFound
	}

	result := &BatchResult{
		BatchID: batch.ID,
		Status:  batch.Status,
	}
	if batch.ErrorMessage != nil {
		result.Error = *batch.ErrorMessage
	}

	if len(batch.ParseSummary) > 0 {
		var ps ParseSummary
		_ = json.Unmarshal(batch.ParseSummary, &ps)
		result.ParseSummary = &ps
	}
	if len(batch.MatchResult) > 0 {
		var mr MatchResult
		_ = json.Unmarshal(batch.MatchResult, &mr)
		result.MatchResult = &mr
	}
	if len(batch.AiSuggestions) > 0 {
		_ = json.Unmarshal(batch.AiSuggestions, &result.Suggestions)
	}
	if len(batch.AiExplanations) > 0 {
		_ = json.Unmarshal(batch.AiExplanations, &result.Explanations)
	}

	return result, nil
}

// ListBatches lists reconciliation batches for a company.
func (s *BankReconService) ListBatches(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]BatchResult, int64, error) {
	batches, err := s.q.ListBankReconBatchesByCompany(ctx, sqlc.ListBankReconBatchesByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list batches: %w", err)
	}

	total, err := s.q.CountBankReconBatchesByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, fmt.Errorf("count batches: %w", err)
	}

	results := make([]BatchResult, len(batches))
	for i, b := range batches {
		results[i] = BatchResult{
			BatchID: b.ID,
			Status:  b.Status,
		}
		if b.ErrorMessage != nil {
			results[i].Error = *b.ErrorMessage
		}
		if len(b.MatchResult) > 0 {
			var mr MatchResult
			_ = json.Unmarshal(b.MatchResult, &mr)
			results[i].MatchResult = &mr
		}
	}

	return results, total, nil
}

// UpdateSessionWithResults links reconciliation results to a reconciliation session.
func (s *BankReconService) UpdateSessionWithResults(ctx context.Context, sessionID uuid.UUID, matchResult *MatchResult, summary *ParseSummary) error {
	summaryJSON, _ := json.Marshal(summary)
	resultJSON, _ := json.Marshal(matchResult)

	now := time.Now()
	completedAt := pgtype.Timestamptz{Time: now, Valid: true}

	return s.q.UpdateReconciliationSession(ctx, sqlc.UpdateReconciliationSessionParams{
		ID:                   sessionID,
		Status:               "completed",
		Summary:              summaryJSON,
		ReconciliationResult: resultJSON,
		CompletedAt:          completedAt,
	})
}

func (s *BankReconService) updateStatus(ctx context.Context, batchID uuid.UUID, status string) {
	_ = s.q.UpdateBankReconBatch(ctx, sqlc.UpdateBankReconBatchParams{
		ID:     batchID,
		Status: status,
	})
}

func bankEntriesToMaps(entries []ParsedBankEntry) []map[string]interface{} {
	result := make([]map[string]interface{}, len(entries))
	for i, e := range entries {
		result[i] = map[string]interface{}{
			"id":          e.ID,
			"date":        e.Date,
			"description": e.Description,
			"amount":      e.Amount,
			"type":        e.Type,
			"reference":   e.Reference,
		}
	}
	return result
}
