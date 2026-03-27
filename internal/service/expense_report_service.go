package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrExpenseReportNotFound = errors.New("expense report not found")
	ErrExpenseReportNotDraft = errors.New("expense report is not in draft status")
	ErrExpenseReportNoItems  = errors.New("expense report has no items")
	ErrExpenseItemNotFound   = errors.New("expense item not found")
	ErrExpenseItemFrozen     = errors.New("cannot modify item: report is not in draft status")
	ErrExpenseTooManyItems   = errors.New("expense report cannot have more than 50 items")
)

// ExpenseAIEvaluator evaluates expense reports for policy compliance.
type ExpenseAIEvaluator interface {
	EvaluateReport(ctx context.Context, reportID, companyID uuid.UUID) error
}

// ExpenseGLCreator creates GL journal entries for approved expenses.
type ExpenseGLCreator interface {
	CreateAccrualEntry(ctx context.Context, report *domain.ExpenseReport, items []domain.ExpenseItem) (*uuid.UUID, error)
}

type ExpenseReportService struct {
	q           *sqlc.Queries
	pool        *pgxpool.Pool
	approverSvc *ExpenseApproverService
	aiSvc       ExpenseAIEvaluator
	glSvc       ExpenseGLCreator
}

func NewExpenseReportService(q *sqlc.Queries, pool *pgxpool.Pool, approverSvc *ExpenseApproverService, aiSvc ExpenseAIEvaluator, glSvc ExpenseGLCreator) *ExpenseReportService {
	return &ExpenseReportService{q: q, pool: pool, approverSvc: approverSvc, aiSvc: aiSvc, glSvc: glSvc}
}

// -- Input DTOs --

type CreateExpenseReportInput struct {
	CompanyID       uuid.UUID
	SubmitterUserID uuid.UUID
	HRPayeeID       *uuid.UUID
	Title           string
	Notes           *string
}

type UpdateExpenseReportInput struct {
	Title string
	Notes *string
}

type CreateItemInput struct {
	Category        string
	Description     string
	Amount          decimal.Decimal
	Currency        string
	MerchantName    *string
	TransactionDate time.Time
	ReceiptURL      *string
	ReceiptOCRData  []byte
	GLAccountID     *uuid.UUID
	PolicyID        *uuid.UUID
}

type UpdateItemInput struct {
	Category        string
	Description     string
	Amount          decimal.Decimal
	MerchantName    *string
	TransactionDate time.Time
	GLAccountID     *uuid.UUID
	PolicyID        *uuid.UUID
}

// -- Report CRUD --

// Create generates a report number in a transaction and creates a new expense report.
func (s *ExpenseReportService) Create(ctx context.Context, input CreateExpenseReportInput) (*domain.ExpenseReport, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// Advisory lock for report number generation
	if err := qtx.AcquireExpenseReportNumberLock(ctx, input.CompanyID.String()); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}

	nextSeq, err := qtx.GenerateExpenseReportNumber(ctx, input.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("generate number: %w", err)
	}
	reportNumber := fmt.Sprintf("EXP-%s-%04d", time.Now().Format("200601"), nextSeq)

	var zeroNum pgtype.Numeric
	_ = zeroNum.Scan("0")

	id := uuid.New()
	row, err := qtx.CreateExpenseReport(ctx, sqlc.CreateExpenseReportParams{
		ID:              id,
		CompanyID:       input.CompanyID,
		SubmitterUserID: input.SubmitterUserID,
		HrPayeeID:       uuidToNullUUID(input.HRPayeeID),
		ReportNumber:    reportNumber,
		Title:           input.Title,
		Status:          string(domain.ExpenseStatusDraft),
		TotalAmount:     zeroNum,
		Currency:        "PHP",
		Notes:           input.Notes,
	})
	if err != nil {
		return nil, fmt.Errorf("create expense report: %w", err)
	}

	// Write audit log
	_, _ = qtx.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionCreated,
		ActorUserID:     pgtype.UUID{Bytes: input.SubmitterUserID, Valid: true},
		ActorType:       "user",
	})

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return toExpenseReport(row), nil
}

// GetByID retrieves a report with items and audit log.
func (s *ExpenseReportService) GetByID(ctx context.Context, id, companyID uuid.UUID) (*domain.ExpenseReport, error) {
	row, err := s.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, ErrExpenseReportNotFound
	}
	report := toExpenseReport(row)

	// Fetch items
	items, err := s.q.ListExpenseItemsByReport(ctx, id)
	if err == nil {
		for _, item := range items {
			report.Items = append(report.Items, *toExpenseItem(item))
		}
	}

	// Fetch audit log
	auditRows, err := s.q.ListExpenseAuditLog(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err == nil {
		for _, a := range auditRows {
			report.AuditLog = append(report.AuditLog, toExpenseAudit(a))
		}
	}

	return report, nil
}

