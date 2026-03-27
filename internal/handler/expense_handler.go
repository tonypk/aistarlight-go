package handler

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// ExpenseHandler handles all expense management HTTP endpoints.
type ExpenseHandler struct {
	reportSvc   *service.ExpenseReportService
	policySvc   *service.ExpensePolicyService
	approverSvc *service.ExpenseApproverService
	receiptSvc  *service.ExpenseReceiptService
	aiSvc       *service.ExpenseAIService
	glSvc       *service.ExpenseGLService
	q           *sqlc.Queries
}

// NewExpenseHandler creates a new ExpenseHandler with all required dependencies.
func NewExpenseHandler(
	reportSvc *service.ExpenseReportService,
	policySvc *service.ExpensePolicyService,
	approverSvc *service.ExpenseApproverService,
	receiptSvc *service.ExpenseReceiptService,
	aiSvc *service.ExpenseAIService,
	glSvc *service.ExpenseGLService,
	q *sqlc.Queries,
) *ExpenseHandler {
	return &ExpenseHandler{
		reportSvc:   reportSvc,
		policySvc:   policySvc,
		approverSvc: approverSvc,
		receiptSvc:  receiptSvc,
		aiSvc:       aiSvc,
		glSvc:       glSvc,
		q:           q,
	}
}

// ===================== Request DTOs =====================

type createExpenseReportRequest struct {
	Title    string  `json:"title" binding:"required"`
	Notes    *string `json:"notes"`
	HRPayeeID *string `json:"hr_payee_id"`
}

type updateExpenseReportRequest struct {
	Title string  `json:"title" binding:"required"`
	Notes *string `json:"notes"`
}

type addExpenseItemRequest struct {
	Category        string  `json:"category" binding:"required"`
	Description     string  `json:"description" binding:"required"`
	Amount          string  `json:"amount" binding:"required"`
	Currency        string  `json:"currency" binding:"required"`
	MerchantName    *string `json:"merchant_name"`
	TransactionDate string  `json:"transaction_date" binding:"required"`
	GLAccountID     *string `json:"gl_account_id"`
	PolicyID        *string `json:"policy_id"`
}

type updateExpenseItemRequest struct {
	Category        string  `json:"category" binding:"required"`
	Description     string  `json:"description" binding:"required"`
	Amount          string  `json:"amount" binding:"required"`
	MerchantName    *string `json:"merchant_name"`
	TransactionDate string  `json:"transaction_date" binding:"required"`
	GLAccountID     *string `json:"gl_account_id"`
	PolicyID        *string `json:"policy_id"`
}

type rejectReportRequest struct {
	Reason *string `json:"reason"`
}

type markPaidRequest struct {
	PaymentReference *string `json:"payment_reference"`
}

type createPolicyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	Category             string  `json:"category" binding:"required"`
	MaxAmount            *string `json:"max_amount"`
	RequiresReceiptAbove *string `json:"requires_receipt_above"`
	AutoApproveBelow     *string `json:"auto_approve_below"`
	AIAutoApprove        bool    `json:"ai_auto_approve"`
	Description          *string `json:"description"`
}

type updatePolicyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	Category             string  `json:"category" binding:"required"`
	MaxAmount            *string `json:"max_amount"`
	RequiresReceiptAbove *string `json:"requires_receipt_above"`
	AutoApproveBelow     *string `json:"auto_approve_below"`
	AIAutoApprove        bool    `json:"ai_auto_approve"`
	Description          *string `json:"description"`
}

type createApproverRequest struct {
	DepartmentName string  `json:"department_name" binding:"required"`
	ApproverUserID string  `json:"approver_user_id" binding:"required"`
	MaxAmount      *string `json:"max_amount"`
	Priority       int     `json:"priority"`
}

type updateApproverRequest struct {
	DepartmentName string  `json:"department_name" binding:"required"`
	MaxAmount      *string `json:"max_amount"`
	Priority       int     `json:"priority"`
}

// ===================== Employee Endpoints =====================

