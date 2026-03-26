package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// -- Expense report statuses --

type ExpenseReportStatus string

const (
	ExpenseStatusDraft           ExpenseReportStatus = "draft"
	ExpenseStatusSubmitted       ExpenseReportStatus = "submitted"
	ExpenseStatusPendingApproval ExpenseReportStatus = "pending_approval"
	ExpenseStatusApproved        ExpenseReportStatus = "approved"
	ExpenseStatusRejected        ExpenseReportStatus = "rejected"
	ExpenseStatusPaid            ExpenseReportStatus = "paid"
)

// -- AI decision types --

type ExpenseAIDecision string

const (
	AIDecisionAutoApproved ExpenseAIDecision = "auto_approved"
	AIDecisionNeedsReview  ExpenseAIDecision = "needs_review"
	AIDecisionHighRisk     ExpenseAIDecision = "high_risk"
)

// -- Expense categories --

const (
	CategoryMeals         = "meals"
	CategoryTransport     = "transport"
	CategoryOffice        = "office"
	CategoryTravel        = "travel"
	CategoryEntertainment = "entertainment"
	CategoryOther         = "other"
)

var ValidCategories = map[string]bool{
	CategoryMeals: true, CategoryTransport: true, CategoryOffice: true,
	CategoryTravel: true, CategoryEntertainment: true, CategoryOther: true,
}

// -- Audit log actions --

const (
	AuditActionCreated   = "created"
	AuditActionSubmitted = "submitted"
	AuditActionAIReview  = "ai_reviewed"
	AuditActionApproved  = "approved"
	AuditActionRejected  = "rejected"
	AuditActionPaid      = "paid"
	AuditActionEdited    = "edited"
)

// -- Domain structs --

type ExpensePolicy struct {
	ID                  uuid.UUID        `json:"id"`
	CompanyID           uuid.UUID        `json:"company_id"`
	Name                string           `json:"name"`
	Category            string           `json:"category"`
	MaxAmount           *decimal.Decimal `json:"max_amount,omitempty"`
	RequiresReceiptAbove *decimal.Decimal `json:"requires_receipt_above,omitempty"`
	AutoApproveBelow    *decimal.Decimal `json:"auto_approve_below,omitempty"`
	AIAutoApprove       bool             `json:"ai_auto_approve"`
	Description         *string          `json:"description,omitempty"`
	IsActive            bool             `json:"is_active"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type ExpenseReport struct {
	ID                    uuid.UUID           `json:"id"`
	CompanyID             uuid.UUID           `json:"company_id"`
	SubmitterUserID       uuid.UUID           `json:"submitter_user_id"`
	HRPayeeID             *uuid.UUID          `json:"hr_payee_id,omitempty"`
	ReportNumber          string              `json:"report_number"`
	Title                 string              `json:"title"`
	Status                ExpenseReportStatus `json:"status"`
	TotalAmount           decimal.Decimal     `json:"total_amount"`
	Currency              string              `json:"currency"`
	SubmittedAt           *time.Time          `json:"submitted_at,omitempty"`
	AIReviewedAt          *time.Time          `json:"ai_reviewed_at,omitempty"`
	AIRiskScore           *int                `json:"ai_risk_score,omitempty"`
	AIDecision            *string             `json:"ai_decision,omitempty"`
	AIDecisionReason      *string             `json:"ai_decision_reason,omitempty"`
	ApproverUserID        *uuid.UUID          `json:"approver_user_id,omitempty"`
	ApprovedAt            *time.Time          `json:"approved_at,omitempty"`
	RejectionReason       *string             `json:"rejection_reason,omitempty"`
	ReviewerUserID        *uuid.UUID          `json:"reviewer_user_id,omitempty"`
	PaidAt                *time.Time          `json:"paid_at,omitempty"`
	PaymentReference      *string             `json:"payment_reference,omitempty"`
	AccrualJournalEntryID *uuid.UUID          `json:"accrual_journal_entry_id,omitempty"`
	PaymentJournalEntryID *uuid.UUID          `json:"payment_journal_entry_id,omitempty"`
	Notes                 *string             `json:"notes,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	// Joined fields
	Items    []ExpenseItem    `json:"items,omitempty"`
	AuditLog []ExpenseAudit   `json:"audit_log,omitempty"`
}

type ExpenseItem struct {
	ID                   uuid.UUID        `json:"id"`
	ExpenseReportID      uuid.UUID        `json:"expense_report_id"`
	Category             string           `json:"category"`
	Description          string           `json:"description"`
	Amount               decimal.Decimal  `json:"amount"`
	Currency             string           `json:"currency"`
	MerchantName         *string          `json:"merchant_name,omitempty"`
	TransactionDate      time.Time        `json:"transaction_date"`
	ReceiptURL           *string          `json:"receipt_url,omitempty"`
	ReceiptOCRData       []byte           `json:"receipt_ocr_data,omitempty"`
	AICategoryConfidence *decimal.Decimal `json:"ai_category_confidence,omitempty"`
	GLAccountID          *uuid.UUID       `json:"gl_account_id,omitempty"`
	PolicyID             *uuid.UUID       `json:"policy_id,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

type ExpenseApprover struct {
	ID             uuid.UUID        `json:"id"`
	CompanyID      uuid.UUID        `json:"company_id"`
	DepartmentName string           `json:"department_name"`
	ApproverUserID uuid.UUID        `json:"approver_user_id"`
	MaxAmount      *decimal.Decimal `json:"max_amount,omitempty"`
	Priority       int              `json:"priority"`
	IsActive       bool             `json:"is_active"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	// Joined
	ApproverName  string `json:"approver_name,omitempty"`
	ApproverEmail string `json:"approver_email,omitempty"`
}

type ExpenseAudit struct {
	ID              uuid.UUID  `json:"id"`
	ExpenseReportID *uuid.UUID `json:"expense_report_id,omitempty"`
	Action          string     `json:"action"`
	ActorUserID     *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorType       string     `json:"actor_type"`
	Details         []byte     `json:"details,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// Joined
	ActorName string `json:"actor_name,omitempty"`
}

// -- Helpers --

// ValidStatusTransitions defines allowed state transitions.
var ValidStatusTransitions = map[ExpenseReportStatus][]ExpenseReportStatus{
	ExpenseStatusDraft:           {ExpenseStatusSubmitted},
	ExpenseStatusSubmitted:       {ExpenseStatusPendingApproval, ExpenseStatusApproved},
	ExpenseStatusPendingApproval: {ExpenseStatusApproved, ExpenseStatusRejected},
	ExpenseStatusRejected:        {ExpenseStatusDraft},
	ExpenseStatusApproved:        {ExpenseStatusPaid},
}

func CanTransition(from, to ExpenseReportStatus) bool {
	allowed, ok := ValidStatusTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
