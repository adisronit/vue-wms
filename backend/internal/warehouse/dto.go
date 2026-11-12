package warehouse

import "time"

// Response is what every warehouse-returning endpoint sends back.
// Utilization is deliberately not included: it would need real
// inventory quantities (occupied vs. capacity), which don't exist yet —
// see PROJECT_STATUS.md. The frontend defaults to 0 in its absence.
type Response struct {
	ID            int64     `json:"id,string" example:"1"`
	Code          string    `json:"code" example:"WH-01"`
	Name          string    `json:"name" example:"Bangkok Warehouse"`
	Type          string    `json:"type" example:"Distribution Center"`
	Address       string    `json:"address"`
	ContactPerson string    `json:"contact_person"`
	Phone         string    `json:"phone"`
	Capacity      *int      `json:"capacity"`
	Status        Status    `json:"status" example:"ACTIVE"`
	ZoneCount     int64     `json:"zones" example:"3"`
	LocationCount int64     `json:"locations" example:"120"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
} //	@name	WarehouseResponse

// CreateRequest is the POST /api/v1/warehouses body.
type CreateRequest struct {
	Code          string `json:"code" binding:"required" example:"WH-04"`
	Name          string `json:"name" binding:"required" example:"Chiang Mai Warehouse"`
	Type          string `json:"type" binding:"required" example:"Distribution Center"`
	Address       string `json:"address" example:"Chiang Mai"`
	ContactPerson string `json:"contact_person" example:"Warehouse Manager"`
	Phone         string `json:"phone" example:"053-000-000"`
	Capacity      *int   `json:"capacity" example:"5000"`
	Status        Status `json:"status" example:"ACTIVE"`
} //	@name	CreateWarehouseRequest

// UpdateRequest is the PUT /api/v1/warehouses/:id body.
type UpdateRequest struct {
	Code          string `json:"code" binding:"required" example:"WH-04"`
	Name          string `json:"name" binding:"required" example:"Chiang Mai Warehouse"`
	Type          string `json:"type" binding:"required" example:"Distribution Center"`
	Address       string `json:"address" example:"Chiang Mai"`
	ContactPerson string `json:"contact_person" example:"Warehouse Manager"`
	Phone         string `json:"phone" example:"053-000-000"`
	Capacity      *int   `json:"capacity" example:"5000"`
	Status        Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateWarehouseRequest

// ListParams narrows GET /api/v1/warehouses to the Warehouse Setup
// screen's actual search box plus type/status filters, with pagination.
type ListParams struct {
	Search string
	Type   string
	Status Status
	Page   int
	Limit  int
}

func toResponse(w WithCounts) Response {
	return Response{
		ID:            w.ID,
		Code:          w.Code,
		Name:          w.Name,
		Type:          w.Type,
		Address:       w.Address,
		ContactPerson: w.ContactPerson,
		Phone:         w.Phone,
		Capacity:      w.Capacity,
		Status:        w.Status,
		ZoneCount:     w.ZoneCount,
		LocationCount: w.LocationCount,
		CreatedAt:     w.CreatedAt,
		UpdatedAt:     w.UpdatedAt,
	}
}