// CreateReport creates a new expense report in draft status.
func (h *ExpenseHandler) CreateReport(c *gin.Context) {
	var req createExpenseReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	var hrPayeeID *uuid.UUID
	if req.HRPayeeID != nil {
		id, err := uuid.Parse(*req.HRPayeeID)
		if err != nil {
			response.BadRequest(c, "invalid hr_payee_id")
			return
		}
		hrPayeeID = &id
	}

	report, err := h.reportSvc.Create(c.Request.Context(), service.CreateExpenseReportInput{
		CompanyID:       companyID,
		SubmitterUserID: userID,
		HRPayeeID:       hrPayeeID,
		Title:           req.Title,
		Notes:           req.Notes,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, report)
}

// ListMyReports returns paginated expense reports for the current user.
func (h *ExpenseHandler) ListMyReports(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	status := c.DefaultQuery("status", "")
	p := pagination.Parse(c)

	reports, total, err := h.reportSvc.ListMy(c.Request.Context(), companyID, userID, status, int32(p.Page), int32(p.Limit))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Paginated(c, reports, int(total), p.Page, p.Limit)
}

// GetReport retrieves a single expense report by ID with items and audit log.
func (h *ExpenseHandler) GetReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	report, err := h.reportSvc.GetByID(c.Request.Context(), id, companyID)
	if err != nil {
		if errors.Is(err, service.ErrExpenseReportNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, report)
}

// UpdateReport updates a draft expense report's title and notes.
func (h *ExpenseHandler) UpdateReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req updateExpenseReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	err = h.reportSvc.UpdateDraft(c.Request.Context(), id, companyID, service.UpdateExpenseReportInput{
		Title: req.Title,
		Notes: req.Notes,
	})
	if err != nil {
		if errors.Is(err, service.ErrExpenseReportNotDraft) {
			response.Conflict(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseReportNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "updated"})
}

// DeleteReport deletes a draft expense report.
func (h *ExpenseHandler) DeleteReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	if err := h.reportSvc.DeleteDraft(c.Request.Context(), id, companyID); err != nil {
		if errors.Is(err, service.ErrExpenseReportNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}

// AddItem adds a line item to a draft expense report.
func (h *ExpenseHandler) AddItem(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req addExpenseItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		response.BadRequest(c, "invalid amount")
		return
	}

	txDate, err := time.Parse("2006-01-02", req.TransactionDate)
	if err != nil {
		response.BadRequest(c, "invalid transaction_date format (use YYYY-MM-DD)")
		return
	}

	var glAccountID *uuid.UUID
	if req.GLAccountID != nil {
		id, err := uuid.Parse(*req.GLAccountID)
		if err != nil {
			response.BadRequest(c, "invalid gl_account_id")
			return
		}
		glAccountID = &id
	}

	var policyID *uuid.UUID
	if req.PolicyID != nil {
		id, err := uuid.Parse(*req.PolicyID)
		if err != nil {
			response.BadRequest(c, "invalid policy_id")
			return
		}
		policyID = &id
	}

	companyID := middleware.GetCompanyID(c)
	item, err := h.reportSvc.AddItem(c.Request.Context(), reportID, companyID, service.CreateItemInput{
		Category:        req.Category,
		Description:     req.Description,
		Amount:          amount,
		Currency:        req.Currency,
		MerchantName:    req.MerchantName,
		TransactionDate: txDate,
		GLAccountID:     glAccountID,
		PolicyID:        policyID,
	})
	if err != nil {
		if errors.Is(err, service.ErrExpenseReportNotDraft) {
			response.Conflict(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseTooManyItems) {
			response.UnprocessableEntity(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseReportNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, item)
}

// UpdateItem updates an existing expense line item.
func (h *ExpenseHandler) UpdateItem(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid item ID")
		return
	}

	var req updateExpenseItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		response.BadRequest(c, "invalid amount")
		return
	}

	txDate, err := time.Parse("2006-01-02", req.TransactionDate)
	if err != nil {
		response.BadRequest(c, "invalid transaction_date format (use YYYY-MM-DD)")
		return
	}

	var glAccountID *uuid.UUID
	if req.GLAccountID != nil {
		id, err := uuid.Parse(*req.GLAccountID)
		if err != nil {
			response.BadRequest(c, "invalid gl_account_id")
			return
		}
		glAccountID = &id
	}

	var policyID *uuid.UUID
	if req.PolicyID != nil {
		id, err := uuid.Parse(*req.PolicyID)
		if err != nil {
			response.BadRequest(c, "invalid policy_id")
			return
		}
		policyID = &id
	}

	err = h.reportSvc.UpdateItem(c.Request.Context(), itemID, service.UpdateItemInput{
		Category:        req.Category,
		Description:     req.Description,
		Amount:          amount,
		MerchantName:    req.MerchantName,
		TransactionDate: txDate,
		GLAccountID:     glAccountID,
		PolicyID:        policyID,
	})
	if err != nil {
		if errors.Is(err, service.ErrExpenseItemFrozen) {
			response.Conflict(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseItemNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "updated"})
}

// DeleteItem removes an expense line item from a draft report.
func (h *ExpenseHandler) DeleteItem(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid item ID")
		return
	}

	err = h.reportSvc.DeleteItem(c.Request.Context(), itemID)
	if err != nil {
		if errors.Is(err, service.ErrExpenseItemFrozen) {
			response.Conflict(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseItemNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}

// UploadReceipt uploads a receipt file for an expense item.
// Validates file type, saves to disk, and triggers OCR extraction.
func (h *ExpenseHandler) UploadReceipt(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid item ID")
		return
	}

	companyID := middleware.GetCompanyID(c)

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file")
		return
	}

	ext, err := h.receiptSvc.ValidateReceiptFile(data)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	receiptURL, err := h.receiptSvc.SaveReceipt(c.Request.Context(), itemID, companyID, data, ext)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Run OCR extraction (non-blocking, ignore errors)
	var ocrResult *service.ExpenseOCRResult
	if h.aiSvc != nil {
		ocrResult, _ = h.aiSvc.ExtractReceipt(c.Request.Context(), data)
	}

	response.OK(c, gin.H{
		"receipt_url": receiptURL,
		"ocr":         ocrResult,
	})
}

// ServeReceipt serves a receipt file by item ID.
func (h *ExpenseHandler) ServeReceipt(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.BadRequest(c, "invalid item ID")
		return
	}

	path, err := h.receiptSvc.GetReceiptPath(c.Request.Context(), itemID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	c.File(path)
}

// SubmitReport submits a draft expense report for approval.
func (h *ExpenseHandler) SubmitReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	err = h.reportSvc.Submit(c.Request.Context(), id, companyID, userID)
	if err != nil {
		if errors.Is(err, service.ErrExpenseReportNoItems) {
			response.UnprocessableEntity(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseReportNotDraft) {
			response.Conflict(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrExpenseReportNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "submitted"})
}

// RevertToDraft reverts a rejected expense report back to draft status.
func (h *ExpenseHandler) RevertToDraft(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	err = h.reportSvc.RevertToDraft(c.Request.Context(), id, companyID, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "reverted to draft"})
}

// ===================== Approval Endpoints =====================

// ListPendingApprovals returns paginated pending expense reports for the current approver.
func (h *ExpenseHandler) ListPendingApprovals(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	p := pagination.Parse(c)

	reports, err := h.q.ListPendingApprovalsForUser(c.Request.Context(), sqlc.ListPendingApprovalsForUserParams{
		CompanyID:       companyID,
		SubmitterUserID: userID,
		Limit:           int32(p.Limit),
		Offset:          int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	total, err := h.q.CountPendingApprovalsForUser(c.Request.Context(), sqlc.CountPendingApprovalsForUserParams{
		CompanyID:       companyID,
		SubmitterUserID: userID,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, reports, int(total), p.Page, p.Limit)
}

// ApproveReport approves a pending expense report.
// Prevents self-approval, generates GL accrual entry, and writes audit log.
func (h *ExpenseHandler) ApproveReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	// Fetch report
	report, err := h.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		response.NotFound(c, "expense report not found")
		return
	}

	// Check self-approval
	if report.SubmitterUserID == userID {
		response.Forbidden(c, "cannot approve your own expense report")
		return
	}

	// Check status
	if report.Status != string(domain.ExpenseStatusPendingApproval) {
		response.Conflict(c, "report is not in pending_approval status")
		return
	}

	// Load report items for GL entry
	items, err := h.q.ListExpenseItemsByReport(ctx, id)
	if err != nil {
		response.InternalError(c, "failed to load report items")
		return
	}

	// Get domain report via service for GL entry creation
	domainReport, err := h.reportSvc.GetByID(ctx, id, companyID)
	if err != nil {
		response.InternalError(c, "failed to load report")
		return
	}

	// Convert sqlc items to domain items for GL
	domainItems := make([]domain.ExpenseItem, len(items))
	for i, item := range items {
		domainItems[i] = domain.ExpenseItem{
			ID:              item.ID,
			ExpenseReportID: item.ExpenseReportID,
			Category:        item.Category,
			Description:     item.Description,
			Amount:          numericToDecimal(item.Amount),
			Currency:        item.Currency,
			MerchantName:    item.MerchantName,
			GLAccountID:     pgtypeUUIDToPtr(item.GlAccountID),
		}
	}

	// Create GL accrual entry
	var accrualJournalEntryID pgtype.UUID
	if h.glSvc != nil {
		journalID, err := h.glSvc.CreateAccrualEntry(ctx, domainReport, domainItems)
		if err == nil && journalID != nil {
			accrualJournalEntryID = pgtype.UUID{Bytes: *journalID, Valid: true}
		}
	}

	// Approve
	err = h.q.UpdateExpenseReportApprove(ctx, sqlc.UpdateExpenseReportApproveParams{
		ID:                    id,
		CompanyID:             companyID,
		ApproverUserID:        pgtype.UUID{Bytes: userID, Valid: true},
		AccrualJournalEntryID: accrualJournalEntryID,
	})
	if err != nil {
		response.InternalError(c, "failed to approve report")
		return
	}

	// Write audit log
	_, _ = h.q.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionApproved,
		ActorUserID:     pgtype.UUID{Bytes: userID, Valid: true},
		ActorType:       "user",
	})

	response.OK(c, gin.H{"message": "approved"})
}

// RejectReport rejects a pending expense report with an optional reason.
func (h *ExpenseHandler) RejectReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req rejectReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body (reason is optional)
		req = rejectReportRequest{}
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	// Fetch report
	report, err := h.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		response.NotFound(c, "expense report not found")
		return
	}

	// Check self-approval
	if report.SubmitterUserID == userID {
		response.Forbidden(c, "cannot reject your own expense report")
		return
	}

	// Check status
	if report.Status != string(domain.ExpenseStatusPendingApproval) {
		response.Conflict(c, "report is not in pending_approval status")
		return
	}

	// Reject
	err = h.q.UpdateExpenseReportReject(ctx, sqlc.UpdateExpenseReportRejectParams{
		ID:              id,
		CompanyID:       companyID,
		ApproverUserID:  pgtype.UUID{Bytes: userID, Valid: true},
		RejectionReason: req.Reason,
	})
	if err != nil {
		response.InternalError(c, "failed to reject report")
		return
	}

	// Write audit log
	var details []byte
	if req.Reason != nil {
		details, _ = json.Marshal(map[string]string{"reason": *req.Reason})
	}
	_, _ = h.q.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionRejected,
		ActorUserID:     pgtype.UUID{Bytes: userID, Valid: true},
		ActorType:       "user",
		Details:         details,
	})

	response.OK(c, gin.H{"message": "rejected"})
}

