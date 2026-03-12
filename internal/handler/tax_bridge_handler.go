package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/ebirforms"
)

// TaxBridgeHandler handles GL-to-tax bridge API endpoints.
type TaxBridgeHandler struct {
	bridge     *service.GLTaxBridge
	companySvc *service.CompanyService
	q          *sqlc.Queries
}

// NewTaxBridgeHandler creates a TaxBridgeHandler.
func NewTaxBridgeHandler(bridge *service.GLTaxBridge, companySvc *service.CompanyService, q *sqlc.Queries) *TaxBridgeHandler {
	return &TaxBridgeHandler{bridge: bridge, companySvc: companySvc, q: q}
}

type taxCalculateRequest struct {
	FormType    string `json:"form_type" binding:"required"`
	PeriodStart string `json:"period_start" binding:"required"`
	PeriodEnd   string `json:"period_end" binding:"required"`
}

// Calculate handles POST /api/v1/tax/calculate
func (h *TaxBridgeHandler) Calculate(c *gin.Context) {
	var req taxCalculateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		response.BadRequest(c, "invalid period_start (use YYYY-MM-DD)")
		return
	}

	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		response.BadRequest(c, "invalid period_end (use YYYY-MM-DD)")
		return
	}

	result, err := h.bridge.CalculateFromGL(c.Request.Context(), companyID, req.FormType, periodStart, periodEnd)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// Export handles GET /api/v1/tax/export?form_type=BIR_2550M&period=2025-01
func (h *TaxBridgeHandler) Export(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	formType := c.Query("form_type")
	periodStr := c.Query("period")

	if formType == "" || periodStr == "" {
		response.BadRequest(c, "form_type and period query parameters required")
		return
	}

	// Parse period (YYYY-MM)
	periodStart, err := time.Parse("2006-01", periodStr)
	if err != nil {
		response.BadRequest(c, "invalid period format (use YYYY-MM)")
		return
	}
	periodEnd := periodStart.AddDate(0, 1, -1) // last day of month

	// Calculate from GL
	result, err := h.bridge.CalculateFromGL(c.Request.Context(), companyID, formType, periodStart, periodEnd)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Get company info for DAT export
	company, err := h.companySvc.GetByID(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "failed to get company info")
		return
	}

	orgInfo := ebirforms.OrgInfo{
		TIN:            getStringOrDefault(company.TINNumber, ""),
		RegisteredName: company.CompanyName,
		TradeName:      company.CompanyName,
		RDOCode:        getStringOrDefault(company.RDOCode, "000"),
		ZipCode:        "0000",
		Address:        getStringOrDefault(company.Address, ""),
		TaxPayerType:   "C",
	}

	data, filename, err := ebirforms.ExportToDAT(formType, result.Result, orgInfo, periodStart)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(200, "application/octet-stream", data)
}

// GetLatestDraft handles GET /api/v1/tax/drafts/latest?form_type=BIR_2550M
func (h *TaxBridgeHandler) GetLatestDraft(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	formType := c.Query("form_type")

	if formType == "" {
		response.BadRequest(c, "form_type query parameter required")
		return
	}

	draft, err := h.q.GetLatestTaxDraft(c.Request.Context(), sqlc.GetLatestTaxDraftParams{
		CompanyID: companyID,
		FormType:  formType,
	})
	if err != nil {
		// No draft found is not an error
		response.OK(c, nil)
		return
	}

	var result map[string]string
	_ = json.Unmarshal(draft.Result, &result)

	resp := map[string]interface{}{
		"id":           draft.ID,
		"form_type":    draft.FormType,
		"triggered_by": draft.TriggeredBy,
		"result":       result,
	}
	if draft.PeriodStart.Valid {
		resp["period_start"] = draft.PeriodStart.Time.Format("2006-01-02")
	}
	if draft.PeriodEnd.Valid {
		resp["period_end"] = draft.PeriodEnd.Time.Format("2006-01-02")
	}
	if draft.CreatedAt.Valid {
		resp["created_at"] = draft.CreatedAt.Time
	}

	response.OK(c, resp)
}

func getStringOrDefault(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}
