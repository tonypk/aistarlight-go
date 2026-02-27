package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// WithholdingHandler handles EWT endpoints.
type WithholdingHandler struct {
	svc      *service.WithholdingService
	supplier *service.SupplierService
}

// NewWithholdingHandler creates a withholding handler.
func NewWithholdingHandler(svc *service.WithholdingService, supplier *service.SupplierService) *WithholdingHandler {
	return &WithholdingHandler{svc: svc, supplier: supplier}
}

type classifyEWTRequest struct {
	Transactions []map[string]interface{} `json:"transactions" binding:"required"`
}

// Classify handles POST /api/v1/withholding/classify.
func (h *WithholdingHandler) Classify(c *gin.Context) {
	var req classifyEWTRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	results, err := h.svc.ClassifyEWTTransactions(c.Request.Context(), req.Transactions, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, results)
}

type createCertificateRequest struct {
	SupplierID   string `json:"supplier_id" binding:"required"`
	SessionID    *string `json:"session_id"`
	Period       string `json:"period" binding:"required"`
	Quarter      string `json:"quarter" binding:"required"`
	ATCCode      string `json:"atc_code" binding:"required"`
	IncomeAmount string `json:"income_amount" binding:"required"`
	EWTRate      string `json:"ewt_rate" binding:"required"`
	TaxWithheld  string `json:"tax_withheld" binding:"required"`
}

// CreateCertificate handles POST /api/v1/withholding/certificates.
func (h *WithholdingHandler) CreateCertificate(c *gin.Context) {
	var req createCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	supplierID, err := uuid.Parse(req.SupplierID)
	if err != nil {
		response.BadRequest(c, "invalid supplier_id")
		return
	}

	var sessionID *uuid.UUID
	if req.SessionID != nil {
		sid, err := uuid.Parse(*req.SessionID)
		if err == nil {
			sessionID = &sid
		}
	}

	incomeAmount, _ := decimal.NewFromString(req.IncomeAmount)
	ewtRate, _ := decimal.NewFromString(req.EWTRate)
	taxWithheld, _ := decimal.NewFromString(req.TaxWithheld)

	companyID := middleware.GetCompanyID(c)

	cert, err := h.svc.CreateCertificate(
		c.Request.Context(),
		companyID, supplierID, sessionID,
		req.Period, req.Quarter, req.ATCCode,
		incomeAmount, ewtRate, taxWithheld,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, cert)
}

// ListCertificates handles GET /api/v1/withholding/certificates.
func (h *WithholdingHandler) ListCertificates(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	certs, total, err := h.svc.ListCertificates(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, certs, int(total), p.Page, p.Limit)
}

// GetCertificate handles GET /api/v1/withholding/certificates/:id.
func (h *WithholdingHandler) GetCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.GetCertificate(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, cert)
}

// Rates handles GET /api/v1/withholding/rates and /ewt-rates.
func (h *WithholdingHandler) Rates(c *gin.Context) {
	jurisdiction := middleware.GetJurisdiction(c)
	if jurisdiction == "SG" {
		response.OK(c, service.ListSGWHTRates())
		return
	}
	response.OK(c, service.ListEWTRates())
}

// EWTSummary handles GET /api/v1/withholding/ewt-summary.
func (h *WithholdingHandler) EWTSummary(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	certs, total, err := h.svc.ListCertificates(c.Request.Context(), companyID, 1000, 0)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{
		"total_certificates": total,
		"certificates_count": len(certs),
		"period":             c.Query("period"),
	})
}

// ListSuppliers handles GET /api/v1/withholding/suppliers.
func (h *WithholdingHandler) ListSuppliers(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)
	search := c.Query("search")

	suppliers, total, err := h.supplier.List(c.Request.Context(), companyID, p.Limit, p.Offset, search)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, suppliers, int(total), p.Page, p.Limit)
}

// CreateSupplier handles POST /api/v1/withholding/suppliers.
func (h *WithholdingHandler) CreateSupplier(c *gin.Context) {
	var req service.CreateSupplierInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	supplier, err := h.supplier.Create(c.Request.Context(), companyID, req)
	if err != nil {
		if err.Error() == fmt.Sprintf("supplier with TIN %s already exists", req.TIN) {
			response.Conflict(c, err.Error())
		} else {
			response.InternalError(c, err.Error())
		}
		return
	}

	response.Created(c, supplier)
}

// UpdateSupplier handles PATCH /api/v1/withholding/suppliers/:id.
func (h *WithholdingHandler) UpdateSupplier(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid supplier id")
		return
	}

	var req service.CreateSupplierInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	supplier, err := h.supplier.Update(c.Request.Context(), id, companyID, req)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, supplier)
}

