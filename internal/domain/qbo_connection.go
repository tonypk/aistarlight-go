package domain

import (
	"time"

	"github.com/google/uuid"
)

type SyncStatus string

const (
	SyncStatusRunning   SyncStatus = "running"
	SyncStatusCompleted SyncStatus = "completed"
	SyncStatusFailed    SyncStatus = "failed"
)

// QBOConnection represents a QuickBooks Online OAuth connection.
// Tokens are NEVER serialized to JSON (json:"-").
type QBOConnection struct {
	ID              uuid.UUID  `json:"id"`
	CompanyID       uuid.UUID  `json:"company_id"`
	RealmID         string     `json:"realm_id"`
	AccessTokenEnc  []byte     `json:"-"`
	RefreshTokenEnc []byte     `json:"-"`
	TokenExpiry     time.Time  `json:"token_expiry"`
	RefreshExpiry   time.Time  `json:"refresh_expiry"`
	Scope           *string    `json:"scope,omitempty"`
	IsActive        bool       `json:"is_active"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus  *string    `json:"last_sync_status,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// QBOSyncLog records details of a QBO sync operation.
type QBOSyncLog struct {
	ID            uuid.UUID  `json:"id"`
	ConnectionID  uuid.UUID  `json:"connection_id"`
	CompanyID     uuid.UUID  `json:"company_id"`
	EntityType    string     `json:"entity_type"`
	SyncType      string     `json:"sync_type"`
	SyncDirection string     `json:"sync_direction"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	RecordsSynced int        `json:"records_synced"`
	RecordsFailed int        `json:"records_failed"`
	ErrorDetails  JSON       `json:"error_details,omitempty"`
	Status        SyncStatus `json:"status"`
}
