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

// Location is the GORM model for the locations table (migrations/0007).
type Location struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	ZoneID    int64     `gorm:"column:zone_id"`
	Code      string    `gorm:"column:code"`
	Aisle     string    `gorm:"column:aisle"`
	Rack      string    `gorm:"column:rack"`
	Bin       string    `gorm:"column:bin"`
	Status    Status    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Location) TableName() string { return "locations" }