// ListMy returns paginated expense reports for a submitter.
func (s *ExpenseReportService) ListMy(ctx context.Context, companyID, userID uuid.UUID, status string, page, pageSize int32) ([]domain.ExpenseReport, int64, error) {
	offset := (page - 1) * pageSize

	rows, err := s.q.ListExpenseReportsBySubmitter(ctx, sqlc.ListExpenseReportsBySubmitterParams{
		CompanyID:       companyID,
		SubmitterUserID: userID,
		Column3:         status,
		Limit:           pageSize,
		Offset:          offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list expense reports: %w", err)
	}

	count, err := s.q.CountExpenseReportsBySubmitter(ctx, sqlc.CountExpenseReportsBySubmitterParams{
		CompanyID:       companyID,
		SubmitterUserID: userID,
		Column3:         status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count expense reports: %w", err)
	}

	result := make([]domain.ExpenseReport, len(rows))
	for i, row := range rows {
		result[i] = *toExpenseReport(row)
	}
	return result, count, nil
}

// UpdateDraft updates title/notes of a draft report.
func (s *ExpenseReportService) UpdateDraft(ctx context.Context, id, companyID uuid.UUID, input UpdateExpenseReportInput) error {
	return s.q.UpdateExpenseReportDraft(ctx, sqlc.UpdateExpenseReportDraftParams{
		ID:        id,
		CompanyID: companyID,
		Title:     input.Title,
		Notes:     input.Notes,
	})
}

// DeleteDraft deletes a draft report.
func (s *ExpenseReportService) DeleteDraft(ctx context.Context, id, companyID uuid.UUID) error {
	return s.q.DeleteExpenseReportDraft(ctx, sqlc.DeleteExpenseReportDraftParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// -- Item CRUD --

// AddItem adds a line item to a draft report.
func (s *ExpenseReportService) AddItem(ctx context.Context, reportID, companyID uuid.UUID, input CreateItemInput) (*domain.ExpenseItem, error) {
	// Verify report is draft
	report, err := s.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        reportID,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, ErrExpenseReportNotFound
	}
	if report.Status != string(domain.ExpenseStatusDraft) {
		return nil, ErrExpenseReportNotDraft
	}

	// Check max items
	count, err := s.q.CountExpenseItemsByReport(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("count items: %w", err)
	}
	if count >= 50 {
		return nil, ErrExpenseTooManyItems
	}

	row, err := s.q.CreateExpenseItem(ctx, sqlc.CreateExpenseItemParams{
		ID:                   uuid.New(),
		ExpenseReportID:      reportID,
		Category:             input.Category,
		Description:          input.Description,
		Amount:               decimalToNumericVal(input.Amount),
		Currency:             input.Currency,
		MerchantName:         input.MerchantName,
		TransactionDate:      timeToDate(input.TransactionDate),
		ReceiptUrl:           input.ReceiptURL,
		ReceiptOcrData:       input.ReceiptOCRData,
		AiCategoryConfidence: pgtype.Numeric{},
		GlAccountID:          uuidToNullUUID(input.GLAccountID),
		PolicyID:             uuidToNullUUID(input.PolicyID),
	})
	if err != nil {
		return nil, fmt.Errorf("create expense item: %w", err)
	}

	return toExpenseItem(row), nil
}

// UpdateItem updates a line item (only if parent report is draft).
func (s *ExpenseReportService) UpdateItem(ctx context.Context, itemID uuid.UUID, input UpdateItemInput) error {
	item, err := s.q.GetExpenseItemByID(ctx, itemID)
	if err != nil {
		return ErrExpenseItemNotFound
	}
	if item.ReportStatus != string(domain.ExpenseStatusDraft) {
		return ErrExpenseItemFrozen
	}

	return s.q.UpdateExpenseItem(ctx, sqlc.UpdateExpenseItemParams{
		ID:              itemID,
		Category:        input.Category,
		Description:     input.Description,
		Amount:          decimalToNumericVal(input.Amount),
		MerchantName:    input.MerchantName,
		TransactionDate: timeToDate(input.TransactionDate),
		GlAccountID:     uuidToNullUUID(input.GLAccountID),
		PolicyID:        uuidToNullUUID(input.PolicyID),
	})
}

// DeleteItem removes a line item (only if parent report is draft).
func (s *ExpenseReportService) DeleteItem(ctx context.Context, itemID uuid.UUID) error {
	item, err := s.q.GetExpenseItemByID(ctx, itemID)
	if err != nil {
		return ErrExpenseItemNotFound
	}
	if item.ReportStatus != string(domain.ExpenseStatusDraft) {
		return ErrExpenseItemFrozen
	}

	return s.q.DeleteExpenseItem(ctx, itemID)
}

// -- Submit Flow --

// Submit submits a draft report: recompute total, trigger AI review.
func (s *ExpenseReportService) Submit(ctx context.Context, id, companyID, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// Lock report
	report, err := qtx.GetExpenseReportForUpdate(ctx, sqlc.GetExpenseReportForUpdateParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return ErrExpenseReportNotFound
	}
	if report.Status != string(domain.ExpenseStatusDraft) {
		return ErrExpenseReportNotDraft
	}

	// Count items
	count, err := qtx.CountExpenseItemsByReport(ctx, id)
	if err != nil {
		return fmt.Errorf("count items: %w", err)
	}
	if count == 0 {
		return ErrExpenseReportNoItems
	}

	// Recompute total
	total, err := qtx.SumExpenseItemsByReport(ctx, id)
	if err != nil {
		return fmt.Errorf("sum items: %w", err)
	}

	// Update to submitted
	if err := qtx.UpdateExpenseReportSubmit(ctx, sqlc.UpdateExpenseReportSubmitParams{
		ID:          id,
		CompanyID:   companyID,
		TotalAmount: total,
	}); err != nil {
		return fmt.Errorf("update submit: %w", err)
	}

	// Audit log
	_, _ = qtx.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionSubmitted,
		ActorUserID:     pgtype.UUID{Bytes: userID, Valid: true},
		ActorType:       "user",
	})

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// AI policy agent runs after commit (async)
	if s.aiSvc != nil {
		go s.aiSvc.EvaluateReport(context.Background(), id, companyID)
	}

	return nil
}

// RevertToDraft reverts a rejected report back to draft.
func (s *ExpenseReportService) RevertToDraft(ctx context.Context, id, companyID, userID uuid.UUID) error {
	if err := s.q.UpdateExpenseReportRejectToDraft(ctx, sqlc.UpdateExpenseReportRejectToDraftParams{
		ID:        id,
		CompanyID: companyID,
	}); err != nil {
		return fmt.Errorf("revert to draft: %w", err)
	}

	_, _ = s.q.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionEdited,
		ActorUserID:     pgtype.UUID{Bytes: userID, Valid: true},
		ActorType:       "user",
		Details:         []byte(`{"action":"reverted_to_draft"}`),
	})

	return nil
}

