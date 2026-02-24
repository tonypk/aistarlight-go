package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type ReportHandler struct {
	svc        *service.ReportService
	companySvc *service.CompanyService
}

func NewReportHandler(svc *service.ReportService, companySvc *service.CompanyService) *ReportHandler {
	return &ReportHandler{svc: svc, companySvc: companySvc}
}

type createReportRequest struct {
	ReportType string                 `json:"report_type" binding:"required"`
	Period     string                 `json:"period" binding:"required"`
	InputData  map[string]interface{} `json:"input_data" binding:"required"`
}

// Create handles POST /api/v1/reports
func (h *ReportHandler) Create(c *gin.Context) {
	var req createReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	report, err := h.svc.Create(c.Request.Context(), service.CreateReportInput{
		CompanyID:  companyID,
		ReportType: req.ReportType,
		Period:     req.Period,
		InputData:  req.InputData,
		CreatedBy:  userID,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, report)
}

// Get handles GET /api/v1/reports/:id
func (h *ReportHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	report, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	// Verify company access
	companyID := middleware.GetCompanyID(c)
	if report.CompanyID != companyID {
		response.Forbidden(c, "no access to this report")
		return
	}

	response.OK(c, report)
}

// List handles GET /api/v1/reports
func (h *ReportHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	reportType := c.Query("report_type")

	var (
		reports []domain.Report
		total   int
		err     error
	)

	if reportType != "" {
		reports, total, err = h.svc.ListByCompanyAndType(c.Request.Context(), companyID, reportType, p)
	} else {
		reports, total, err = h.svc.ListByCompany(c.Request.Context(), companyID, p)
	}

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, reports, total, p.Page, p.Limit)
}

type updateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateStatus handles PATCH /api/v1/reports/:id/status
func (h *ReportHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	report, err := h.svc.UpdateStatus(c.Request.Context(), id, domain.ReportStatus(req.Status), userID)
	if err != nil {
		switch err {
		case service.ErrReportNotFound:
			response.NotFound(c, err.Error())
		case service.ErrVersionConflict:
			response.Conflict(c, err.Error())
		default:
			response.BadRequest(c, err.Error())
		}
		return
	}

	response.OK(c, report)
}

// Recalculate handles POST /api/v1/reports/:id/recalculate
func (h *ReportHandler) Recalculate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	userID := middleware.GetUserID(c)

	report, err := h.svc.Recalculate(c.Request.Context(), id, userID)
	if err != nil {
		switch err {
		case service.ErrReportNotFound:
			response.NotFound(c, err.Error())
		case service.ErrReportNotEditable:
			response.BadRequest(c, err.Error())
		case service.ErrVersionConflict:
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err.Error())
		}
		return
	}

	response.OK(c, report)
}

type overrideRequest struct {
	Overrides map[string]string `json:"overrides" binding:"required"`
	Notes     *string           `json:"notes"`
	Version   int32             `json:"version" binding:"required"`
}

// ApplyOverrides handles POST /api/v1/reports/:id/overrides
func (h *ReportHandler) ApplyOverrides(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	var req overrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	report, err := h.svc.ApplyOverrides(c.Request.Context(), service.OverrideInput{
		ReportID:  id,
		UserID:    userID,
		Overrides: req.Overrides,
		Notes:     req.Notes,
		Version:   req.Version,
	})
	if err != nil {
		switch err {
		case service.ErrReportNotFound:
			response.NotFound(c, err.Error())
		case service.ErrReportNotEditable:
			response.BadRequest(c, err.Error())
		case service.ErrVersionConflict:
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err.Error())
		}
		return
	}

	response.OK(c, report)
}

// Delete handles DELETE /api/v1/reports/:id
func (h *ReportHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		if err == service.ErrReportNotFound {
			response.NotFound(c, err.Error())
			return
		}
		response.BadRequest(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": true}})
}

// Calculate handles POST /api/v1/reports/calculate (preview calculation without saving)
func (h *ReportHandler) Calculate(c *gin.Context) {
	var req createReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := service.CalculateReport(req.ReportType, req.InputData)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, result)
}

// DownloadPDF handles GET /api/v1/reports/:id/pdf
func (h *ReportHandler) DownloadPDF(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	report, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if report.CompanyID != companyID {
		response.Forbidden(c, "no access to this report")
		return
	}

	// Parse calculated data for PDF generation.
	var calcData map[string]string
	if err := json.Unmarshal(report.CalculatedData, &calcData); err != nil {
		response.InternalError(c, "invalid report data")
		return
	}
	calcData["period"] = report.Period

	// Get company info.
	company, err := h.companySvc.GetByID(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "company not found")
		return
	}

	companyInfo := service.CompanyInfo{
		CompanyName: company.CompanyName,
	}
	if company.TINNumber != nil {
		companyInfo.TINNumber = *company.TINNumber
	}
	if company.RDOCode != nil {
		companyInfo.RDOCode = *company.RDOCode
	}

	filename := fmt.Sprintf("%s_%s.pdf", report.ReportType, report.Period)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := service.GeneratePDFReport(c.Writer, report.ReportType, calcData, companyInfo); err != nil {
		response.InternalError(c, "PDF generation failed")
		return
	}
}

// DownloadCSV handles GET /api/v1/reports/:id/csv
func (h *ReportHandler) DownloadCSV(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid report ID")
		return
	}

	report, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if report.CompanyID != companyID {
		response.Forbidden(c, "no access to this report")
		return
	}

	var calcData map[string]string
	if err := json.Unmarshal(report.CalculatedData, &calcData); err != nil {
		response.InternalError(c, "invalid report data")
		return
	}

	filename := fmt.Sprintf("%s_%s.csv", report.ReportType, report.Period)
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := service.GenerateCSVExport(c.Writer, calcData); err != nil {
		response.InternalError(c, "CSV generation failed")
		return
	}
}
