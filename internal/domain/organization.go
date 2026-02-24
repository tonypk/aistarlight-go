package domain

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Plan         string    `json:"plan"`
	MaxCompanies int       `json:"max_companies"`
	MaxUsers     int       `json:"max_users"`
	Settings     JSON      `json:"settings"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
