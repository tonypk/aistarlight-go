package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// TaskHandler handles async task status endpoints.
type TaskHandler struct {
	svc *service.TaskService
}

// NewTaskHandler creates a task handler.
func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// Get handles GET /api/v1/tasks/:id — poll task status.
func (h *TaskHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid task id")
		return
	}

	task, err := h.svc.GetTask(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, task)
}

// List handles GET /api/v1/tasks — list tasks for company.
func (h *TaskHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	tasks, total, err := h.svc.ListTasks(c.Request.Context(), companyID, int32(p.Limit), int32(p.Offset))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, tasks, int(total), p.Page, p.Limit)
}
