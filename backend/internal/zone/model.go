// Package zone owns zones — the middle tier of Warehouse -> Zone ->
// Location, from WarehouseView.vue's "Zone Setup" tab. Every zone
// belongs to exactly one warehouse (internal/warehouse); every location
// belongs to exactly one zone (internal/location, which depends on this
// package the same way this package depends on warehouse).
package zone

import "time"

// Status mirrors the frontend's Zone status <select> (Active/Inactive).
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

// Type is the zone's functional role, matching the Zone Setup tab's
// fixed <select> options exactly.
type Type string

const (
	TypeReceiving Type = "RECEIVING"
	TypeStorage   Type = "STORAGE"
	TypePicking   Type = "PICKING"
	TypePacking   Type = "PACKING"
	TypeShipping  Type = "SHIPPING"
	TypeQC        Type = "QC"
	TypeHold      Type = "HOLD"
)

func (t Type) IsValid() bool {
	switch t {
	case TypeReceiving, TypeStorage, TypePicking, TypePacking, TypeShipping, TypeQC, TypeHold:
		return true
	default:
		return false
	}
}

// Zone is the GORM model for the zones table (migrations/0006).
type Zone struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	// Denormalized directly onto zones rather than inherited only via
	// WarehouseID, per tenant-er-design.md §3.
	TenantID    int64     `gorm:"column:tenant_id;not null;default:1"`
	WarehouseID int64     `gorm:"column:warehouse_id"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Type        Type      `gorm:"column:type"`
	Status      Status    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Zone) TableName() string { return "zones" }