// ===================== Finance Endpoints =====================

// ListFinanceQueue returns paginated approved expense reports awaiting payment.
func (h *ExpenseHandler) ListFinanceQueue(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	reports, err := h.q.ListExpenseReportsByStatus(c.Request.Context(), sqlc.ListExpenseReportsByStatusParams{
		CompanyID: companyID,
		Status:    string(domain.ExpenseStatusApproved),
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	total, err := h.q.CountExpenseReportsByStatus(c.Request.Context(), sqlc.CountExpenseReportsByStatusParams{
		CompanyID: companyID,
		Status:    string(domain.ExpenseStatusApproved),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, reports, int(total), p.Page, p.Limit)
}

// MarkPaid marks an approved expense report as paid and creates the payment GL entry.
func (h *ExpenseHandler) MarkPaid(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req markPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body (payment_reference is optional)
		req = markPaidRequest{}
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	// Fetch report
	report, err := h.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		response.NotFound(c, "expense report not found")
		return
	}

	// Check status
	if report.Status != string(domain.ExpenseStatusApproved) {
		response.Conflict(c, "report is not in approved status")
		return
	}

	// Get domain report for GL entry
	domainReport, err := h.reportSvc.GetByID(ctx, id, companyID)
	if err != nil {
		response.InternalError(c, "failed to load report")
		return
	}

	// Create payment GL entry
	var paymentJournalEntryID pgtype.UUID
	if h.glSvc != nil {
		journalID, err := h.glSvc.CreatePaymentEntry(ctx, domainReport)
		if err == nil && journalID != nil {
			paymentJournalEntryID = pgtype.UUID{Bytes: *journalID, Valid: true}
		}
	}

	// Mark paid
	err = h.q.UpdateExpenseReportPaid(ctx, sqlc.UpdateExpenseReportPaidParams{
		ID:                    id,
		CompanyID:             companyID,
		ReviewerUserID:        pgtype.UUID{Bytes: userID, Valid: true},
		PaymentReference:      req.PaymentReference,
		PaymentJournalEntryID: paymentJournalEntryID,
	})
	if err != nil {
		response.InternalError(c, "failed to mark report as paid")
		return
	}

	// Write audit log
	var details []byte
	if req.PaymentReference != nil {
		details, _ = json.Marshal(map[string]string{"payment_reference": *req.PaymentReference})
	}
	_, _ = h.q.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: id, Valid: true},
		Action:          domain.AuditActionPaid,
		ActorUserID:     pgtype.UUID{Bytes: userID, Valid: true},
		ActorType:       "user",
		Details:         details,
	})

	response.OK(c, gin.H{"message": "marked as paid"})
}

