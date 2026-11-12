// Package product owns product master data, from ProductMasterView.vue's
// "Product Info" and "Inventory Rules" tabs. The frontend's "Barcode
// Mapping" (multiple barcodes per product) and "UOM Conversion" tabs are
// deliberately not modeled here — see PROJECT_STATUS.md — so this
// package has no child tables and no cross-package dependencies, unlike
// internal/warehouse -> zone -> location.
package product

import "time"

// Status mirrors the frontend's Active/Inactive <select> — the Product
// Master screen has no delete button anywhere; toggling status is the
// established way to retire a product.
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

// Category mirrors the frontend's fixed Category <select> options
// exactly — no separate categories table, since the frontend never
// exposes managing the category list itself.
type Category string

const (
	CategoryPackaging   Category = "PACKAGING"
	CategoryLabel       Category = "LABEL"
	CategoryRFID        Category = "RFID"
	CategoryRawMaterial Category = "RAW_MATERIAL"
)

func (c Category) IsValid() bool {
	switch c {
	case CategoryPackaging, CategoryLabel, CategoryRFID, CategoryRawMaterial:
		return true
	default:
		return false
	}
}

// UOM mirrors the frontend's fixed Base UOM <select> options.
type UOM string

const (
	UOMPieces UOM = "PCS"
	UOMBox    UOM = "BOX"
	UOMRoll   UOM = "ROLL"
	UOMKg     UOM = "KG"
)

func (u UOM) IsValid() bool {
	switch u {
	case UOMPieces, UOMBox, UOMRoll, UOMKg:
		return true
	default:
		return false
	}
}

// Product is the GORM model for the products table (migrations/0008).
type Product struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0043, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID       int64     `gorm:"column:tenant_id;not null;default:1"`
	SKU            string    `gorm:"column:sku"`
	Name           string    `gorm:"column:name"`
	Description    string    `gorm:"column:description"`
	Category       Category  `gorm:"column:category"`
	BaseUOM        UOM       `gorm:"column:base_uom"`
	PackSize       int       `gorm:"column:pack_size"`
	Barcode        *string   `gorm:"column:barcode"`
	LotRequired    bool      `gorm:"column:lot_required"`
	SerialRequired bool      `gorm:"column:serial_required"`
	ExpiryRequired bool      `gorm:"column:expiry_required"`
	DefaultZone    string    `gorm:"column:default_zone"`
	MinStock       int       `gorm:"column:min_stock"`
	MaxStock       int       `gorm:"column:max_stock"`
	Status         Status    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Product) TableName() string { return "products" }
