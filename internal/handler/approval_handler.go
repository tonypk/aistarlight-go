package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// ApprovalHandler handles receipt approval endpoints.
type ApprovalHandler struct {
	approvals *service.ApprovalService
	q         *sqlc.Queries
}

// NewApprovalHandler creates an ApprovalHandler.
func NewApprovalHandler(approvals *service.ApprovalService, q *sqlc.Queries) *ApprovalHandler {
	return &ApprovalHandler{approvals: approvals, q: q}
}

// GetSettings handles GET /api/v1/approvals/settings.
func (h *ApprovalHandler) GetSettings(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	settings, err := h.approvals.GetSettings(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if settings == nil {
		response.OK(c, gin.H{
			"is_enabled":                  false,
			"amount_threshold":            10000,
			"new_vendor_receipts":         3,
			"risk_flags_require_approval": true,
		})
		return
	}
	response.OK(c, settings)
}

// UpdateSettings handles PUT /api/v1/approvals/settings.
func (h *ApprovalHandler) UpdateSettings(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req struct {
		IsEnabled                bool     `json:"is_enabled"`
		AmountThreshold          *float64 `json:"amount_threshold"`
		NewVendorReceipts        *int32   `json:"new_vendor_receipts"`
		RiskFlagsRequireApproval bool     `json:"risk_flags_require_approval"`
		ApproverUserID           *string  `json:"approver_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	params := sqlc.UpsertApprovalSettingsParams{
		CompanyID:                companyID,
		IsEnabled:                req.IsEnabled,
		NewVendorReceipts:        req.NewVendorReceipts,
		RiskFlagsRequireApproval: req.RiskFlagsRequireApproval,
	}

	if req.AmountThreshold != nil {
		params.AmountThreshold = floatToNumeric(*req.AmountThreshold)
	}

	if req.ApproverUserID != nil {
		uid, err := uuid.Parse(*req.ApproverUserID)
		if err != nil {
			response.BadRequest(c, "invalid approver_user_id")
			return
		}
		params.ApproverUserID = pgtype.UUID{Bytes: uid, Valid: true}
	}

	settings, err := h.q.UpsertApprovalSettings(c.Request.Context(), params)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, settings)
}

// ListPending handles GET /api/v1/approvals/pending.
func (h *ApprovalHandler) ListPending(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	approvals, err := h.q.ListPendingApprovals(c.Request.Context(), sqlc.ListPendingApprovalsParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	total, err := h.q.CountPendingApprovals(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, approvals, int(total), p.Page, p.Limit)
}

// List handles GET /api/v1/approvals.
func (h *ApprovalHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	approvals, err := h.q.ListApprovalsByCompany(c.Request.Context(), sqlc.ListApprovalsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	total, err := h.q.CountApprovalsByCompany(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, approvals, int(total), p.Page, p.Limit)
}

type approveRejectRequest struct {
	Notes *string `json:"notes"`
}

// Approve handles POST /api/v1/approvals/:id/approve.
func (h *ApprovalHandler) Approve(c *gin.Context) {
	approvalID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid approval id")
		return
	}
	userID := middleware.GetUserID(c)

	var req approveRejectRequest
	_ = c.ShouldBindJSON(&req)

	result, err := h.approvals.Approve(c.Request.Context(), approvalID, userID, req.Notes)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// Reject handles POST /api/v1/approvals/:id/reject.
func (h *ApprovalHandler) Reject(c *gin.Context) {
	approvalID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid approval id")
		return
	}
	userID := middleware.GetUserID(c)

	var req approveRejectRequest
	_ = c.ShouldBindJSON(&req)

	result, err := h.approvals.Reject(c.Request.Context(), approvalID, userID, req.Notes)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// floatToNumeric converts a float64 to pgtype.Numeric.
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(f)
	return n
}
