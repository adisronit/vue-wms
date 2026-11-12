// Package warehouse owns warehouse master data — the top of the
// Warehouse -> Zone -> Location hierarchy from WarehouseView.vue's
// "Warehouse Setup" screen. Zones and locations are separate packages
// (internal/zone, internal/location) per CLAUDE.md's per-domain layout;
// this package only knows about zones/locations through a narrow
// interface (see service.go) to avoid an import cycle.
package warehouse

import "time"

// Status mirrors the frontend's two-state Warehouse status <select>
// (Active/Inactive) — no LOCKED state here, unlike users.
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

// Warehouse is the GORM model for the warehouses table (migrations/0005).
// Capacity is basic master data the admin sets; there is no "occupied"
// column here, since that would require real inventory quantities,
// which are out of scope this phase — utilization is not computable yet.
type Warehouse struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0043, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID      int64     `gorm:"column:tenant_id;not null;default:1"`
	Code          string    `gorm:"column:code"`
	Name          string    `gorm:"column:name"`
	Type          string    `gorm:"column:type"`
	Address       string    `gorm:"column:address"`
	ContactPerson string    `gorm:"column:contact_person"`
	Phone         string    `gorm:"column:phone"`
	Capacity      *int      `gorm:"column:capacity"`
	Status        Status    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Warehouse) TableName() string { return "warehouses" }

// WithCounts pairs a Warehouse with its zone/location counts — what the
// Warehouse Setup table's "Zones" and "Locations" columns need, computed
// from the real child tables rather than stored/duplicated on the row.
type WithCounts struct {
	Warehouse
	ZoneCount     int64 `gorm:"column:zone_count"`
	LocationCount int64 `gorm:"column:location_count"`
}
