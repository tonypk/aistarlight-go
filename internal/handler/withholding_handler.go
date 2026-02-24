package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// WithholdingHandler handles EWT endpoints.
type WithholdingHandler struct {
	svc *service.WithholdingService
}

// NewWithholdingHandler creates a withholding handler.
func NewWithholdingHandler(svc *service.WithholdingService) *WithholdingHandler {
	return &WithholdingHandler{svc: svc}
}

type classifyEWTRequest struct {
	Transactions []map[string]interface{} `json:"transactions" binding:"required"`
}

// Classify handles POST /api/v1/withholding/classify.
func (h *WithholdingHandler) Classify(c *gin.Context) {
	var req classifyEWTRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	results, err := h.svc.ClassifyEWTTransactions(c.Request.Context(), req.Transactions, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, results)
}

type createCertificateRequest struct {
	SupplierID   string `json:"supplier_id" binding:"required"`
	SessionID    *string `json:"session_id"`
	Period       string `json:"period" binding:"required"`
	Quarter      string `json:"quarter" binding:"required"`
	ATCCode      string `json:"atc_code" binding:"required"`
	IncomeAmount string `json:"income_amount" binding:"required"`
	EWTRate      string `json:"ewt_rate" binding:"required"`
	TaxWithheld  string `json:"tax_withheld" binding:"required"`
}

// CreateCertificate handles POST /api/v1/withholding/certificates.
func (h *WithholdingHandler) CreateCertificate(c *gin.Context) {
	var req createCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	supplierID, err := uuid.Parse(req.SupplierID)
	if err != nil {
		response.BadRequest(c, "invalid supplier_id")
		return
	}

	var sessionID *uuid.UUID
	if req.SessionID != nil {
		sid, err := uuid.Parse(*req.SessionID)
		if err == nil {
			sessionID = &sid
		}
	}

	incomeAmount, _ := decimal.NewFromString(req.IncomeAmount)
	ewtRate, _ := decimal.NewFromString(req.EWTRate)
	taxWithheld, _ := decimal.NewFromString(req.TaxWithheld)

	companyID := middleware.GetCompanyID(c)

	cert, err := h.svc.CreateCertificate(
		c.Request.Context(),
		companyID, supplierID, sessionID,
		req.Period, req.Quarter, req.ATCCode,
		incomeAmount, ewtRate, taxWithheld,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, cert)
}

// ListCertificates handles GET /api/v1/withholding/certificates.
func (h *WithholdingHandler) ListCertificates(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	certs, total, err := h.svc.ListCertificates(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, certs, int(total), p.Page, p.Limit)
}

// GetCertificate handles GET /api/v1/withholding/certificates/:id.
func (h *WithholdingHandler) GetCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.GetCertificate(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, cert)
}

// Rates handles GET /api/v1/withholding/rates.
func (h *WithholdingHandler) Rates(c *gin.Context) {
	response.OK(c, service.ListEWTRates())
}
