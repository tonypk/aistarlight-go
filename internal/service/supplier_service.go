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

// SupplierService handles supplier CRUD operations.
type SupplierService struct {
	q *sqlc.Queries
}

// NewSupplierService creates a SupplierService.
func NewSupplierService(q *sqlc.Queries) *SupplierService {
	return &SupplierService{q: q}
}

// SupplierResponse is the API response for a supplier.
type SupplierResponse struct {
	ID              string  `json:"id"`
	TIN             string  `json:"tin"`
	Name            string  `json:"name"`
	Address         *string `json:"address"`
	SupplierType    string  `json:"supplier_type"`
	DefaultEWTRate  float64 `json:"default_ewt_rate"`
	DefaultATCCode  *string `json:"default_atc_code"`
	IsVATRegistered bool    `json:"is_vat_registered"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func supplierToResponse(s sqlc.Supplier) SupplierResponse {
	resp := SupplierResponse{
		ID:              s.ID.String(),
		TIN:             s.Tin,
		Name:            s.Name,
		Address:         s.Address,
		SupplierType:    s.SupplierType,
		DefaultATCCode:  s.DefaultAtcCode,
		IsVATRegistered: s.IsVatRegistered,
		CreatedAt:       s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       s.UpdatedAt.Format(time.RFC3339),
	}
	if f, err := s.DefaultEwtRate.Float64Value(); err == nil {
		resp.DefaultEWTRate = f.Float64
	}
	return resp
}

// FindOrCreate looks up a supplier by TIN; if not found, creates one.
// If TIN is empty but name is provided, searches by name without auto-creating.
func (s *SupplierService) FindOrCreate(ctx context.Context, companyID uuid.UUID, tin, name string) (*SupplierResponse, error) {
	tin = strings.TrimSpace(tin)
	name = strings.TrimSpace(name)

	if tin != "" {
		existing, err := s.q.GetSupplierByTIN(ctx, sqlc.GetSupplierByTINParams{
			CompanyID: companyID,
			Tin:       tin,
		})
		if err == nil {
			resp := supplierToResponse(existing)
			return &resp, nil
		}

		// Not found — create with defaults
		resp, err := s.Create(ctx, companyID, CreateSupplierInput{
			TIN:             tin,
			Name:            name,
			SupplierType:    "corporation",
			IsVATRegistered: true,
		})
		if err != nil {
			return nil, fmt.Errorf("find or create supplier: %w", err)
		}
		slog.Info("auto-created supplier", "tin", tin, "name", name, "company_id", companyID)
		return resp, nil
	}

	// TIN empty — search by name only, no auto-create
	if name == "" {
		return nil, fmt.Errorf("both TIN and name are empty")
	}
	suppliers, err := s.q.SearchSuppliersByCompany(ctx, sqlc.SearchSuppliersByCompanyParams{
		CompanyID: companyID,
		Column2:   &name,
		Limit:     1,
		Offset:    0,
	})
	if err != nil || len(suppliers) == 0 {
		return nil, fmt.Errorf("no supplier found for name %q", name)
	}
	resp := supplierToResponse(suppliers[0])
	return &resp, nil
}

// GetByID returns a supplier by ID.
func (s *SupplierService) GetByID(ctx context.Context, id uuid.UUID) (*SupplierResponse, error) {
	sup, err := s.q.GetSupplierByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("supplier not found")
	}
	resp := supplierToResponse(sup)
	return &resp, nil
}

// List returns paginated suppliers for a company, with optional search.
func (s *SupplierService) List(ctx context.Context, companyID uuid.UUID, limit, offset int, search string) ([]SupplierResponse, int64, error) {
	var suppliers []sqlc.Supplier
	var err error

	if search != "" {
		suppliers, err = s.q.SearchSuppliersByCompany(ctx, sqlc.SearchSuppliersByCompanyParams{
			CompanyID: companyID,
			Column2:   &search,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
	} else {
		suppliers, err = s.q.ListSuppliersByCompany(ctx, sqlc.ListSuppliersByCompanyParams{
			CompanyID: companyID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list suppliers: %w", err)
	}

	total, err := s.q.CountSuppliersByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	result := make([]SupplierResponse, len(suppliers))
	for i, sup := range suppliers {
		result[i] = supplierToResponse(sup)
	}
	return result, total, nil
}

// CreateSupplierInput holds input for creating a supplier.
type CreateSupplierInput struct {
	TIN             string  `json:"tin"`
	Name            string  `json:"name"`
	Address         *string `json:"address"`
	SupplierType    string  `json:"supplier_type"`
	DefaultEWTRate  float64 `json:"default_ewt_rate"`
	DefaultATCCode  *string `json:"default_atc_code"`
	IsVATRegistered bool    `json:"is_vat_registered"`
}

// Create creates a new supplier, checking TIN uniqueness within the company.
func (s *SupplierService) Create(ctx context.Context, companyID uuid.UUID, input CreateSupplierInput) (*SupplierResponse, error) {
	// Check TIN uniqueness
	if input.TIN != "" {
		_, err := s.q.GetSupplierByTIN(ctx, sqlc.GetSupplierByTINParams{
			CompanyID: companyID,
			Tin:       input.TIN,
		})
		if err == nil {
			return nil, fmt.Errorf("supplier with TIN %s already exists", input.TIN)
		}
	}

	supType := input.SupplierType
	if supType == "" {
		supType = "individual"
	}

	var ewtRate pgtype.Numeric
	scanNumeric(&ewtRate, input.DefaultEWTRate)

	supplier, err := s.q.CreateSupplier(ctx, sqlc.CreateSupplierParams{
		ID:              uuid.New(),
		CompanyID:       companyID,
		Tin:             input.TIN,
		Name:            input.Name,
		Address:         input.Address,
		SupplierType:    supType,
		DefaultEwtRate:  ewtRate,
		DefaultAtcCode:  input.DefaultATCCode,
		IsVatRegistered: input.IsVATRegistered,
	})
	if err != nil {
		return nil, fmt.Errorf("create supplier: %w", err)
	}

	resp := supplierToResponse(supplier)
	return &resp, nil
}

// Update updates an existing supplier.
func (s *SupplierService) Update(ctx context.Context, id, companyID uuid.UUID, input CreateSupplierInput) (*SupplierResponse, error) {
	existing, err := s.q.GetSupplierByID(ctx, id)
	if err != nil || existing.CompanyID != companyID {
		return nil, fmt.Errorf("supplier not found")
	}

	name := input.Name
	if name == "" {
		name = existing.Name
	}
	tin := input.TIN
	if tin == "" {
		tin = existing.Tin
	}
	supType := input.SupplierType
	if supType == "" {
		supType = existing.SupplierType
	}
	addr := input.Address
	if addr == nil {
		addr = existing.Address
	}
	atcCode := input.DefaultATCCode
	if atcCode == nil {
		atcCode = existing.DefaultAtcCode
	}

	var ewtRate pgtype.Numeric
	if input.DefaultEWTRate > 0 {
		scanNumeric(&ewtRate, input.DefaultEWTRate)
	} else {
		ewtRate = existing.DefaultEwtRate
	}

	err = s.q.UpdateSupplier(ctx, sqlc.UpdateSupplierParams{
		ID:              id,
		Tin:             tin,
		Name:            name,
		Address:         addr,
		SupplierType:    supType,
		DefaultEwtRate:  ewtRate,
		DefaultAtcCode:  atcCode,
		IsVatRegistered: input.IsVATRegistered,
	})
	if err != nil {
		return nil, fmt.Errorf("update supplier: %w", err)
	}

	updated, err := s.q.GetSupplierByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := supplierToResponse(updated)
	return &resp, nil
}

// Delete deletes a supplier.
func (s *SupplierService) Delete(ctx context.Context, id, companyID uuid.UUID) error {
	existing, err := s.q.GetSupplierByID(ctx, id)
	if err != nil || existing.CompanyID != companyID {
		return fmt.Errorf("supplier not found")
	}

	return s.q.DeleteSupplier(ctx, sqlc.DeleteSupplierParams{
		ID:        id,
		CompanyID: companyID,
	})
}
