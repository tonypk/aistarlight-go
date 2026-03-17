package service

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// EISInvoice represents an invoice in BIR EIS CTC JSON format.
type EISInvoice struct {
	Header EISHeader `json:"header"`
	Seller EISParty  `json:"seller"`
	Buyer  EISParty  `json:"buyer"`
	Items  []EISItem `json:"items"`
	Totals EISTotals `json:"totals"`
}

// EISHeader contains invoice metadata.
type EISHeader struct {
	InvoiceNumber  string `json:"invoice_number"`
	InvoiceType    string `json:"invoice_type"`
	InvoiceDate    string `json:"invoice_date"`
	DueDate        string `json:"due_date,omitempty"`
	ReferenceNo    string `json:"reference_number,omitempty"`
	CurrencyCode   string `json:"currency_code"`
	TransmissionID string `json:"transmission_id,omitempty"`
}

// EISParty represents a seller or buyer.
type EISParty struct {
	Name           string `json:"name"`
	TIN            string `json:"tin"`
	Address        string `json:"address,omitempty"`
	BranchCode     string `json:"branch_code,omitempty"`
	Classification string `json:"classification,omitempty"`
}

// EISItem represents a line item.
type EISItem struct {
	LineNumber  int     `json:"line_number"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
	VATType     string  `json:"vat_type"`
	VATRate     float64 `json:"vat_rate"`
	VATAmount   float64 `json:"vat_amount"`
	Discount    float64 `json:"discount,omitempty"`
	ATCCode     string  `json:"atc_code,omitempty"`
}

// EISTotals contains invoice totals.
type EISTotals struct {
	Subtotal       float64 `json:"subtotal"`
	VATAmount      float64 `json:"vat_amount"`
	DiscountAmount float64 `json:"discount_amount"`
	TotalAmount    float64 `json:"total_amount"`
	VatableSales   float64 `json:"vatable_sales"`
	VatExemptSales float64 `json:"vat_exempt_sales"`
	ZeroRatedSales float64 `json:"zero_rated_sales"`
}

// buildEISInvoice converts a single invoice + items to an EISInvoice struct.
func buildEISInvoice(invoice sqlc.Invoice, items []sqlc.InvoiceItem, sellerName, sellerTIN string) EISInvoice {
	eis := EISInvoice{
		Header: EISHeader{
			InvoiceNumber: invoice.InvoiceNumber,
			InvoiceType:   mapInvoiceType(invoice.InvoiceType),
			InvoiceDate:   formatPGDate(invoice.InvoiceDate),
			DueDate:       formatPGDate(invoice.DueDate),
			CurrencyCode:  "PHP",
		},
		Seller: EISParty{
			Name: sellerName,
			TIN:  sellerTIN,
		},
		Buyer: EISParty{
			Name:    invoice.CustomerName,
			TIN:     derefStr(invoice.CustomerTin),
			Address: derefStr(invoice.CustomerAddress),
		},
		Totals: EISTotals{
			Subtotal:       numericToFloat(invoice.Subtotal),
			VATAmount:      numericToFloat(invoice.VatAmount),
			DiscountAmount: numericToFloat(invoice.DiscountAmount),
			TotalAmount:    numericToFloat(invoice.TotalAmount),
			VatableSales:   numericToFloat(invoice.VatableSales),
			VatExemptSales: numericToFloat(invoice.VatExemptSales),
			ZeroRatedSales: numericToFloat(invoice.ZeroRatedSales),
		},
	}

	if invoice.ReferenceNumber != nil {
		eis.Header.ReferenceNo = *invoice.ReferenceNumber
	}

	for _, item := range items {
		eis.Items = append(eis.Items, EISItem{
			LineNumber:  int(item.LineNumber),
			Description: item.Description,
			Quantity:    numericToFloat(item.Quantity),
			UnitPrice:   numericToFloat(item.UnitPrice),
			Amount:      numericToFloat(item.Amount),
			VATType:     item.VatType,
			VATRate:     numericToFloat(item.VatRate),
			VATAmount:   numericToFloat(item.VatAmount),
			Discount:    numericToFloat(item.Discount),
			ATCCode:     derefStr(item.AtcCode),
		})
	}

	return eis
}

// ExportEIS converts an invoice with items to BIR EIS JSON format.
func ExportEIS(invoice sqlc.Invoice, items []sqlc.InvoiceItem, sellerName, sellerTIN string) ([]byte, error) {
	eis := buildEISInvoice(invoice, items, sellerName, sellerTIN)
	data, err := json.MarshalIndent(eis, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal EIS JSON: %w", err)
	}
	return data, nil
}

// ExportEISBatch exports multiple invoices as a JSON array for batch EIS submission.
func ExportEISBatch(invoicesWithItems []InvoiceWithItems, sellerName, sellerTIN string) ([]byte, error) {
	batch := make([]EISInvoice, 0, len(invoicesWithItems))
	for _, inv := range invoicesWithItems {
		batch = append(batch, buildEISInvoice(inv.Invoice, inv.Items, sellerName, sellerTIN))
	}
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal EIS batch JSON: %w", err)
	}
	return data, nil
}

func mapInvoiceType(t string) string {
	switch t {
	case "sales":
		return "01" // BIR: Sales Invoice
	case "purchase":
		return "02"
	case "credit_note":
		return "03"
	case "debit_note":
		return "04"
	default:
		return "01"
	}
}

func formatPGDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
