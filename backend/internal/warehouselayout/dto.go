package warehouselayout

import "time"

// SaveObjectRequest is one shape within SaveLayoutRequest.Objects.
type SaveObjectRequest struct {
	ObjectType      string  `json:"object_type" binding:"required" example:"RACK"`
	ReferenceType   *string `json:"reference_type" example:"ZONE"`
	ReferenceID     *int64  `json:"reference_id,string" example:"1"`
	X               float64 `json:"x" example:"120"`
	Y               float64 `json:"y" example:"80"`
	Width           float64 `json:"width" binding:"required" example:"200"`
	Height          float64 `json:"height" binding:"required" example:"100"`
	RotationDegrees float64 `json:"rotation_degrees" example:"0"`
	FillColor       *string `json:"fill_color" example:"#fde68a"`
	StrokeColor     *string `json:"stroke_color" example:"#d97706"`
	Label           *string `json:"label" example:"Rack A1"`
	ZIndex          int     `json:"z_index" example:"0"`
} //	@name	SaveWarehouseLayoutObjectRequest

// SaveLayoutRequest is PUT /warehouses/{id}/layout — the editor's one
// "Save" action. Objects is the COMPLETE object graph for the new
// version (not a diff) — the frontend canvas always holds the full
// authoritative in-memory state, so a save simply persists all of it
// as one new version, matching every other document-style Confirm in
// this codebase's own "caller sends the full intended state" shape
// rather than a partial-patch API.
type SaveLayoutRequest struct {
	BackgroundImageID *int64              `json:"background_image_id,string" example:"1"`
	ScalePxPerMeter   *float64            `json:"scale_px_per_meter" example:"42.5"`
	Remark            *string             `json:"remark" example:"Added new receiving dock"`
	Objects           []SaveObjectRequest `json:"objects" binding:"required,dive"`
} //	@name	SaveWarehouseLayoutRequest

// ObjectResponse mirrors Object plus ReferenceLabel — the referenced
// zone/location's own real display label, resolved server-side (see
// Repository.ResolveReferenceLabels), nil whenever ReferenceID is nil
// OR no longer resolves to a real row in this tenant.
type ObjectResponse struct {
	ID              int64   `json:"id,string"`
	ObjectType      string  `json:"object_type"`
	ReferenceType   *string `json:"reference_type"`
	ReferenceID     *int64  `json:"reference_id,string"`
	ReferenceLabel  *string `json:"reference_label"`
	X               float64 `json:"x"`
	Y               float64 `json:"y"`
	Width           float64 `json:"width"`
	Height          float64 `json:"height"`
	RotationDegrees float64 `json:"rotation_degrees"`
	FillColor       *string `json:"fill_color"`
	StrokeColor     *string `json:"stroke_color"`
	Label           *string `json:"label"`
	ZIndex          int     `json:"z_index"`
} //	@name	WarehouseLayoutObjectResponse

// VersionResponse is one full layout snapshot — GET /warehouses/{id}/layout
// and GET /warehouses/{id}/layout/versions/{versionId} both return this
// shape.
type VersionResponse struct {
	ID                      int64   `json:"id,string"`
	WarehouseID             int64   `json:"warehouse_id,string"`
	VersionNo               int     `json:"version_no"`
	BackgroundImageID       *int64  `json:"background_image_id,string"`
	BackgroundImageURL      *string `json:"background_image_url"`
	BackgroundImageWidthPx  *int    `json:"background_image_width_px"`
	BackgroundImageHeightPx *int    `json:"background_image_height_px"`
	// ScalePxPerMeter is nil (render "not calibrated") when the editor
	// never performed the calibration step — never a fabricated
	// default scale.
	ScalePxPerMeter *float64         `json:"scale_px_per_meter"`
	Remark          *string          `json:"remark"`
	CreatedByName   *string          `json:"created_by_name"`
	CreatedAt       time.Time        `json:"created_at"`
	Objects         []ObjectResponse `json:"objects"`
} //	@name	WarehouseLayoutVersionResponse

// VersionSummaryResponse is one row of the version history list — no
// Objects (that would be expensive for a long history; fetch a
// specific version's own full detail via GetVersion when needed).
type VersionSummaryResponse struct {
	ID            int64     `json:"id,string"`
	VersionNo     int       `json:"version_no"`
	Remark        *string   `json:"remark"`
	CreatedByName *string   `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
	ObjectCount   int64     `json:"object_count"`
} //	@name	WarehouseLayoutVersionSummaryResponse

// ImageResponse is the result of a background image upload.
type ImageResponse struct {
	ID       int64  `json:"id,string"`
	URL      string `json:"url"`
	WidthPx  int    `json:"width_px"`
	HeightPx int    `json:"height_px"`
} //	@name	WarehouseLayoutImageResponse
