// Package location owns bin locations — the bottom tier of Warehouse ->
// Zone -> Location, from WarehouseView.vue's "Location Structure" tab
// (zone -> aisle -> rack -> bin). Every location belongs to exactly one
// zone (internal/zone).
package location

import "time"

// Status mirrors the frontend's implicit Active/Inactive convention used
// by every other status field in this hierarchy.
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

// Type is the individual bin's own functional role (migrations/0050,
// Location Master Management) — independent from its zone's own Type
// (internal/zone.Type): a location's type describes what that specific
// bin is used for, which can differ from its zone's overall purpose
// (e.g. a QC bin living inside a general STORAGE zone). Reuses the same
// 7-value vocabulary as zone.Type because it's already a validated,
// familiar set for this schema — not because the two are meant to be
// kept in sync. Optional: "" means not set, matching every existing
// optional field in this codebase (e.g. product.Description).
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
	case "", TypeReceiving, TypeStorage, TypePicking, TypePacking, TypeShipping, TypeQC, TypeHold:
		return true
	default:
		return false
	}
}

// Location is the GORM model for the locations table (migrations/0007,
// 0050).
type Location struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	// Denormalized directly onto locations, per tenant-er-design.md §3.
	// Unlike every other method on this package's Repository before
	// Location Master Management, every query now filters by this
	// column explicitly from a verified request context — see
	// repository.go's own doc.
	TenantID int64  `gorm:"column:tenant_id;not null;default:1"`
	ZoneID   int64  `gorm:"column:zone_id"`
	Code     string `gorm:"column:code"`
	// Name (migrations/0050) is optional — mirrors zones.name's own
	// purpose (a human-readable label), not every bin needs one beyond
	// its code.
	Name      string    `gorm:"column:name"`
	Aisle     string    `gorm:"column:aisle"`
	Rack      string    `gorm:"column:rack"`
	Bin       string    `gorm:"column:bin"`
	Type      Type      `gorm:"column:type"`
	Capacity  *int      `gorm:"column:capacity"`
	Status    Status    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Location) TableName() string { return "locations" }
