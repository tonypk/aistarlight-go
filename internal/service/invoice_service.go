package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// InvoiceService handles invoice CRUD and EIS operations.
type InvoiceService struct {
	q *sqlc.Queries
}

// NewInvoiceService creates an InvoiceService.
func NewInvoiceService(q *sqlc.Queries) *InvoiceService {
	return &InvoiceService{q: q}
}

// CreateInvoiceRequest is the input for creating an invoice.
type CreateInvoiceRequest struct {
	InvoiceNumber   string          `json:"invoice_number"`
	InvoiceType     string          `json:"invoice_type"`
	CustomerName    string          `json:"customer_name"`
	CustomerTIN     string          `json:"customer_tin"`
	CustomerAddress string          `json:"customer_address"`
	InvoiceDate     string          `json:"invoice_date"`
	DueDate         string          `json:"due_date"`
	ReferenceNumber string          `json:"reference_number"`
	Notes           string          `json:"notes"`
	VendorID        *uuid.UUID      `json:"vendor_id"`
	Items           []InvoiceItemIn `json:"items"`
}

// InvoiceItemIn is the input for an invoice line item.
type InvoiceItemIn struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	VATType     string  `json:"vat_type"`
	VATRate     float64 `json:"vat_rate"`
	Discount    float64 `json:"discount"`
	ATCCode     string  `json:"atc_code"`
}

// InvoiceWithItems wraps an invoice with its line items.
type InvoiceWithItems struct {
	sqlc.Invoice `json:"invoice"`
	Items        []sqlc.InvoiceItem `json:"items"`
}

