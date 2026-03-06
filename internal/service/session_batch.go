package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const maxBatchSize = 200

// BatchUpdateItem represents a single transaction update in a batch.
type BatchUpdateItem struct {
	ID       uuid.UUID `json:"id" binding:"required"`
	VATType  *string   `json:"vat_type,omitempty"`
	Category *string   `json:"category,omitempty"`
	TIN      *string   `json:"tin,omitempty"`
}

// BatchUpdateResult summarizes the outcome of a batch update.
type BatchUpdateResult struct {
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// BatchUpdateTransactions applies bulk updates to transactions within a session.
func (s *SessionService) BatchUpdateTransactions(ctx context.Context, sessionID, companyID uuid.UUID, items []BatchUpdateItem) (*BatchUpdateResult, error) {
	if len(items) == 0 {
		return &BatchUpdateResult{}, nil
	}
	if len(items) > maxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum (%d)", len(items), maxBatchSize)
	}

	// Verify session ownership
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	result := &BatchUpdateResult{}
	for _, item := range items {
		txn, err := s.q.GetTransactionByID(ctx, item.ID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: transaction not found", item.ID))
			continue
		}
		if txn.CompanyID != companyID || txn.SessionID != sessionID {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: not in this session", item.ID))
			continue
		}

		vatType := txn.VatType
		if item.VATType != nil && *item.VATType != "" {
			vatType = *item.VATType
		}
		category := txn.Category
		if item.Category != nil && *item.Category != "" {
			category = *item.Category
		}

		confNum := pgtype.Numeric{}
		_ = confNum.Scan("1.00")

		err = s.q.BulkUpdateTransactionClassification(ctx, sqlc.BulkUpdateTransactionClassificationParams{
			ID:                   item.ID,
			VatType:              vatType,
			Category:             category,
			Confidence:           confNum,
			ClassificationSource: "user_override",
		})
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: update failed", item.ID))
			continue
		}

		// Auto-create correction rules from user override
		desc := ""
		if txn.Description != nil {
			desc = *txn.Description
		}
		s.autoCreateCorrectionRules(ctx, companyID, desc, txn.VatType, vatType, txn.Category, category)

		result.Updated++
	}

	return result, nil
}
