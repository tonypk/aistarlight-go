package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type QBOHandler struct {
	svc        *service.QBOService
	accountSvc *service.AccountService
}

func NewQBOHandler(svc *service.QBOService, accountSvc *service.AccountService) *QBOHandler {
	return &QBOHandler{svc: svc, accountSvc: accountSvc}
}

// AuthURL returns the QBO OAuth authorization URL.
func (h *QBOHandler) AuthURL(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	url := h.svc.AuthURL(companyID)
	response.OK(c, gin.H{"auth_url": url})
}

// Callback handles the QBO OAuth callback.
func (h *QBOHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	realmID := c.Query("realmId")
	if code == "" || realmID == "" {
		response.BadRequest(c, "missing code or realmId")
		return
	}

	companyID := middleware.GetCompanyID(c)
	conn, err := h.svc.HandleCallback(c.Request.Context(), companyID, code, realmID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, conn)
}

// Status returns the QBO connection status (no tokens).
func (h *QBOHandler) Status(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := h.svc.Status(c.Request.Context(), companyID)
	if err != nil {
		if err == service.ErrQBONotConnected {
			response.OK(c, gin.H{"connected": false})
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{
		"connected":        true,
		"realm_id":         conn.RealmID,
		"is_active":        conn.IsActive,
		"last_sync_at":     conn.LastSyncAt,
		"last_sync_status": conn.LastSyncStatus,
		"token_expiry":     conn.TokenExpiry,
		"refresh_expiry":   conn.RefreshExpiry,
	})
}

// Disconnect deactivates the QBO connection.
func (h *QBOHandler) Disconnect(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if err := h.svc.Disconnect(c.Request.Context(), companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "disconnected"})
}

// SyncAccounts triggers a COA sync from QBO.
func (h *QBOHandler) SyncAccounts(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	log, err := h.svc.SyncAccounts(c.Request.Context(), companyID, h.accountSvc)
	if err != nil {
		if err == service.ErrQBONotConnected || err == service.ErrQBOTokenExpired {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, log)
}

// SyncLogs returns paginated sync logs.
func (h *QBOHandler) SyncLogs(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	logs, total, err := h.svc.ListSyncLogs(c.Request.Context(), companyID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Paginated(c, logs, int(total), p.Page, p.Limit)
}
