package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// TransactionHandler handles company-wide transaction endpoints.
type TransactionHandler struct {
	svc     *service.SessionService
	baseURL string
}

// NewTransactionHandler creates a TransactionHandler.
func NewTransactionHandler(svc *service.SessionService, baseURL string) *TransactionHandler {
	return &TransactionHandler{svc: svc, baseURL: baseURL}
}

// ListAll handles GET /api/v1/transactions.
func (h *TransactionHandler) ListAll(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	var filters service.TransactionFilters
	filters.SourceType = c.Query("source_type")
	filters.Category = c.Query("category")
	filters.Search = c.Query("search")

	if df := c.Query("date_from"); df != "" {
		if t, err := time.Parse("2006-01-02", df); err == nil {
			filters.DateFrom = pgtype.Date{Time: t, Valid: true}
		}
	}
	if dt := c.Query("date_to"); dt != "" {
		if t, err := time.Parse("2006-01-02", dt); err == nil {
			filters.DateTo = pgtype.Date{Time: t, Valid: true}
		}
	}

	txns, total, err := h.svc.ListCompanyTransactions(c.Request.Context(), companyID, p.Limit, p.Offset, filters, h.baseURL)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, txns, int(total), p.Page, p.Limit)
}
