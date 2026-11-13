// Package productfamily owns the Product Family master data module
// (EWTA Product Master enhancement) — tenant-scoped records that group
// sellable SKUs/variants under one family (e.g. "Cheddar Block" family
// with 150g/300g variants). Same minimal flat shape and tenant-scoping
// discipline as internal/brand.
package productfamily

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

// Family is the GORM model for the product_families table
// (migrations/0052).
type Family struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	TenantID  int64     `gorm:"column:tenant_id;not null;default:1"`
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	Status    Status    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Family) TableName() string { return "product_families" }
