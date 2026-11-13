package productfamily

import "time"

// Response is what every family-returning endpoint sends back.
type Response struct {
	ID        int64     `json:"id,string" example:"1"`
	Code      string    `json:"code" example:"FAM-CHEDDAR"`
	Name      string    `json:"name" example:"Cheddar Block"`
	Status    Status    `json:"status" example:"ACTIVE"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
} //	@name	ProductFamilyResponse

// CreateRequest is the POST /api/v1/product-families body.
type CreateRequest struct {
	Code   string `json:"code" binding:"required" example:"FAM-CHEDDAR"`
	Name   string `json:"name" binding:"required" example:"Cheddar Block"`
	Status Status `json:"status" example:"ACTIVE"`
} //	@name	CreateProductFamilyRequest

// UpdateRequest is the PUT /api/v1/product-families/:id body.
type UpdateRequest struct {
	Code   string `json:"code" binding:"required" example:"FAM-CHEDDAR"`
	Name   string `json:"name" binding:"required" example:"Cheddar Block"`
	Status Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateProductFamilyRequest

// ListParams narrows GET /api/v1/product-families to the Family
// selector's search box and status filter, with pagination and sorting.
type ListParams struct {
	Search  string
	Status  Status
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

func toResponse(f Family) Response {
	return Response{ID: f.ID, Code: f.Code, Name: f.Name, Status: f.Status, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt}
}
