package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type OrgHandler struct {
	org *service.OrgService
}

func NewOrgHandler(org *service.OrgService) *OrgHandler {
	return &OrgHandler{org: org}
}

type createOrgRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug"`
}

func (h *OrgHandler) Create(c *gin.Context) {
	var req createOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	org, err := h.org.Create(c.Request.Context(), service.CreateOrgInput{
		Name:    req.Name,
		Slug:    req.Slug,
		OwnerID: userID,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, org)
}

func (h *OrgHandler) Get(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	org, err := h.org.GetByID(c.Request.Context(), orgID)
	if err != nil {
		response.NotFound(c, "organization not found")
		return
	}

	response.OK(c, org)
}

type updateOrgRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

func (h *OrgHandler) Update(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	var req updateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	org, err := h.org.Update(c.Request.Context(), orgID, req.Name, req.Slug)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, org)
}

func (h *OrgHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	p := pagination.Parse(c)

	orgs, total, err := h.org.ListByUser(c.Request.Context(), userID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, orgs, total, p.Page, p.Limit)
}

func (h *OrgHandler) ListMembers(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	members, err := h.org.ListMembers(c.Request.Context(), orgID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, members)
}

type addMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role" binding:"required"`
}

func (h *OrgHandler) AddMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	err = h.org.AddMember(c.Request.Context(), orgID, req.UserID, domain.OrgRole(req.Role))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, gin.H{"message": "member added"})
}

type updateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

func (h *OrgHandler) UpdateMemberRole(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.BadRequest(c, "invalid user ID")
		return
	}

	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	err = h.org.UpdateMemberRole(c.Request.Context(), orgID, userID, domain.OrgRole(req.Role))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "role updated"})
}

func (h *OrgHandler) RemoveMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.BadRequest(c, "invalid user ID")
		return
	}

	err = h.org.RemoveMember(c.Request.Context(), orgID, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "member removed"})
}
