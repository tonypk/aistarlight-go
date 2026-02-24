package domain

import (
	"time"

	"github.com/google/uuid"
)

type FormSchema struct {
	ID               uuid.UUID `json:"id"`
	FormType         string    `json:"form_type"`
	Version          int       `json:"version"`
	Name             string    `json:"name"`
	Frequency        string    `json:"frequency"`
	IsActive         bool      `json:"is_active"`
	SchemaDef        JSON      `json:"schema_def"`
	CalculationRules JSON      `json:"calculation_rules"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