// ===================== Admin Policy Endpoints =====================

// CreatePolicy creates a new expense policy.
func (h *ExpenseHandler) CreatePolicy(c *gin.Context) {
	var req createPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	maxAmount, err := parseOptionalDecimal(req.MaxAmount)
	if err != nil {
		response.BadRequest(c, "invalid max_amount")
		return
	}
	requiresReceiptAbove, err := parseOptionalDecimal(req.RequiresReceiptAbove)
	if err != nil {
		response.BadRequest(c, "invalid requires_receipt_above")
		return
	}
	autoApproveBelow, err := parseOptionalDecimal(req.AutoApproveBelow)
	if err != nil {
		response.BadRequest(c, "invalid auto_approve_below")
		return
	}

	policy, err := h.policySvc.Create(c.Request.Context(), service.CreatePolicyInput{
		CompanyID:            companyID,
		Name:                 req.Name,
		Category:             req.Category,
		MaxAmount:            maxAmount,
		RequiresReceiptAbove: requiresReceiptAbove,
		AutoApproveBelow:     autoApproveBelow,
		AIAutoApprove:        req.AIAutoApprove,
		Description:          req.Description,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCategory) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, policy)
}

// ListPolicies returns all active expense policies for the company.
func (h *ExpenseHandler) ListPolicies(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	policies, err := h.policySvc.List(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, policies)
}

