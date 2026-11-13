// Package brand owns the Brand master data module (EWTA Product Master
// enhancement) — tenant-scoped brand records (code, name, status) the
// Product Add/Edit form's Brand selector picks from. See
// internal/productcategory/repository.go's doc for why this module's
// Repository, unlike internal/product/internal/warehouse, threads an
// explicit, verified tenant_id through every query from day one.
package brand

import "time"

// Status mirrors every other master-data module's ACTIVE/INACTIVE
// toggle.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive:
		return true
	default:
		return false
	}
}

// Brand is the GORM model for the brands table (migrations/0051).
type Brand struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	TenantID  int64     `gorm:"column:tenant_id;not null;default:1"`
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	Status    Status    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Brand) TableName() string { return "brands" }
