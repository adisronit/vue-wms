package zone

import "time"

// Response is what every zone-returning endpoint sends back.
type Response struct {
	ID          int64     `json:"id,string" example:"1"`
	WarehouseID int64     `json:"warehouse_id,string" example:"1"`
	Code        string    `json:"code" example:"A"`
	Name        string    `json:"name" example:"Storage Zone A"`
	Type        Type      `json:"type" example:"STORAGE"`
	Status      Status    `json:"status" example:"ACTIVE"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
} //	@name	ZoneResponse

// CreateRequest is the POST /api/v1/zones body.
type CreateRequest struct {
	WarehouseID int64  `json:"warehouse_id,string" binding:"required" example:"1"`
	Code        string `json:"code" binding:"required" example:"D"`
	Name        string `json:"name" binding:"required" example:"Hold Zone D"`
	Type        Type   `json:"type" binding:"required" example:"HOLD"`
	Status      Status `json:"status" example:"ACTIVE"`
} //	@name	CreateZoneRequest

// UpdateRequest is the PUT /api/v1/zones/:id body. WarehouseID is
// intentionally not included — a zone does not move between warehouses
// after creation in this phase; the frontend's Zone Setup tab always
// edits zones scoped to one open warehouse drawer.
type UpdateRequest struct {
	Code   string `json:"code" binding:"required" example:"D"`
	Name   string `json:"name" binding:"required" example:"Hold Zone D"`
	Type   Type   `json:"type" binding:"required" example:"HOLD"`
	Status Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateZoneRequest

// ListParams narrows GET /api/v1/zones. warehouse_id is required — the
// Zone Setup tab only ever lists zones for one open warehouse.
type ListParams struct {
	WarehouseID int64
	Status      Status
	Page        int
	Limit       int
}

func toResponse(z Zone) Response {
	return Response{
		ID: z.ID, WarehouseID: z.WarehouseID, Code: z.Code, Name: z.Name,
		Type: z.Type, Status: z.Status, CreatedAt: z.CreatedAt, UpdatedAt: z.UpdatedAt,
	}
}