// GetPolicy retrieves a single expense policy by ID.
func (h *ExpenseHandler) GetPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	policy, err := h.policySvc.GetByID(c.Request.Context(), id, companyID)
	if err != nil {
		if errors.Is(err, service.ErrPolicyNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, policy)
}

// UpdatePolicy updates an existing expense policy.
func (h *ExpenseHandler) UpdatePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy ID")
		return
	}

	var req updatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	maxAmount, err := parseOptionalDecimal(req.MaxAmount)
	if err != nil {
		response.BadRequest(c, "invalid max_amount")
		return
	}
	requiresReceiptAbove, err := parseOptionalDecimal(req.RequiresReceiptAbove)
	if err != nil {
		response.BadRequest(c, "invalid requires_receipt_above")
		return
	}
	autoApproveBelow, err := parseOptionalDecimal(req.AutoApproveBelow)
	if err != nil {
		response.BadRequest(c, "invalid auto_approve_below")
		return
	}

	err = h.policySvc.Update(c.Request.Context(), id, companyID, service.UpdatePolicyInput{
		Name:                 req.Name,
		Category:             req.Category,
		MaxAmount:            maxAmount,
		RequiresReceiptAbove: requiresReceiptAbove,
		AutoApproveBelow:     autoApproveBelow,
		AIAutoApprove:        req.AIAutoApprove,
		Description:          req.Description,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCategory) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "updated"})
}

// DeletePolicy deactivates an expense policy (soft delete).
func (h *ExpenseHandler) DeletePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	if err := h.policySvc.Deactivate(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deactivated"})
}

// ===================== Admin Approver Endpoints =====================

