package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// NotificationHandler handles notification endpoints.
type NotificationHandler struct {
	svc *service.NotificationService
}

// NewNotificationHandler creates a NotificationHandler.
func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// List handles GET /api/v1/notifications
func (h *NotificationHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	notifications, total, err := h.svc.ListForCompany(c.Request.Context(), companyID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, notifications, total, p.Page, p.Limit)
}

// UnreadCount handles GET /api/v1/notifications/unread-count
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	count, err := h.svc.CountUnread(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"unread_count": count})
}

// MarkRead handles PATCH /api/v1/notifications/:id/read
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid notification ID")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.svc.MarkRead(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"marked_read": true})
}

// MarkAllRead handles POST /api/v1/notifications/mark-all-read
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := h.svc.MarkAllRead(c.Request.Context(), companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"marked_all_read": true})
}
