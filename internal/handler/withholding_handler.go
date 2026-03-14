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
	svc    *service.WithholdingService
	vendor *service.VendorService
}

// NewWithholdingHandler creates a withholding handler.
func NewWithholdingHandler(svc *service.WithholdingService, vendor *service.VendorService) *WithholdingHandler {
	return &WithholdingHandler{svc: svc, vendor: vendor}
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
	VendorID     string `json:"vendor_id" binding:"required"`
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

	vendorID, err := uuid.Parse(req.VendorID)
	if err != nil {
		response.BadRequest(c, "invalid vendor_id")
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
		companyID, vendorID, sessionID,
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
	switch jurisdiction {
	case "SG":
		response.OK(c, service.ListSGWHTRates())
	case "LK":
		response.OK(c, service.ListLKWHTRates())
	default:
		response.OK(c, service.ListEWTRates())
	}
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

// ListVendors handles GET /api/v1/withholding/vendors (+ /suppliers compat).
func (h *WithholdingHandler) ListVendors(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)
	search := c.Query("search")

	vendors, total, err := h.vendor.List(c.Request.Context(), companyID, p.Limit, p.Offset, search)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, vendors, int(total), p.Page, p.Limit)
}

// CreateVendor handles POST /api/v1/withholding/vendors (+ /suppliers compat).
func (h *WithholdingHandler) CreateVendor(c *gin.Context) {
	var req service.CreateVendorInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	vendor, err := h.vendor.Create(c.Request.Context(), companyID, req)
	if err != nil {
		if err.Error() == fmt.Sprintf("vendor with TIN %s already exists", req.TIN) {
			response.Conflict(c, err.Error())
		} else {
			response.InternalError(c, err.Error())
		}
		return
	}

	response.Created(c, vendor)
}

// UpdateVendor handles PATCH /api/v1/withholding/vendors/:id (+ /suppliers compat).
func (h *WithholdingHandler) UpdateVendor(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid vendor id")
		return
	}

	var req service.CreateVendorInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	vendor, err := h.vendor.Update(c.Request.Context(), id, companyID, req)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, vendor)
}

// DeleteVendor handles DELETE /api/v1/withholding/vendors/:id (+ /suppliers compat).
func (h *WithholdingHandler) DeleteVendor(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid vendor id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.vendor.Delete(c.Request.Context(), id, companyID); err != nil {
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

	// Get vendor info
	v, _ := h.vendor.GetByID(c.Request.Context(), cert.VendorID)

	format := c.DefaultQuery("format", "pdf")
	if format == "csv" {
		h.downloadCertificateCSV(c, cert, v)
		return
	}

	// PDF output (default)
	companyID := middleware.GetCompanyID(c)
	company, _ := h.svc.GetCompanyForPDF(c.Request.Context(), companyID)

	vendorRow, _ := h.svc.GetVendorByID(c.Request.Context(), cert.VendorID)

	pdfBytes, err := h.svc.GenerateCertificatePDF(c.Request.Context(), cert, company, &vendorRow)
	if err != nil {
		response.InternalError(c, "failed to generate PDF: "+err.Error())
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=BIR2307_%s_%s.pdf", cert.Period, cert.Quarter))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func (h *WithholdingHandler) downloadCertificateCSV(c *gin.Context, cert *sqlc.WithholdingCertificate, v *service.VendorResponse) {
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

	if v != nil {
		_ = w.Write([]string{"Vendor Name", v.Name})
		_ = w.Write([]string{"Vendor TIN", v.TIN})
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
	_ = w.Write([]string{"Vendor TIN", "Vendor Name", "ATC Code", "Income Type", "Income Amount", "EWT Rate (%)", "Tax Withheld", "Period", "Quarter"})

	for _, cert := range certs {
		if period != "all" && cert.Period != period {
			continue
		}

		vendorName := ""
		vendorTIN := ""
		if v, err := h.vendor.GetByID(c.Request.Context(), cert.VendorID); err == nil {
			vendorName = v.Name
			vendorTIN = v.TIN
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

		_ = w.Write([]string{vendorTIN, vendorName, cert.AtcCode, cert.IncomeType, incAmt, ewtRate, taxWithheld, cert.Period, cert.Quarter})
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
