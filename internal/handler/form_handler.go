package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// FormHandler handles form schema endpoints.
type FormHandler struct {
	reportSvc *service.ReportService
}

// NewFormHandler creates a form handler.
func NewFormHandler(reportSvc *service.ReportService) *FormHandler {
	return &FormHandler{reportSvc: reportSvc}
}

type formSummary struct {
	FormType  string `json:"form_type"`
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	Status    string `json:"status"`
}

var supportedForms = []formSummary{
	{FormType: "BIR_2550M", Name: "Monthly Value-Added Tax Declaration", Frequency: "monthly", Status: "active"},
	{FormType: "BIR_2550Q", Name: "Quarterly Value-Added Tax Return", Frequency: "quarterly", Status: "active"},
	{FormType: "BIR_1601C", Name: "Monthly Remittance of Withholding Tax on Compensation", Frequency: "monthly", Status: "active"},
	{FormType: "BIR_0619E", Name: "Monthly Remittance of Creditable Income Taxes Withheld (Expanded)", Frequency: "monthly", Status: "active"},
	{FormType: "BIR_1701", Name: "Annual Income Tax Return (Individuals)", Frequency: "annual", Status: "active"},
	{FormType: "BIR_1702", Name: "Annual Income Tax Return (Corporations)", Frequency: "annual", Status: "active"},
	{FormType: "BIR_2316", Name: "Certificate of Compensation Payment / Tax Withheld", Frequency: "annual", Status: "active"},
}

// formSchema maps form types to their detailed schemas.
var formSchemas = map[string]gin.H{
	"BIR_2550M": {
		"form_type": "BIR_2550M", "name": "Monthly Value-Added Tax Declaration",
		"frequency": "monthly", "version": 1,
		"schema_def": gin.H{
			"sections": []gin.H{
				{
					"id": "sales", "name": "Sales/Receipts",
					"fields": []gin.H{
						{"id": "vatable_sales", "line": "14A", "label": "Vatable Sales", "editable": true, "required": true},
						{"id": "vat_exempt_sales", "line": "14B", "label": "VAT-Exempt Sales", "editable": true},
						{"id": "zero_rated_sales", "line": "14C", "label": "Zero-Rated Sales", "editable": true},
					},
				},
				{
					"id": "purchases", "name": "Purchases",
					"fields": []gin.H{
						{"id": "domestic_purchases", "line": "18A", "label": "Domestic Purchases of Goods", "editable": true},
						{"id": "domestic_services", "line": "18B", "label": "Domestic Purchases of Services", "editable": true},
						{"id": "importation", "line": "18C", "label": "Importation of Goods", "editable": true},
					},
				},
				{
					"id": "tax", "name": "Tax Computation",
					"fields": []gin.H{
						{"id": "output_tax", "line": "15", "label": "Output Tax", "editable": false},
						{"id": "input_tax", "line": "19", "label": "Input Tax", "editable": false},
						{"id": "vat_payable", "line": "20", "label": "VAT Payable", "editable": false},
					},
				},
			},
		},
		"calculation_rules": gin.H{
			"output_tax":  "vatable_sales * 0.12",
			"input_tax":   "(domestic_purchases + domestic_services + importation) * 0.12",
			"vat_payable": "output_tax - input_tax",
		},
	},
	"BIR_2550Q": {
		"form_type": "BIR_2550Q", "name": "Quarterly Value-Added Tax Return",
		"frequency": "quarterly", "version": 1,
		"schema_def": gin.H{
			"sections": []gin.H{
				{"id": "sales", "name": "Sales/Receipts", "fields": []gin.H{
					{"id": "vatable_sales", "line": "14A", "label": "Vatable Sales", "editable": true, "required": true},
					{"id": "vat_exempt_sales", "line": "14B", "label": "VAT-Exempt Sales", "editable": true},
					{"id": "zero_rated_sales", "line": "14C", "label": "Zero-Rated Sales", "editable": true},
				}},
				{"id": "purchases", "name": "Purchases", "fields": []gin.H{
					{"id": "domestic_purchases_goods", "line": "18A", "label": "Domestic Purchases of Goods", "editable": true},
					{"id": "domestic_purchases_services", "line": "18B", "label": "Domestic Purchases of Services", "editable": true},
				}},
			},
		},
		"calculation_rules": gin.H{
			"output_tax":  "vatable_sales * 0.12",
			"input_tax":   "(domestic_purchases_goods + domestic_purchases_services) * 0.12",
			"vat_payable": "output_tax - input_tax",
		},
	},
	"BIR_1601C": {
		"form_type": "BIR_1601C", "name": "Monthly Remittance of Withholding Tax on Compensation",
		"frequency": "monthly", "version": 1,
		"schema_def": gin.H{
			"sections": []gin.H{
				{"id": "compensation", "name": "Compensation", "fields": []gin.H{
					{"id": "total_compensation", "line": "1", "label": "Total Amount of Compensation", "editable": true, "required": true},
					{"id": "non_taxable", "line": "2", "label": "Non-Taxable/Exempt Compensation", "editable": true},
					{"id": "taxable_compensation", "line": "3", "label": "Taxable Compensation", "editable": false},
					{"id": "tax_withheld", "line": "4", "label": "Tax Required to be Withheld", "editable": false},
				}},
			},
		},
		"calculation_rules": gin.H{
			"taxable_compensation": "total_compensation - non_taxable",
		},
	},
}

// List handles GET /api/v1/forms.
func (h *FormHandler) List(c *gin.Context) {
	response.OK(c, supportedForms)
}

// GetSchema handles GET /api/v1/forms/:formType.
func (h *FormHandler) GetSchema(c *gin.Context) {
	formType := c.Param("formType")

	schema, ok := formSchemas[formType]
	if !ok {
		// Return a generic schema for forms without detailed definitions
		for _, f := range supportedForms {
			if f.FormType == formType {
				response.OK(c, gin.H{
					"form_type":         f.FormType,
					"name":              f.Name,
					"frequency":         f.Frequency,
					"version":           1,
					"schema_def":        gin.H{"sections": []gin.H{}},
					"calculation_rules": gin.H{},
				})
				return
			}
		}
		response.NotFound(c, "form type not found")
		return
	}

	response.OK(c, schema)
}
