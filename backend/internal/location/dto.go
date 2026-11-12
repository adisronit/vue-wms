package location

import "time"

// Response is what every location-returning endpoint sends back.
type Response struct {
	ID        int64     `json:"id,string" example:"1"`
	ZoneID    int64     `json:"zone_id,string" example:"1"`
	Code      string    `json:"code" example:"A-01-01-01"`
	Aisle     string    `json:"aisle" example:"01"`
	Rack      string    `json:"rack" example:"01"`
	Bin       string    `json:"bin" example:"01"`
	Status    Status    `json:"status" example:"ACTIVE"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
} //	@name	LocationResponse

// CreateRequest is the POST /api/v1/locations body.
type CreateRequest struct {
	ZoneID int64  `json:"zone_id,string" binding:"required" example:"1"`
	Code   string `json:"code" binding:"required" example:"A-01-01-04"`
	Aisle  string `json:"aisle" example:"01"`
	Rack   string `json:"rack" example:"01"`
	Bin    string `json:"bin" example:"04"`
	Status Status `json:"status" example:"ACTIVE"`
} //	@name	CreateLocationRequest

// UpdateRequest is the PUT /api/v1/locations/:id body. ZoneID is
// intentionally not included, for the same reason zone.UpdateRequest
// omits warehouse_id.
type UpdateRequest struct {
	Code   string `json:"code" binding:"required" example:"A-01-01-04"`
	Aisle  string `json:"aisle" example:"01"`
	Rack   string `json:"rack" example:"01"`
	Bin    string `json:"bin" example:"04"`
	Status Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateLocationRequest

// ListParams narrows GET /api/v1/locations. zone_id is required.
type ListParams struct {
	ZoneID int64
	Status Status
	Page   int
	Limit  int
}

func toResponse(l Location) Response {
	return Response{
		ID: l.ID, ZoneID: l.ZoneID, Code: l.Code, Aisle: l.Aisle, Rack: l.Rack, Bin: l.Bin,
		Status: l.Status, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}
