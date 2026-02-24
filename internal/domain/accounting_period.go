package domain

import (
	"time"

	"github.com/google/uuid"
)

type PeriodType string

const (
	PeriodMonthly   PeriodType = "monthly"
	PeriodQuarterly PeriodType = "quarterly"
	PeriodAnnual    PeriodType = "annual"
)

type PeriodStatus string

const (
	PeriodOpen   PeriodStatus = "open"
	PeriodClosed PeriodStatus = "closed"
	PeriodLocked PeriodStatus = "locked"
)

type AccountingPeriod struct {
	ID         uuid.UUID    `json:"id"`
	CompanyID  uuid.UUID    `json:"company_id"`
	Name       string       `json:"name"`
	PeriodType PeriodType   `json:"period_type"`
	StartDate  time.Time    `json:"start_date"`
	EndDate    time.Time    `json:"end_date"`
	Status     PeriodStatus `json:"status"`
	ClosedBy   *uuid.UUID   `json:"closed_by,omitempty"`
	ClosedAt   *time.Time   `json:"closed_at,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}