// -- Conversion helpers --

func uuidToNullUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func nullUUIDToPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

func timeToPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func int32ToPtr(n *int32) *int {
	if n == nil {
		return nil
	}
	v := int(*n)
	return &v
}

func decimalToNumericVal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func numericToDecimalVal(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func timeToDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func dateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

func toExpenseReport(r sqlc.ExpenseReport) *domain.ExpenseReport {
	return &domain.ExpenseReport{
		ID:                    r.ID,
		CompanyID:             r.CompanyID,
		SubmitterUserID:       r.SubmitterUserID,
		HRPayeeID:             nullUUIDToPtr(r.HrPayeeID),
		ReportNumber:          r.ReportNumber,
		Title:                 r.Title,
		Status:                domain.ExpenseReportStatus(r.Status),
		TotalAmount:           numericToDecimalVal(r.TotalAmount),
		Currency:              r.Currency,
		SubmittedAt:           timeToPtr(r.SubmittedAt),
		AIReviewedAt:          timeToPtr(r.AiReviewedAt),
		AIRiskScore:           int32ToPtr(r.AiRiskScore),
		AIDecision:            r.AiDecision,
		AIDecisionReason:      r.AiDecisionReason,
		ApproverUserID:        nullUUIDToPtr(r.ApproverUserID),
		ApprovedAt:            timeToPtr(r.ApprovedAt),
		RejectionReason:       r.RejectionReason,
		ReviewerUserID:        nullUUIDToPtr(r.ReviewerUserID),
		PaidAt:                timeToPtr(r.PaidAt),
		PaymentReference:      r.PaymentReference,
		AccrualJournalEntryID: nullUUIDToPtr(r.AccrualJournalEntryID),
		PaymentJournalEntryID: nullUUIDToPtr(r.PaymentJournalEntryID),
		Notes:                 r.Notes,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
	}
}

func toExpenseItem(i sqlc.ExpenseItem) *domain.ExpenseItem {
	return &domain.ExpenseItem{
		ID:                   i.ID,
		ExpenseReportID:      i.ExpenseReportID,
		Category:             i.Category,
		Description:          i.Description,
		Amount:               numericToDecimalVal(i.Amount),
		Currency:             i.Currency,
		MerchantName:         i.MerchantName,
		TransactionDate:      dateToTime(i.TransactionDate),
		ReceiptURL:           i.ReceiptUrl,
		ReceiptOCRData:       i.ReceiptOcrData,
		AICategoryConfidence: numericToPtrDecimal(i.AiCategoryConfidence),
		GLAccountID:          nullUUIDToPtr(i.GlAccountID),
		PolicyID:             nullUUIDToPtr(i.PolicyID),
		CreatedAt:            i.CreatedAt,
		UpdatedAt:            i.UpdatedAt,
	}
}

func toExpenseAudit(a sqlc.ListExpenseAuditLogRow) domain.ExpenseAudit {
	return domain.ExpenseAudit{
		ID:              a.ID,
		ExpenseReportID: nullUUIDToPtr(a.ExpenseReportID),
		Action:          a.Action,
		ActorUserID:     nullUUIDToPtr(a.ActorUserID),
		ActorType:       a.ActorType,
		Details:         a.Details,
		CreatedAt:       a.CreatedAt,
		ActorName:       a.ActorName,
	}
}
