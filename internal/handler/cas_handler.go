package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// CASHandler handles BIR CAS compliance endpoints.
type CASHandler struct {
	svc *service.CASService
}

func NewCASHandler(svc *service.CASService) *CASHandler {
	return &CASHandler{svc: svc}
}

// RunCheck runs a full CAS compliance check for the user's company.
// POST /api/v1/cas/check
func (h *CASHandler) RunCheck(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	result, err := h.svc.RunComplianceCheck(c.Request.Context(), companyID, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, result)
}

// GetLatest returns the most recent CAS compliance check.
// GET /api/v1/cas/latest
func (h *CASHandler) GetLatest(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	result, err := h.svc.GetLatestCheck(c.Request.Context(), companyID)
	if err != nil {
		response.NotFound(c, "no compliance checks found")
		return
	}
	response.OK(c, result)
}

// ListChecks returns paginated CAS compliance check history.
// GET /api/v1/cas/checks?limit=10&offset=0
func (h *CASHandler) ListChecks(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset, err2 := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err2 != nil || offset < 0 {
		offset = 0
	}

	checks, err := h.svc.ListChecks(c.Request.Context(), companyID, limit, offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, checks)
}

// SubsidiaryLedger returns subsidiary ledger entries for a journal book type.
// GET /api/v1/cas/subsidiary-ledger?book=sales_journal&from=2025-01-01&to=2025-12-31
func (h *CASHandler) SubsidiaryLedger(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	book := c.DefaultQuery("book", service.JournalBookGeneral)

	var from, to *time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = &t
		}
	}

	entries, err := h.svc.GetSubsidiaryLedger(c.Request.Context(), companyID, book, from, to)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, entries)
}
