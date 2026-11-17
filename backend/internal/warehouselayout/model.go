// Package warehouselayout implements Phase 34I's Interactive 2D
// Warehouse Layout — a purely visual/spatial overlay over a
// warehouse's real zones/locations. Unlike every other Phase 34
// package, this one has a real write path (an authorized editor can
// save shapes, positions, and an optional calibrated background
// image) — but it NEVER mutates warehouses/zones/locations/inventory.
// A layout object may optionally reference a real zone or location row
// (ReferenceType/ReferenceID) purely so the viewer can display that
// row's own real code/name; deleting or renaming the referenced row
// is never blocked and never cascades here — a dangling reference
// simply falls back to the shape's own free-text Label.
//
// Every Save creates a brand-new Version row (never updates one in
// place) — the highest VersionNo per warehouse is always what the
// viewer shows by default; every older version stays readable as
// history, matching the "new data model for layout objects/versions"
// requirement without a separate DRAFT/PUBLISHED workflow, which
// would add real complexity this feature does not need (there is
// exactly one mutable "current" state per warehouse, not a multi-
// stage approval flow).
//
// RBAC uses its own "layout" module (view/update — see
// internal/rbac/catalog.go), independent of report.view/export: this
// is an editor with real write access to warehouse_layout_* tables,
// not a read-only report, so conflating the two permissions would let
// a report-viewer role silently gain write access (or vice versa).
package warehouselayout

import "time"

// ObjectType is the closed set of shapes a layout object may be.
type ObjectType string

const (
	ObjectTypeZone     ObjectType = "ZONE"
	ObjectTypeLocation ObjectType = "LOCATION"
	ObjectTypeRack     ObjectType = "RACK"
	ObjectTypeAisle    ObjectType = "AISLE"
	ObjectTypeDock     ObjectType = "DOCK"
	ObjectTypeWall     ObjectType = "WALL"
	ObjectTypeLabel    ObjectType = "LABEL"
	ObjectTypeShape    ObjectType = "SHAPE"
)

func (t ObjectType) IsValid() bool {
	switch t {
	case ObjectTypeZone, ObjectTypeLocation, ObjectTypeRack, ObjectTypeAisle,
		ObjectTypeDock, ObjectTypeWall, ObjectTypeLabel, ObjectTypeShape:
		return true
	default:
		return false
	}
}

// ReferenceType is the closed set of real master-data tables an object
// may optionally point at for display purposes only.
type ReferenceType string

const (
	ReferenceTypeZone     ReferenceType = "ZONE"
	ReferenceTypeLocation ReferenceType = "LOCATION"
)

func (t ReferenceType) IsValid() bool {
	switch t {
	case ReferenceTypeZone, ReferenceTypeLocation:
		return true
	default:
		return false
	}
}

// Version is the GORM model for the warehouse_layout_versions table
// (migrations/0120) — one saved snapshot of a warehouse's layout.
type Version struct {
	ID                      int64     `gorm:"column:id;primaryKey"`
	TenantID                int64     `gorm:"column:tenant_id;not null"`
	WarehouseID             int64     `gorm:"column:warehouse_id"`
	VersionNo               int       `gorm:"column:version_no"`
	BackgroundImageID       *int64    `gorm:"column:background_image_id"`
	BackgroundImageWidthPx  *int      `gorm:"column:background_image_width_px"`
	BackgroundImageHeightPx *int      `gorm:"column:background_image_height_px"`
	ScalePxPerMeter         *float64  `gorm:"column:scale_px_per_meter"`
	Remark                  *string   `gorm:"column:remark"`
	CreatedBy               *int64    `gorm:"column:created_by"`
	CreatedAt               time.Time `gorm:"column:created_at"`
}

func (Version) TableName() string { return "warehouse_layout_versions" }

// Object is the GORM model for the warehouse_layout_objects table
// (migrations/0121) — one drawn shape belonging to exactly one
// Version. X/Y is the shape's own top-left corner; RotationDegrees is
// applied about its own center by the frontend renderer only, never
// persisted pre-rotated.
type Object struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	TenantID        int64      `gorm:"column:tenant_id;not null"`
	LayoutVersionID int64      `gorm:"column:layout_version_id"`
	ObjectType      ObjectType `gorm:"column:object_type"`
	ReferenceType   *string    `gorm:"column:reference_type"`
	ReferenceID     *int64     `gorm:"column:reference_id"`
	X               float64    `gorm:"column:x"`
	Y               float64    `gorm:"column:y"`
	Width           float64    `gorm:"column:width"`
	Height          float64    `gorm:"column:height"`
	RotationDegrees float64    `gorm:"column:rotation_degrees"`
	FillColor       *string    `gorm:"column:fill_color"`
	StrokeColor     *string    `gorm:"column:stroke_color"`
	Label           *string    `gorm:"column:label"`
	ZIndex          int        `gorm:"column:z_index"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
}

func (Object) TableName() string { return "warehouse_layout_objects" }

// Image is the GORM model for the warehouse_layout_images table
// (migrations/0122) — metadata only; the actual bytes live on disk
// under config.ImageStorageDir, reusing internal/product's own
// image.go upload/serve pattern. JPG/PNG only (not WebP, unlike
// product images) — this package needs each image's real pixel
// dimensions for on-canvas calibration, and decoding those requires
// only the Go standard library's image/jpeg and image/png packages;
// adding golang.org/x/image/webp purely to read WebP headers was
// judged not worth a new dependency for this one feature (a
// deliberate scope decision, not an oversight — product images stay
// JPG/PNG/WebP as before, unaffected).
type Image struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	TenantID         int64     `gorm:"column:tenant_id;not null"`
	WarehouseID      int64     `gorm:"column:warehouse_id"`
	FileName         string    `gorm:"column:file_name"`
	OriginalFilename *string   `gorm:"column:original_filename"`
	ContentType      string    `gorm:"column:content_type"`
	ByteSize         int       `gorm:"column:byte_size"`
	WidthPx          int       `gorm:"column:width_px"`
	HeightPx         int       `gorm:"column:height_px"`
	CreatedBy        *int64    `gorm:"column:created_by"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (Image) TableName() string { return "warehouse_layout_images" }
