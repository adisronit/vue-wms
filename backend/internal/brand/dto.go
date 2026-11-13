package brand

import "time"

// Response is what every brand-returning endpoint sends back.
type Response struct {
	ID        int64     `json:"id,string" example:"1"`
	Code      string    `json:"code" example:"BR-ANCHOR"`
	Name      string    `json:"name" example:"Anchor"`
	Status    Status    `json:"status" example:"ACTIVE"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
} //	@name	BrandResponse

// CreateRequest is the POST /api/v1/brands body.
type CreateRequest struct {
	Code   string `json:"code" binding:"required" example:"BR-ANCHOR"`
	Name   string `json:"name" binding:"required" example:"Anchor"`
	Status Status `json:"status" example:"ACTIVE"`
} //	@name	CreateBrandRequest

// UpdateRequest is the PUT /api/v1/brands/:id body.
type UpdateRequest struct {
	Code   string `json:"code" binding:"required" example:"BR-ANCHOR"`
	Name   string `json:"name" binding:"required" example:"Anchor"`
	Status Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateBrandRequest

// ListParams narrows GET /api/v1/brands to the Brand selector's search
// box and status filter, with pagination and sorting.
type ListParams struct {
	Search  string
	Status  Status
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

func toResponse(b Brand) Response {
	return Response{ID: b.ID, Code: b.Code, Name: b.Name, Status: b.Status, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}
