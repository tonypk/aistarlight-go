package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrReceiptTooLarge        = errors.New("file exceeds 10MB limit")
	ErrReceiptTotalTooLarge   = errors.New("total receipt size exceeds 100MB limit")
	ErrReceiptUnsupportedType = errors.New("unsupported file type: only JPEG, PNG, PDF allowed")
)

var magicBytes = map[string][]byte{
	".jpg": {0xFF, 0xD8, 0xFF},
	".png": {0x89, 0x50, 0x4E, 0x47},
	".pdf": {0x25, 0x50, 0x44, 0x46},
}

const (
	maxFileSize  = 10 * 1024 * 1024  // 10MB per file
	maxTotalSize = 100 * 1024 * 1024 // 100MB per report
)

type ExpenseReceiptService struct {
	q         *sqlc.Queries
	uploadDir string
}

func NewExpenseReceiptService(q *sqlc.Queries, uploadDir string) *ExpenseReceiptService {
	return &ExpenseReceiptService{q: q, uploadDir: uploadDir}
}

// ValidateReceiptFile checks magic bytes and file size.
// Returns detected extension or error.
func (s *ExpenseReceiptService) ValidateReceiptFile(data []byte) (string, error) {
	if len(data) > maxFileSize {
		return "", ErrReceiptTooLarge
	}
	for ext, magic := range magicBytes {
		if len(data) >= len(magic) && bytes.Equal(data[:len(magic)], magic) {
			return ext, nil
		}
	}
	return "", ErrReceiptUnsupportedType
}

// SaveReceipt saves receipt file to disk and updates the item's receipt_url.
func (s *ExpenseReceiptService) SaveReceipt(ctx context.Context, itemID, companyID uuid.UUID, data []byte, ext string) (string, error) {
	// Build path: {uploadDir}/receipts/{companyID}/{YYYY-MM}/{itemID}.{ext}
	now := time.Now()
	relDir := filepath.Join("receipts", companyID.String(), now.Format("2006-01"))
	absDir := filepath.Join(s.uploadDir, relDir)

	if err := os.MkdirAll(absDir, 0755); err != nil {
		return "", fmt.Errorf("create receipt dir: %w", err)
	}

	filename := itemID.String() + ext
	absPath := filepath.Join(absDir, filename)
	relPath := filepath.Join(relDir, filename)

	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return "", fmt.Errorf("write receipt file: %w", err)
	}

	// Update item's receipt_url
	if err := s.q.UpdateExpenseItemReceipt(ctx, sqlc.UpdateExpenseItemReceiptParams{
		ID:                   itemID,
		ReceiptUrl:           &relPath,
		ReceiptOcrData:       nil,
		AiCategoryConfidence: pgtype.Numeric{},
	}); err != nil {
		return "", fmt.Errorf("update item receipt: %w", err)
	}

	return relPath, nil
}

// GetReceiptPath returns the absolute file path for a receipt.
func (s *ExpenseReceiptService) GetReceiptPath(ctx context.Context, itemID uuid.UUID) (string, error) {
	item, err := s.q.GetExpenseItemByID(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("item not found: %w", err)
	}
	if item.ReceiptUrl == nil || *item.ReceiptUrl == "" {
		return "", errors.New("no receipt attached")
	}
	return filepath.Join(s.uploadDir, *item.ReceiptUrl), nil
}