// DeleteSupplier handles DELETE /api/v1/withholding/suppliers/:id.
func (h *WithholdingHandler) DeleteSupplier(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid supplier id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.supplier.Delete(c.Request.Context(), id, companyID); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

// DownloadCertificate handles GET /api/v1/withholding/certificates/:id/download.
// Default: PDF. Use ?format=csv for CSV export.
func (h *WithholdingHandler) DownloadCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.GetCertificate(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	// Get supplier info
	sup, _ := h.supplier.GetByID(c.Request.Context(), cert.SupplierID)

	format := c.DefaultQuery("format", "pdf")
	if format == "csv" {
		h.downloadCertificateCSV(c, cert, sup)
		return
	}

	// PDF output (default)
	companyID := middleware.GetCompanyID(c)
	company, _ := h.svc.GetCompanyForPDF(c.Request.Context(), companyID)

	supplierRow, _ := h.svc.GetSupplierByID(c.Request.Context(), cert.SupplierID)

	pdfBytes, err := h.svc.GenerateCertificatePDF(c.Request.Context(), cert, company, &supplierRow)
	if err != nil {
		response.InternalError(c, "failed to generate PDF: "+err.Error())
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=BIR2307_%s_%s.pdf", cert.Period, cert.Quarter))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func (h *WithholdingHandler) downloadCertificateCSV(c *gin.Context, cert *sqlc.WithholdingCertificate, sup *service.SupplierResponse) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=BIR2307_%s_%s.csv", cert.Period, cert.Quarter))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Field", "Value"})
	_ = w.Write([]string{"Certificate ID", cert.ID.String()})
	_ = w.Write([]string{"Period", cert.Period})
	_ = w.Write([]string{"Quarter", cert.Quarter})
	_ = w.Write([]string{"ATC Code", cert.AtcCode})
	_ = w.Write([]string{"Income Type", cert.IncomeType})

	incAmt := "0.00"
	if f, err := cert.IncomeAmount.Float64Value(); err == nil {
		incAmt = fmt.Sprintf("%.2f", f.Float64)
	}
	_ = w.Write([]string{"Income Amount", incAmt})

	ewtRate := "0.00"
	if f, err := cert.EwtRate.Float64Value(); err == nil {
		ewtRate = fmt.Sprintf("%.2f", f.Float64)
	}
	_ = w.Write([]string{"EWT Rate (%)", ewtRate})

	taxWithheld := "0.00"
	if f, err := cert.TaxWithheld.Float64Value(); err == nil {
		taxWithheld = fmt.Sprintf("%.2f", f.Float64)
	}
	_ = w.Write([]string{"Tax Withheld", taxWithheld})
	_ = w.Write([]string{"Status", cert.Status})

	if sup != nil {
		_ = w.Write([]string{"Supplier Name", sup.Name})
		_ = w.Write([]string{"Supplier TIN", sup.TIN})
	}

	w.Flush()
	c.Status(http.StatusOK)
}

// GetSAWT handles GET /api/v1/withholding/sawt.
func (h *WithholdingHandler) GetSAWT(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	period := c.Query("period")

	certs, total, err := h.svc.ListCertificates(c.Request.Context(), companyID, 1000, 0)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Filter by period if specified, and aggregate by ATC code
	type sawtEntry struct {
		ATCCode      string  `json:"atc_code"`
		IncomeType   string  `json:"income_type"`
		TotalIncome  float64 `json:"total_income"`
		TotalTax     float64 `json:"total_tax"`
		EWTRate      float64 `json:"ewt_rate"`
		Certificates int     `json:"certificates"`
	}

	byATC := make(map[string]*sawtEntry)
	for _, cert := range certs {
		if period != "" && cert.Period != period {
			continue
		}

		entry, ok := byATC[cert.AtcCode]
		if !ok {
			rate := 0.0
			if f, err := cert.EwtRate.Float64Value(); err == nil {
				rate = f.Float64
			}
			entry = &sawtEntry{
				ATCCode:    cert.AtcCode,
				IncomeType: cert.IncomeType,
				EWTRate:    rate,
			}
			byATC[cert.AtcCode] = entry
		}

		if f, err := cert.IncomeAmount.Float64Value(); err == nil {
			entry.TotalIncome += f.Float64
		}
		if f, err := cert.TaxWithheld.Float64Value(); err == nil {
			entry.TotalTax += f.Float64
		}
		entry.Certificates++
	}

	entries := make([]sawtEntry, 0, len(byATC))
	for _, e := range byATC {
		entries = append(entries, *e)
	}

	response.OK(c, gin.H{
		"entries":              entries,
		"period":               period,
		"total_certificates":   total,
		"total_income":         sumField(entries, func(e sawtEntry) float64 { return e.TotalIncome }),
		"total_tax_withheld":   sumField(entries, func(e sawtEntry) float64 { return e.TotalTax }),
	})
}

// DownloadSAWT handles GET /api/v1/withholding/sawt/download.
func (h *WithholdingHandler) DownloadSAWT(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	period := c.DefaultQuery("period", "all")

	certs, _, err := h.svc.ListCertificates(c.Request.Context(), companyID, 10000, 0)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=SAWT_%s.csv", period))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Supplier TIN", "Supplier Name", "ATC Code", "Income Type", "Income Amount", "EWT Rate (%)", "Tax Withheld", "Period", "Quarter"})

	for _, cert := range certs {
		if period != "all" && cert.Period != period {
			continue
		}

		supName := ""
		supTIN := ""
		if sup, err := h.supplier.GetByID(c.Request.Context(), cert.SupplierID); err == nil {
			supName = sup.Name
			supTIN = sup.TIN
		}

		incAmt := "0.00"
		if f, err := cert.IncomeAmount.Float64Value(); err == nil {
			incAmt = fmt.Sprintf("%.2f", f.Float64)
		}
		ewtRate := "0.00"
		if f, err := cert.EwtRate.Float64Value(); err == nil {
			ewtRate = fmt.Sprintf("%.2f", f.Float64)
		}
		taxWithheld := "0.00"
		if f, err := cert.TaxWithheld.Float64Value(); err == nil {
			taxWithheld = fmt.Sprintf("%.2f", f.Float64)
		}

		_ = w.Write([]string{supTIN, supName, cert.AtcCode, cert.IncomeType, incAmt, ewtRate, taxWithheld, cert.Period, cert.Quarter})
	}

	w.Flush()
	c.Status(http.StatusOK)
}

func sumField[T any](items []T, fn func(T) float64) float64 {
	var total float64
	for _, item := range items {
		total += fn(item)
	}
	return total
}
