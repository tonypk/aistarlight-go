package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ReceiptBridge converts OCR'd receipts into transaction records.
type ReceiptBridge struct {
	q          *sqlc.Queries
	classifier *ClassifierService
}

// NewReceiptBridge creates a ReceiptBridge.
func NewReceiptBridge(q *sqlc.Queries, classifier *ClassifierService) *ReceiptBridge {
	return &ReceiptBridge{q: q, classifier: classifier}
}

// ConvertReceiptToTransactions converts a completed receipt batch into transaction records.
// projectTag is optional and will be stored on each created transaction.
func (b *ReceiptBridge) ConvertReceiptToTransactions(ctx context.Context, companyID uuid.UUID, receiptID uuid.UUID, sessionID uuid.UUID, projectTag *string) ([]domain.Transaction, error) {
	batch, err := b.q.GetReceiptBatchByID(ctx, receiptID)
	if err != nil {
		return nil, fmt.Errorf("receipt batch not found: %w", err)
	}

	if batch.CompanyID != companyID {
		return nil, fmt.Errorf("receipt batch does not belong to company")
	}
	if batch.Status != "completed" {
		return nil, fmt.Errorf("receipt batch is not completed (status: %s)", batch.Status)
	}

	// Parse results JSON
	var results []ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil {
		return nil, fmt.Errorf("parse receipt results: %w", err)
	}

	var transactions []domain.Transaction
	var txnIDs []uuid.UUID

	for i, result := range results {
		if result.Error != "" {
			continue // skip failed OCR results
		}

		txn, err := b.createTransactionFromReceipt(ctx, companyID, sessionID, batch.ID, i, result, projectTag)
		if err != nil {
			return nil, fmt.Errorf("create transaction from receipt %d: %w", i, err)
		}

		transactions = append(transactions, *txn)
		txnIDs = append(txnIDs, txn.ID)
	}

	// Link receipt batch to created transactions
	if len(txnIDs) > 0 {
		if err := b.q.LinkReceiptToTransactions(ctx, sqlc.LinkReceiptToTransactionsParams{
			ID:             receiptID,
			TransactionIds: txnIDs,
		}); err != nil {
			return nil, fmt.Errorf("link receipt to transactions: %w", err)
		}
	}

	return transactions, nil
}

func (b *ReceiptBridge) createTransactionFromReceipt(ctx context.Context, companyID, sessionID, batchID uuid.UUID, rowIndex int, result ReceiptResult, projectTag *string) (*domain.Transaction, error) {
	parsed := result.Parsed

	// Extract amount
	amount := decimal.Zero
	if parsed.TotalAmount.Value != nil {
		switch v := parsed.TotalAmount.Value.(type) {
		case float64:
			amount = decimal.NewFromFloat(v)
		case string:
			amount, _ = decimal.NewFromString(v)
		}
	}

	// Extract VAT amount
	vatAmount := decimal.Zero
	if parsed.VATAmount.Value != nil {
		switch v := parsed.VATAmount.Value.(type) {
		case float64:
			vatAmount = decimal.NewFromFloat(v)
		case string:
			vatAmount, _ = decimal.NewFromString(v)
		}
	}

	// Extract VAT type
	vatType := "vatable"
	if parsed.VATType.Value != nil {
		if v, ok := parsed.VATType.Value.(string); ok && v != "" {
			vatType = v
		}
	}

	// Extract category
	category := "goods"
	if parsed.Category.Value != nil {
		if v, ok := parsed.Category.Value.(string); ok && v != "" {
			category = v
		}
	}

	// Extract date
	var txnDate pgtype.Date
	if parsed.Date.Value != nil {
		if dateStr, ok := parsed.Date.Value.(string); ok {
			for _, layout := range []string{"2006-01-02", "01/02/2006", "1/2/2006"} {
				if t, err := time.Parse(layout, dateStr); err == nil {
					txnDate = pgtype.Date{Time: t, Valid: true}
					break
				}
			}
		}
	}
	if !txnDate.Valid {
		txnDate = pgtype.Date{Time: time.Now(), Valid: true}
	}

	// Build description
	desc := "Receipt"
	if parsed.VendorName.Value != nil {
		if v, ok := parsed.VendorName.Value.(string); ok && v != "" {
			desc = v
		}
	}

	// Extract TIN
	var tin *string
	if parsed.TIN.Value != nil {
		if v, ok := parsed.TIN.Value.(string); ok && v != "" {
			tin = &v
		}
	}

	// Store raw data
	rawData, _ := json.Marshal(result)

	amountNum := pgtype.Numeric{}
	_ = amountNum.Scan(amount.String())
	vatAmountNum := pgtype.Numeric{}
	_ = vatAmountNum.Scan(vatAmount.String())
	confidence := pgtype.Numeric{}
	_ = confidence.Scan(fmt.Sprintf("%.2f", result.OverallConfidence))

	txnID := uuid.New()
	dbTxn, err := b.q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		ID:                   txnID,
		CompanyID:            companyID,
		SessionID:            sessionID,
		SourceType:           "receipt",
		SourceFileID:         batchID.String(),
		RowIndex:             int32(rowIndex),
		Date:                 txnDate,
		Description:          &desc,
		Amount:               amountNum,
		VatAmount:            vatAmountNum,
		VatType:              vatType,
		Category:             category,
		Tin:                  tin,
		Confidence:           confidence,
		ClassificationSource: "ocr",
		RawData:              rawData,
		MatchStatus:          "unmatched",
		ProjectTag:           projectTag,
	})
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}

	txn := &domain.Transaction{
		ID:                   dbTxn.ID,
		CompanyID:            dbTxn.CompanyID,
		SessionID:            dbTxn.SessionID,
		SourceType:           dbTxn.SourceType,
		SourceFileID:         dbTxn.SourceFileID,
		RowIndex:             int(dbTxn.RowIndex),
		Amount:               amount,
		VATAmount:            vatAmount,
		VATType:              dbTxn.VatType,
		Category:             dbTxn.Category,
		TIN:                  dbTxn.Tin,
		Confidence:           decimal.NewFromFloat(result.OverallConfidence),
		ClassificationSource: dbTxn.ClassificationSource,
		MatchStatus:          dbTxn.MatchStatus,
		Description:          &desc,
		CreatedAt:            dbTxn.CreatedAt,
		UpdatedAt:            dbTxn.UpdatedAt,
	}
	if txnDate.Valid {
		t := txnDate.Time
		txn.Date = &t
	}

	return txn, nil
}
