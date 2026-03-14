package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// VendorService handles vendor CRUD operations.
type VendorService struct {
	q *sqlc.Queries
}

// NewVendorService creates a VendorService.
func NewVendorService(q *sqlc.Queries) *VendorService {
	return &VendorService{q: q}
}

// VendorResponse is the API response for a vendor.
type VendorResponse struct {
	ID               string  `json:"id"`
	TIN              string  `json:"tin"`
	Name             string  `json:"name"`
	Address          *string `json:"address"`
	VendorType       string  `json:"vendor_type"`
	DefaultEWTRate   float64 `json:"default_ewt_rate"`
	DefaultATCCode   *string `json:"default_atc_code"`
	IsVATRegistered  bool    `json:"is_vat_registered"`
	Email            *string `json:"email,omitempty"`
	Phone            *string `json:"phone,omitempty"`
	PaymentTermsDays *int32  `json:"payment_terms_days,omitempty"`
	CurrencyCode     *string `json:"currency_code,omitempty"`
	IsActive         bool    `json:"is_active"`
	Notes            *string `json:"notes,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func vendorToResponse(v sqlc.Vendor) VendorResponse {
	resp := VendorResponse{
		ID:               v.ID.String(),
		TIN:              v.Tin,
		Name:             v.Name,
		Address:          v.Address,
		VendorType:       v.VendorType,
		DefaultATCCode:   v.DefaultAtcCode,
		IsVATRegistered:  v.IsVatRegistered,
		Email:            v.Email,
		Phone:            v.Phone,
		PaymentTermsDays: v.PaymentTermsDays,
		CurrencyCode:     v.CurrencyCode,
		IsActive:         v.IsActive,
		Notes:            v.Notes,
		CreatedAt:        v.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        v.UpdatedAt.Format(time.RFC3339),
	}
	if f, err := v.DefaultEwtRate.Float64Value(); err == nil {
		resp.DefaultEWTRate = f.Float64
	}
	return resp
}

// FindOrCreate looks up a vendor by TIN; if not found, creates one.
// If TIN is empty but name is provided, searches by name without auto-creating.
func (s *VendorService) FindOrCreate(ctx context.Context, companyID uuid.UUID, tin, name string) (*VendorResponse, error) {
	tin = strings.TrimSpace(tin)
	name = strings.TrimSpace(name)

	if tin != "" {
		existing, err := s.q.GetVendorByTIN(ctx, sqlc.GetVendorByTINParams{
			CompanyID: companyID,
			Tin:       tin,
		})
		if err == nil {
			resp := vendorToResponse(existing)
			return &resp, nil
		}

		// Not found — create with defaults
		resp, err := s.Create(ctx, companyID, CreateVendorInput{
			TIN:             tin,
			Name:            name,
			VendorType:      "corporation",
			IsVATRegistered: true,
		})
		if err != nil {
			return nil, fmt.Errorf("find or create vendor: %w", err)
		}
		slog.Info("auto-created vendor", "tin", tin, "name", name, "company_id", companyID)
		return resp, nil
	}

	// TIN empty — search by name only, no auto-create
	if name == "" {
		return nil, fmt.Errorf("both TIN and name are empty")
	}
	vendors, err := s.q.SearchVendorsByCompany(ctx, sqlc.SearchVendorsByCompanyParams{
		CompanyID: companyID,
		Column2:   &name,
		Limit:     1,
		Offset:    0,
	})
	if err != nil || len(vendors) == 0 {
		return nil, fmt.Errorf("no vendor found for name %q", name)
	}
	resp := vendorToResponse(vendors[0])
	return &resp, nil
}

// GetByID returns a vendor by ID.
func (s *VendorService) GetByID(ctx context.Context, id uuid.UUID) (*VendorResponse, error) {
	v, err := s.q.GetVendorByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("vendor not found")
	}
	resp := vendorToResponse(v)
	return &resp, nil
}

// List returns paginated vendors for a company, with optional search.
func (s *VendorService) List(ctx context.Context, companyID uuid.UUID, limit, offset int, search string) ([]VendorResponse, int64, error) {
	var vendors []sqlc.Vendor
	var err error

	if search != "" {
		vendors, err = s.q.SearchVendorsByCompany(ctx, sqlc.SearchVendorsByCompanyParams{
			CompanyID: companyID,
			Column2:   &search,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
	} else {
		vendors, err = s.q.ListVendorsByCompany(ctx, sqlc.ListVendorsByCompanyParams{
			CompanyID: companyID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list vendors: %w", err)
	}

	total, err := s.q.CountVendorsByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	result := make([]VendorResponse, len(vendors))
	for i, v := range vendors {
		result[i] = vendorToResponse(v)
	}
	return result, total, nil
}

// CreateVendorInput holds input for creating a vendor.
type CreateVendorInput struct {
	TIN              string  `json:"tin"`
	Name             string  `json:"name"`
	Address          *string `json:"address"`
	VendorType       string  `json:"vendor_type"`
	DefaultEWTRate   float64 `json:"default_ewt_rate"`
	DefaultATCCode   *string `json:"default_atc_code"`
	IsVATRegistered  bool    `json:"is_vat_registered"`
	Email            *string `json:"email"`
	Phone            *string `json:"phone"`
	PaymentTermsDays *int32  `json:"payment_terms_days"`
	CurrencyCode     *string `json:"currency_code"`
	IsActive         *bool   `json:"is_active"`
	Notes            *string `json:"notes"`
}

// Create creates a new vendor, checking TIN uniqueness within the company.
func (s *VendorService) Create(ctx context.Context, companyID uuid.UUID, input CreateVendorInput) (*VendorResponse, error) {
	// Check TIN uniqueness
	if input.TIN != "" {
		_, err := s.q.GetVendorByTIN(ctx, sqlc.GetVendorByTINParams{
			CompanyID: companyID,
			Tin:       input.TIN,
		})
		if err == nil {
			return nil, fmt.Errorf("vendor with TIN %s already exists", input.TIN)
		}
	}

	vendorType := input.VendorType
	if vendorType == "" {
		vendorType = "individual"
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var ewtRate pgtype.Numeric
	scanNumeric(&ewtRate, input.DefaultEWTRate)

	vendor, err := s.q.CreateVendor(ctx, sqlc.CreateVendorParams{
		ID:               uuid.New(),
		CompanyID:        companyID,
		Tin:              input.TIN,
		Name:             input.Name,
		Address:          input.Address,
		VendorType:       vendorType,
		DefaultEwtRate:   ewtRate,
		DefaultAtcCode:   input.DefaultATCCode,
		IsVatRegistered:  input.IsVATRegistered,
		Email:            input.Email,
		Phone:            input.Phone,
		PaymentTermsDays: input.PaymentTermsDays,
		CurrencyCode:     input.CurrencyCode,
		IsActive:         isActive,
		Notes:            input.Notes,
	})
	if err != nil {
		return nil, fmt.Errorf("create vendor: %w", err)
	}

	resp := vendorToResponse(vendor)
	return &resp, nil
}

// Update updates an existing vendor.
func (s *VendorService) Update(ctx context.Context, id, companyID uuid.UUID, input CreateVendorInput) (*VendorResponse, error) {
	existing, err := s.q.GetVendorByID(ctx, id)
	if err != nil || existing.CompanyID != companyID {
		return nil, fmt.Errorf("vendor not found")
	}

	name := input.Name
	if name == "" {
		name = existing.Name
	}
	tin := input.TIN
	if tin == "" {
		tin = existing.Tin
	}
	vendorType := input.VendorType
	if vendorType == "" {
		vendorType = existing.VendorType
	}
	addr := input.Address
	if addr == nil {
		addr = existing.Address
	}
	atcCode := input.DefaultATCCode
	if atcCode == nil {
		atcCode = existing.DefaultAtcCode
	}
	email := input.Email
	if email == nil {
		email = existing.Email
	}
	phone := input.Phone
	if phone == nil {
		phone = existing.Phone
	}
	paymentTerms := input.PaymentTermsDays
	if paymentTerms == nil {
		paymentTerms = existing.PaymentTermsDays
	}
	currencyCode := input.CurrencyCode
	if currencyCode == nil {
		currencyCode = existing.CurrencyCode
	}
	notes := input.Notes
	if notes == nil {
		notes = existing.Notes
	}
	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var ewtRate pgtype.Numeric
	if input.DefaultEWTRate > 0 {
		scanNumeric(&ewtRate, input.DefaultEWTRate)
	} else {
		ewtRate = existing.DefaultEwtRate
	}

	err = s.q.UpdateVendor(ctx, sqlc.UpdateVendorParams{
		ID:               id,
		Tin:              tin,
		Name:             name,
		Address:          addr,
		VendorType:       vendorType,
		DefaultEwtRate:   ewtRate,
		DefaultAtcCode:   atcCode,
		IsVatRegistered:  input.IsVATRegistered,
		Email:            email,
		Phone:            phone,
		PaymentTermsDays: paymentTerms,
		CurrencyCode:     currencyCode,
		DefaultAccountID: existing.DefaultAccountID,
		IsActive:         isActive,
		Notes:            notes,
	})
	if err != nil {
		return nil, fmt.Errorf("update vendor: %w", err)
	}

	updated, err := s.q.GetVendorByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := vendorToResponse(updated)
	return &resp, nil
}

// Delete deletes a vendor.
func (s *VendorService) Delete(ctx context.Context, id, companyID uuid.UUID) error {
	existing, err := s.q.GetVendorByID(ctx, id)
	if err != nil || existing.CompanyID != companyID {
		return fmt.Errorf("vendor not found")
	}

	return s.q.DeleteVendor(ctx, sqlc.DeleteVendorParams{
		ID:        id,
		CompanyID: companyID,
	})
}
