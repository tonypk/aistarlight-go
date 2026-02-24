package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// FinancialStatementHandler handles financial statement API endpoints.
type FinancialStatementHandler struct {
	svc *service.FinancialStatementService
}

// NewFinancialStatementHandler creates a FinancialStatementHandler.
func NewFinancialStatementHandler(svc *service.FinancialStatementService) *FinancialStatementHandler {
	return &FinancialStatementHandler{svc: svc}
}

// BalanceSheet handles GET /api/v1/statements/balance-sheet?as_of=2025-12-31
func (h *FinancialStatementHandler) BalanceSheet(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	asOfStr := c.Query("as_of")
	if asOfStr == "" {
		asOfStr = time.Now().Format("2006-01-02")
	}

	asOf, err := time.Parse("2006-01-02", asOfStr)
	if err != nil {
		response.BadRequest(c, "invalid as_of date (use YYYY-MM-DD)")
		return
	}

	bs, err := h.svc.BalanceSheet(c.Request.Context(), companyID, asOf)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, bs)
}

// IncomeStatement handles GET /api/v1/statements/income-statement?from=2025-01-01&to=2025-12-31
func (h *FinancialStatementHandler) IncomeStatement(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	fromStr := c.Query("from")
	toStr := c.Query("to")

	if fromStr == "" || toStr == "" {
		response.BadRequest(c, "from and to query parameters required (YYYY-MM-DD)")
		return
	}

	fromDate, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		response.BadRequest(c, "invalid from date (use YYYY-MM-DD)")
		return
	}

	toDate, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		response.BadRequest(c, "invalid to date (use YYYY-MM-DD)")
		return
	}

	is, err := h.svc.IncomeStatement(c.Request.Context(), companyID, fromDate, toDate)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, is)
}
