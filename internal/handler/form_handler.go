package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// FormHandler handles form schema endpoints.
type FormHandler struct {
	schemaSvc *service.FormSchemaService
}

// NewFormHandler creates a form handler.
func NewFormHandler(schemaSvc *service.FormSchemaService) *FormHandler {
	return &FormHandler{schemaSvc: schemaSvc}
}

// List handles GET /api/v1/forms.
func (h *FormHandler) List(c *gin.Context) {
	forms, err := h.schemaSvc.ListForms(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, forms)
}

// GetSchema handles GET /api/v1/forms/:formType.
func (h *FormHandler) GetSchema(c *gin.Context) {
	formType := c.Param("formType")

	schema, err := h.schemaSvc.GetSchema(c.Request.Context(), formType)
	if err != nil {
		response.NotFound(c, "form type not found")
		return
	}

	response.OK(c, schema)
}
