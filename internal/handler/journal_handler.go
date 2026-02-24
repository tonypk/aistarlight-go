package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type JournalHandler struct {
	svc *service.JournalService
}

func NewJournalHandler(svc *service.JournalService) *JournalHandler {
	return &JournalHandler{svc: svc}
}

type createJournalEntryRequest struct {
	EntryDate   string                    `json:"entry_date" binding:"required"`
	Reference   *string                   `json:"reference"`
	Description *string                   `json:"description"`
	SourceType  *string                   `json:"source_type"`
	SourceID    *string                   `json:"source_id"`
	Memo        *string                   `json:"memo"`
	Lines       []createJournalLineRequest `json:"lines" binding:"required,min=2"`
}

type createJournalLineRequest struct {
	AccountID   string `json:"account_id" binding:"required"`
	Description string `json:"description"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
}

func (h *JournalHandler) Create(c *gin.Context) {
	var req createJournalEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	entryDate, err := time.Parse("2006-01-02", req.EntryDate)
	if err != nil {
		response.BadRequest(c, "invalid entry_date format (use YYYY-MM-DD)")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	lines := make([]service.CreateJournalLineInput, len(req.Lines))
	for i, l := range req.Lines {
		accountID, err := uuid.Parse(l.AccountID)
		if err != nil {
			response.BadRequest(c, "invalid account_id in line")
			return
		}

		debit := decimal.Zero
		credit := decimal.Zero
		if l.Debit != "" {
			debit, err = decimal.NewFromString(l.Debit)
			if err != nil {
				response.BadRequest(c, "invalid debit amount in line")
				return
			}
		}
		if l.Credit != "" {
			credit, err = decimal.NewFromString(l.Credit)
			if err != nil {
				response.BadRequest(c, "invalid credit amount in line")
				return
			}
		}

		var desc *string
		if l.Description != "" {
			desc = &l.Description
		}

		lines[i] = service.CreateJournalLineInput{
			AccountID:   accountID,
			Description: desc,
			Debit:       debit,
			Credit:      credit,
		}
	}

	var sourceID *uuid.UUID
	if req.SourceID != nil {
		sid, err := uuid.Parse(*req.SourceID)
		if err == nil {
			sourceID = &sid
		}
	}

	entry, err := h.svc.Create(c.Request.Context(), service.CreateJournalEntryInput{
		CompanyID:   companyID,
		EntryDate:   entryDate,
		Reference:   req.Reference,
		Description: req.Description,
		SourceType:  req.SourceType,
		SourceID:    sourceID,
		Memo:        req.Memo,
		CreatedBy:   userID,
		Lines:       lines,
	})
	if err != nil {
		if err == service.ErrUnbalancedEntry || err == service.ErrEmptyLines {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, entry)
}

func (h *JournalHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid journal entry ID")
		return
	}

	entry, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if entry.CompanyID != companyID {
		response.Forbidden(c, "no access")
		return
	}
	response.OK(c, entry)
}

func (h *JournalHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	entries, total, err := h.svc.List(c.Request.Context(), companyID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Paginated(c, entries, int(total), p.Page, p.Limit)
}

func (h *JournalHandler) Post(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid journal entry ID")
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.svc.Post(c.Request.Context(), id, userID); err != nil {
		if err == service.ErrJournalNotDraft || err == service.ErrPeriodClosed {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "posted"})
}

func (h *JournalHandler) Reverse(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid journal entry ID")
		return
	}

	userID := middleware.GetUserID(c)
	reversal, err := h.svc.Reverse(c.Request.Context(), id, userID)
	if err != nil {
		if err == service.ErrJournalNotPosted || err == service.ErrJournalAlreadyReversed {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, reversal)
}
