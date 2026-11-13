// Package product owns product master data, from ProductMasterView.vue's
// Basic Information / Warehouse Settings / Additional Details sections
// (previously a 4-tab layout — see PROJECT_STATUS.md for the EWTA
// Product Master UX redesign this replaces). product_barcodes
// (barcode.go) is this package's one child table, added by the EWTA
// enhancement to make the frontend's previously-decorative "Barcode
// Mapping" tab real; "UOM Conversion"/packaging-conversion arithmetic
// remains deferred (P1, not modeled here).
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

// Category mirrors the frontend's original fixed Category <select>
// options. The EWTA redesign stops exposing this as a user-facing
// control (CategoryID — the hierarchical internal/productcategory
// master — is now the one category selector the form shows); this
// field, its CHECK constraint, and every existing row are otherwise
// completely untouched, so Create/Update still accept and store it
// exactly as before — see Service.Create/Update, which default an
// omitted Category to CategoryPackaging so the new, simplified request
// shape still satisfies the column's NOT NULL + CHECK contract.
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

// UOM mirrors the frontend's Base UOM <select> options. G/L/ML were
// added by the EWTA enhancement (migrations/0054 widened
// CK_products_base_uom to match) for imported-food products priced and
// stocked by weight/volume rather than piece/box/roll — every existing
// PCS/BOX/ROLL/KG row and caller is unaffected.
type UOM string

const (
	UOMPieces     UOM = "PCS"
	UOMBox        UOM = "BOX"
	UOMRoll       UOM = "ROLL"
	UOMKg         UOM = "KG"
	UOMGram       UOM = "G"
	UOMLiter      UOM = "L"
	UOMMilliliter UOM = "ML"
)

func (u UOM) IsValid() bool {
	switch u {
	case UOMPieces, UOMBox, UOMRoll, UOMKg, UOMGram, UOMLiter, UOMMilliliter:
		return true
	default:
		return false
	}
}

// StorageType is the EWTA Warehouse Settings section's storage
// condition selector (migrations/0053/0054).
type StorageType string

const (
	StorageAmbient StorageType = "AMBIENT"
	StorageChilled StorageType = "CHILLED"
	StorageFrozen  StorageType = "FROZEN"
)

func (s StorageType) IsValid() bool {
	switch s {
	case "", StorageAmbient, StorageChilled, StorageFrozen:
		return true
	default:
		return false
	}
}

// RotationRule is the EWTA Warehouse Settings section's FIFO/FEFO
// selector — independent of (and may be combined with) LotRequired/
// ExpiryRequired below.
type RotationRule string

const (
	RotationFIFO RotationRule = "FIFO"
	RotationFEFO RotationRule = "FEFO"
)

func (r RotationRule) IsValid() bool {
	switch r {
	case "", RotationFIFO, RotationFEFO:
		return true
	default:
		return false
	}
}

// WeightUnit covers GrossWeight/NetWeight below.
type WeightUnit string

const (
	WeightGram     WeightUnit = "G"
	WeightKilogram WeightUnit = "KG"
)

func (w WeightUnit) IsValid() bool {
	switch w {
	case "", WeightGram, WeightKilogram:
		return true
	default:
		return false
	}
}

// Product is the GORM model for the products table (migrations/0008,
// extended by the EWTA enhancement's migrations/0053-0055).
type Product struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0043, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale. As
	// of the EWTA enhancement this is no longer just a DB default:
	// Repository now sets/filters it explicitly on every call (see
	// repository.go's own doc for why that changed).
	TenantID    int64    `gorm:"column:tenant_id;not null;default:1"`
	SKU         string   `gorm:"column:sku"`
	Name        string   `gorm:"column:name"`
	NameTH      *string  `gorm:"column:name_th"`
	Description string   `gorm:"column:description"`
	Category    Category `gorm:"column:category"`
	// CategoryID (migrations/0049) is the hierarchical
	// internal/productcategory master's FK — the one category selector
	// the EWTA-redesigned form shows (see Category's own doc above for
	// why the legacy enum is kept but hidden).
	CategoryID *int64 `gorm:"column:category_id"`
	BrandID    *int64 `gorm:"column:brand_id"`
	FamilyID   *int64 `gorm:"column:family_id"`
	// VariantName distinguishes sellable SKUs within the same Family
	// (e.g. Family "Cheddar Block" + VariantName "150g Block").
	VariantName          string       `gorm:"column:variant_name"`
	BaseUOM              UOM          `gorm:"column:base_uom"`
	PackSize             int          `gorm:"column:pack_size"`
	PackSizeUnit         string       `gorm:"column:pack_size_unit"`
	Barcode              *string      `gorm:"column:barcode"`
	LotRequired          bool         `gorm:"column:lot_required"`
	SerialRequired       bool         `gorm:"column:serial_required"`
	ExpiryRequired       bool         `gorm:"column:expiry_required"`
	StorageType          StorageType  `gorm:"column:storage_type"`
	RotationRule         RotationRule `gorm:"column:rotation_rule"`
	DefaultSupplierID    *int64       `gorm:"column:default_supplier_id"`
	CountryOfOrigin      string       `gorm:"column:country_of_origin"`
	PreferredWarehouseID *int64       `gorm:"column:preferred_warehouse_id"`
	PreferredZoneID      *int64       `gorm:"column:preferred_zone_id"`
	// DefaultZone is the original free-text A/B/C zone picker
	// (migrations/0008) — kept exactly as-is for backward compatibility;
	// PreferredWarehouseID/PreferredZoneID above are the new, real FK
	// relationships the EWTA Warehouse Settings section actually uses.
	DefaultZone  string     `gorm:"column:default_zone"`
	Manufacturer string     `gorm:"column:manufacturer"`
	MinStock     int        `gorm:"column:min_stock"`
	MaxStock     int        `gorm:"column:max_stock"`
	GrossWeight  *float64   `gorm:"column:gross_weight"`
	NetWeight    *float64   `gorm:"column:net_weight"`
	WeightUnit   WeightUnit `gorm:"column:weight_unit"`
	LengthMM     *float64   `gorm:"column:length_mm"`
	WidthMM      *float64   `gorm:"column:width_mm"`
	HeightMM     *float64   `gorm:"column:height_mm"`
	// Image* columns: ImagePath is a server-generated, UUID-based
	// relative path under config.ImageStorageDir — see image.go's own
	// doc for why this is never derived from client input.
	ImagePath         *string    `gorm:"column:image_path"`
	ImageOriginalName *string    `gorm:"column:image_original_name"`
	ImageMimeType     *string    `gorm:"column:image_mime_type"`
	ImageSizeBytes    *int       `gorm:"column:image_size_bytes"`
	ImageUploadedAt   *time.Time `gorm:"column:image_uploaded_at"`
	Status            Status     `gorm:"column:status"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (Product) TableName() string { return "products" }
