package domain

import (
	"time"

	"github.com/google/uuid"
)

type ReceiptBatch struct {
	ID             uuid.UUID  `json:"id"`
	CompanyID      uuid.UUID  `json:"company_id"`
	UserID         uuid.UUID  `json:"user_id"`
	Status         string     `json:"status"`
	TotalImages    int        `json:"total_images"`
	ProcessedCount int        `json:"processed_count"`
	SessionID      *uuid.UUID `json:"session_id,omitempty"`
	ReportID       *uuid.UUID `json:"report_id,omitempty"`
	ReportType     string     `json:"report_type"`
	Period         string     `json:"period"`
	Results        JSON       `json:"results,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