// CreateApprover registers a new expense approver for a department.
func (h *ExpenseHandler) CreateApprover(c *gin.Context) {
	var req createApproverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	approverUserID, err := uuid.Parse(req.ApproverUserID)
	if err != nil {
		response.BadRequest(c, "invalid approver_user_id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	maxAmount, err := parseOptionalDecimal(req.MaxAmount)
	if err != nil {
		response.BadRequest(c, "invalid max_amount")
		return
	}

	approver, err := h.approverSvc.Create(c.Request.Context(), service.CreateApproverInput{
		CompanyID:      companyID,
		DepartmentName: req.DepartmentName,
		ApproverUserID: approverUserID,
		MaxAmount:      maxAmount,
		Priority:       req.Priority,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, approver)
}

// ListApprovers returns all active expense approvers for the company.
func (h *ExpenseHandler) ListApprovers(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	approvers, err := h.approverSvc.List(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, approvers)
}

// UpdateApprover modifies an existing expense approver's configuration.
func (h *ExpenseHandler) UpdateApprover(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid approver ID")
		return
	}

	var req updateApproverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	maxAmount, err := parseOptionalDecimal(req.MaxAmount)
	if err != nil {
		response.BadRequest(c, "invalid max_amount")
		return
	}

	err = h.approverSvc.Update(c.Request.Context(), id, companyID, service.UpdateApproverInput{
		DepartmentName: req.DepartmentName,
		MaxAmount:      maxAmount,
		Priority:       req.Priority,
	})
	if err != nil {
		if errors.Is(err, service.ErrApproverNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "updated"})
}

// DeleteApprover deactivates an expense approver (soft delete).
func (h *ExpenseHandler) DeleteApprover(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid approver ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	if err := h.approverSvc.Deactivate(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deactivated"})
}

// ===================== Analytics Endpoint =====================

// GetAnalytics returns expense analytics: summary counts, spend by category, and spend by department.
// Query params: from, to (YYYY-MM-DD format). Defaults to current month.
func (h *ExpenseHandler) GetAnalytics(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	ctx := c.Request.Context()

	fromStr := c.DefaultQuery("from", time.Now().Format("2006-01")+"-01")
	toStr := c.DefaultQuery("to", time.Now().AddDate(0, 1, 0).Format("2006-01")+"-01")

	fromTime, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		response.BadRequest(c, "invalid 'from' date format (use YYYY-MM-DD)")
		return
	}
	toTime, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		response.BadRequest(c, "invalid 'to' date format (use YYYY-MM-DD)")
		return
	}

	// Summary counts
	summary, err := h.q.GetExpenseSpendSummary(ctx, sqlc.GetExpenseSpendSummaryParams{
		CompanyID:   companyID,
		CreatedAt:   fromTime,
		CreatedAt_2: toTime,
	})
	if err != nil {
		response.InternalError(c, "failed to load summary: "+err.Error())
		return
	}

	// Spend by category
	fromTZ := pgtype.Timestamptz{Time: fromTime, Valid: true}
	toTZ := pgtype.Timestamptz{Time: toTime, Valid: true}

	byCategory, err := h.q.GetExpenseSpendByCategory(ctx, sqlc.GetExpenseSpendByCategoryParams{
		CompanyID:    companyID,
		ApprovedAt:   fromTZ,
		ApprovedAt_2: toTZ,
	})
	if err != nil {
		response.InternalError(c, "failed to load category breakdown: "+err.Error())
		return
	}

	// Spend by department
	byDepartment, err := h.q.GetExpenseSpendByDepartment(ctx, sqlc.GetExpenseSpendByDepartmentParams{
		CompanyID:    companyID,
		ApprovedAt:   fromTZ,
		ApprovedAt_2: toTZ,
	})
	if err != nil {
		response.InternalError(c, "failed to load department breakdown: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"summary":       summary,
		"by_category":   byCategory,
		"by_department": byDepartment,
		"period": gin.H{
			"from": fromStr,
			"to":   toStr,
		},
	})
}

// ===================== Helpers =====================

// parseOptionalDecimal parses an optional string into *decimal.Decimal.
func parseOptionalDecimal(s *string) (*decimal.Decimal, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// numericToDecimal converts pgtype.Numeric to decimal.Decimal.
func numericToDecimal(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

// pgtypeUUIDToPtr converts pgtype.UUID to *uuid.UUID.
func pgtypeUUIDToPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