// Create creates an invoice with line items.
func (s *InvoiceService) Create(ctx context.Context, companyID, userID uuid.UUID, req CreateInvoiceRequest) (*InvoiceWithItems, error) {
	invoiceType := req.InvoiceType
	if invoiceType == "" {
		invoiceType = "sales"
	}

	invoiceDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		return nil, fmt.Errorf("invalid invoice_date: %w", err)
	}

	var dueDate pgtype.Date
	if req.DueDate != "" {
		d, err := time.Parse("2006-01-02", req.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		dueDate = pgtype.Date{Time: d, Valid: true}
	}

	// Auto-generate invoice number if not provided
	invoiceNumber := req.InvoiceNumber
	if invoiceNumber == "" {
		nextNum, err := s.q.GetNextInvoiceNumber(ctx, companyID)
		if err != nil {
			return nil, fmt.Errorf("get next invoice number: %w", err)
		}
		invoiceNumber = fmt.Sprintf("INV-%06d", nextNum)
	}

	// Calculate totals from items
	var subtotal, vatAmount, discountTotal float64
	var vatableSales, vatExemptSales, zeroRatedSales float64

	for _, item := range req.Items {
		qty := item.Quantity
		if qty == 0 {
			qty = 1
		}
		lineAmount := qty * item.UnitPrice
		lineDiscount := item.Discount
		lineNet := lineAmount - lineDiscount
		vatRate := item.VATRate
		if vatRate == 0 && item.VATType == "vatable" {
			vatRate = 12.0
		}
		var lineVAT float64
		if item.VATType == "vatable" {
			lineVAT = lineNet * vatRate / 100
			vatableSales += lineNet
		} else if item.VATType == "zero_rated" {
			zeroRatedSales += lineNet
		} else {
			vatExemptSales += lineNet
		}
		subtotal += lineNet
		vatAmount += lineVAT
		discountTotal += lineDiscount
	}

	totalAmount := subtotal + vatAmount

	invoice, err := s.q.CreateInvoice(ctx, sqlc.CreateInvoiceParams{
		CompanyID:       companyID,
		InvoiceNumber:   invoiceNumber,
		InvoiceType:     invoiceType,
		Status:          "draft",
		CustomerName:    req.CustomerName,
		CustomerTin:     strPtrOrNil(req.CustomerTIN),
		CustomerAddress: strPtrOrNil(req.CustomerAddress),
		InvoiceDate:     pgtype.Date{Time: invoiceDate, Valid: true},
		DueDate:         dueDate,
		Subtotal:        floatToNum(subtotal),
		VatAmount:       floatToNum(vatAmount),
		DiscountAmount:  floatToNum(discountTotal),
		TotalAmount:     floatToNum(totalAmount),
		VatableSales:    floatToNum(vatableSales),
		VatExemptSales:  floatToNum(vatExemptSales),
		ZeroRatedSales:  floatToNum(zeroRatedSales),
		ReferenceNumber: strPtrOrNil(req.ReferenceNumber),
		Notes:           strPtrOrNil(req.Notes),
		VendorID:        uuidToPG(req.VendorID),
		CreatedBy:       pgtype.UUID{Bytes: userID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}

	// Create line items
	var items []sqlc.InvoiceItem
	for i, item := range req.Items {
		qty := item.Quantity
		if qty == 0 {
			qty = 1
		}
		lineAmount := qty * item.UnitPrice
		lineNet := lineAmount - item.Discount
		vatRate := item.VATRate
		if vatRate == 0 && item.VATType == "vatable" {
			vatRate = 12.0
		}
		var lineVAT float64
		if item.VATType == "vatable" {
			lineVAT = lineNet * vatRate / 100
		}

		invItem, err := s.q.CreateInvoiceItem(ctx, sqlc.CreateInvoiceItemParams{
			InvoiceID:   invoice.ID,
			LineNumber:  int32(i + 1),
			Description: item.Description,
			Quantity:    floatToNum(qty),
			UnitPrice:   floatToNum(item.UnitPrice),
			Amount:      floatToNum(lineNet),
			VatType:     item.VATType,
			VatRate:     floatToNum(vatRate),
			VatAmount:   floatToNum(lineVAT),
			Discount:    floatToNum(item.Discount),
			AtcCode:     strPtrOrNil(item.ATCCode),
		})
		if err != nil {
			return nil, fmt.Errorf("create invoice item %d: %w", i+1, err)
		}
		items = append(items, invItem)
	}

	return &InvoiceWithItems{Invoice: invoice, Items: items}, nil
}

// Get retrieves an invoice with its items.
func (s *InvoiceService) Get(ctx context.Context, companyID, invoiceID uuid.UUID) (*InvoiceWithItems, error) {
	invoice, err := s.q.GetInvoiceByID(ctx, sqlc.GetInvoiceByIDParams{
		ID:        invoiceID,
		CompanyID: companyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("invoice not found: %w", err)
		}
		return nil, fmt.Errorf("get invoice: %w", err)
	}

	items, err := s.q.ListInvoiceItems(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list invoice items: %w", err)
	}

	return &InvoiceWithItems{Invoice: invoice, Items: items}, nil
}

// List returns paginated invoices.
func (s *InvoiceService) List(ctx context.Context, companyID uuid.UUID, limit, offset int32, status, invoiceType string) ([]sqlc.Invoice, int64, error) {
	invoices, err := s.q.ListInvoicesByCompany(ctx, sqlc.ListInvoicesByCompanyParams{
		CompanyID: companyID,
		Limit:     limit,
		Offset:    offset,
		Column4:   status,
		Column5:   invoiceType,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list invoices: %w", err)
	}

	count, err := s.q.CountInvoicesByCompany(ctx, sqlc.CountInvoicesByCompanyParams{
		CompanyID: companyID,
		Column2:   status,
		Column3:   invoiceType,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count invoices: %w", err)
	}

	return invoices, count, nil
}

// UpdateStatus updates the invoice status.
func (s *InvoiceService) UpdateStatus(ctx context.Context, companyID, invoiceID uuid.UUID, status string) error {
	return s.q.UpdateInvoiceStatus(ctx, sqlc.UpdateInvoiceStatusParams{
		ID:        invoiceID,
		CompanyID: companyID,
		Status:    status,
	})
}

// Delete removes an invoice and its items. Only draft invoices can be deleted.
func (s *InvoiceService) Delete(ctx context.Context, companyID, invoiceID uuid.UUID) error {
	inv, err := s.q.GetInvoiceByID(ctx, sqlc.GetInvoiceByIDParams{
		ID:        invoiceID,
		CompanyID: companyID,
	})
	if err != nil {
		return fmt.Errorf("invoice not found: %w", err)
	}
	if inv.Status != "draft" {
		return fmt.Errorf("cannot delete invoice with status %q", inv.Status)
	}
	return s.q.DeleteInvoice(ctx, sqlc.DeleteInvoiceParams{
		ID:        invoiceID,
		CompanyID: companyID,
	})
}

// floatToNum converts a float64 to pgtype.Numeric with 2 decimal places.
func floatToNum(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.2f", f))
	return n
}

// strPtrOrNil returns nil for empty strings, otherwise a pointer to the string.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// uuidToPG converts a *uuid.UUID to pgtype.UUID.
func uuidToPG(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
