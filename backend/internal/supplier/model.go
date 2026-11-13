// Package supplier owns the Supplier master data module (Master Data
// Expansion) — tenant-scoped supplier records (company name, tax ID, a
// single contact person, address, email, phone). See
// internal/productcategory/repository.go's doc for why this module's
// Repository, unlike internal/product/internal/warehouse, threads an
// explicit, verified tenant_id through every query from day one.
package supplier

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

// Supplier is the GORM model for the suppliers table (migrations/0049).
type Supplier struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	TenantID      int64     `gorm:"column:tenant_id;not null;default:1"`
	Code          string    `gorm:"column:code"`
	CompanyName   string    `gorm:"column:company_name"`
	TaxID         string    `gorm:"column:tax_id"`
	ContactPerson string    `gorm:"column:contact_person"`
	ContactPhone  string    `gorm:"column:contact_phone"`
	Address       string    `gorm:"column:address"`
	Email         string    `gorm:"column:email"`
	Phone         string    `gorm:"column:phone"`
	Status        Status    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Supplier) TableName() string { return "suppliers" }
