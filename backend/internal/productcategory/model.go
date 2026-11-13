// Package productcategory owns the Product Category master data module
// (Master Data Expansion) — a tenant-scoped hierarchy of categories that
// Product Master's optional category_id selection references. This is
// deliberately separate from internal/product's own fixed Category enum
// (PACKAGING/LABEL/RFID/RAW_MATERIAL): that enum is preserved unchanged
// for backward compatibility with existing data/filters/business logic,
// while this table is the new, admin-managed, arbitrarily-nestable
// category list the frontend's Product Master category picker now also
// offers alongside it. See internal/product/dto.go's CategoryID field
// doc for how the two coexist.
package productcategory

import "time"

// Status mirrors every other master-data module's ACTIVE/INACTIVE
// toggle (internal/product.Status, internal/warehouse.Status, ...).
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

// Category is the GORM model for the product_categories table
// (migrations/0049). TenantID has the same not-null-default-1 shape
// internal/role.Role documents, but — unlike product/warehouse, which
// never actually thread a verified tenant_id through their
// service/repository layers — every Create/Update/Delete/List call on
// this module's Repository always sets/filters this column explicitly
// from middleware.EnforcedTenantID, never relying on the default. See
// repository.go's own doc for why this module is held to that stricter
// standard from day one.
type Category struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	TenantID    int64     `gorm:"column:tenant_id;not null;default:1"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	ParentID    *int64    `gorm:"column:parent_id"`
	Description string    `gorm:"column:description"`
	Status      Status    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Category) TableName() string { return "product_categories" }
