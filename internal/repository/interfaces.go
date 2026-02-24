package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// ---- Organization ----

type OrgRepo interface {
	Create(ctx context.Context, org *domain.Organization) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
	Update(ctx context.Context, org *domain.Organization) error
	ListByUser(ctx context.Context, userID uuid.UUID, p pagination.Params) ([]domain.Organization, int, error)
}

type OrgMemberRepo interface {
	Add(ctx context.Context, m *domain.OrgMember) error
	Remove(ctx context.Context, orgID, userID uuid.UUID) error
	UpdateRole(ctx context.Context, orgID, userID uuid.UUID, role domain.OrgRole) error
	GetRole(ctx context.Context, orgID, userID uuid.UUID) (domain.OrgRole, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]domain.OrgMember, error)
}

// ---- Company ----

type CompanyRepo interface {
	Create(ctx context.Context, c *domain.Company) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Company, error)
	Update(ctx context.Context, c *domain.Company) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, p pagination.Params) ([]domain.Company, int, error)
	ListByUser(ctx context.Context, userID uuid.UUID, p pagination.Params) ([]domain.Company, int, error)
}

type CompanyMemberRepo interface {
	Add(ctx context.Context, m *domain.CompanyMember) error
	Remove(ctx context.Context, companyID, userID uuid.UUID) error
	UpdateRole(ctx context.Context, companyID, userID uuid.UUID, role domain.CompanyRole) error
	GetRole(ctx context.Context, companyID, userID uuid.UUID) (domain.CompanyRole, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID) ([]domain.CompanyMember, error)
}

// ---- User ----

type UserRepo interface {
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByAPIKey(ctx context.Context, key string) (*domain.User, error)
	Update(ctx context.Context, u *domain.User) error
}

// ---- Report ----

type ReportRepo interface {
	Create(ctx context.Context, r *domain.Report) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Report, error)
	Update(ctx context.Context, r *domain.Report) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Report, int, error)
	ListByCompanyAndType(ctx context.Context, companyID uuid.UUID, reportType string, p pagination.Params) ([]domain.Report, int, error)
}

// ---- Transaction ----

type TransactionRepo interface {
	CreateBatch(ctx context.Context, txns []domain.Transaction) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error)
	Update(ctx context.Context, t *domain.Transaction) error
	ListBySession(ctx context.Context, sessionID uuid.UUID, p pagination.Params) ([]domain.Transaction, int, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Transaction, int, error)
	DeleteBySession(ctx context.Context, sessionID uuid.UUID) error
}

// ---- Reconciliation ----

type ReconciliationRepo interface {
	Create(ctx context.Context, s *domain.ReconciliationSession) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ReconciliationSession, error)
	Update(ctx context.Context, s *domain.ReconciliationSession) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.ReconciliationSession, int, error)
}

// ---- Chat ----

type ChatRepo interface {
	Create(ctx context.Context, m *domain.ChatMessage) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, limit int) ([]domain.ChatMessage, error)
}

// ---- Knowledge ----

type KnowledgeRepo interface {
	Create(ctx context.Context, k *domain.KnowledgeChunk) error
	SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]domain.KnowledgeChunk, error)
	ListByCategory(ctx context.Context, category string) ([]domain.KnowledgeChunk, error)
}

// ---- Supplier ----

type SupplierRepo interface {
	Create(ctx context.Context, s *domain.Supplier) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Supplier, error)
	Update(ctx context.Context, s *domain.Supplier) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Supplier, int, error)
	GetByTIN(ctx context.Context, companyID uuid.UUID, tin string) (*domain.Supplier, error)
}

// ---- Withholding ----

type WithholdingRepo interface {
	Create(ctx context.Context, w *domain.WithholdingCertificate) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.WithholdingCertificate, error)
	Update(ctx context.Context, w *domain.WithholdingCertificate) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.WithholdingCertificate, int, error)
	ListBySupplier(ctx context.Context, supplierID uuid.UUID, period string) ([]domain.WithholdingCertificate, error)
}

// ---- Correction ----

type CorrectionRepo interface {
	Create(ctx context.Context, c *domain.Correction) error
	ListByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]domain.Correction, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Correction, int, error)
}

type CorrectionRuleRepo interface {
	Create(ctx context.Context, r *domain.CorrectionRule) error
	Update(ctx context.Context, r *domain.CorrectionRule) error
	ListActiveByCompany(ctx context.Context, companyID uuid.UUID) ([]domain.CorrectionRule, error)
	FindMatching(ctx context.Context, companyID uuid.UUID, ruleType, field string) ([]domain.CorrectionRule, error)
}

type ValidationResultRepo interface {
	Create(ctx context.Context, v *domain.ValidationResult) error
	GetLatestByReport(ctx context.Context, reportID uuid.UUID) (*domain.ValidationResult, error)
	ListByReport(ctx context.Context, reportID uuid.UUID) ([]domain.ValidationResult, error)
}

// ---- Audit ----

type AuditRepo interface {
	Create(ctx context.Context, a *domain.AuditLog) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.AuditLog, int, error)
	ListByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]domain.AuditLog, error)
}

// ---- Others ----

type AnomalyRepo interface {
	Create(ctx context.Context, a *domain.Anomaly) error
	Update(ctx context.Context, a *domain.Anomaly) error
	ListBySession(ctx context.Context, sessionID uuid.UUID) ([]domain.Anomaly, error)
}

type FormSchemaRepo interface {
	GetByType(ctx context.Context, formType string) (*domain.FormSchema, error)
	ListActive(ctx context.Context) ([]domain.FormSchema, error)
	Upsert(ctx context.Context, s *domain.FormSchema) error
}

type UserPreferenceRepo interface {
	Upsert(ctx context.Context, pref *domain.UserPreference) error
	GetByCompanyAndType(ctx context.Context, companyID uuid.UUID, reportType string) (*domain.UserPreference, error)
}

type ReceiptBatchRepo interface {
	Create(ctx context.Context, b *domain.ReceiptBatch) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ReceiptBatch, error)
	Update(ctx context.Context, b *domain.ReceiptBatch) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.ReceiptBatch, int, error)
}

type BankReconBatchRepo interface {
	Create(ctx context.Context, b *domain.BankReconciliationBatch) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.BankReconciliationBatch, error)
	Update(ctx context.Context, b *domain.BankReconciliationBatch) error
	ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.BankReconciliationBatch, int, error)
}

type RevokedTokenRepo interface {
	Create(ctx context.Context, t *domain.RevokedToken) error
	IsRevoked(ctx context.Context, jti string) (bool, error)
	CleanExpired(ctx context.Context) error
}
