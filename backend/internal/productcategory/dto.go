package productcategory

import "time"

// Response is what every category-returning endpoint sends back.
type Response struct {
	ID          int64     `json:"id,string" example:"1"`
	Code        string    `json:"code" example:"CAT-PKG"`
	Name        string    `json:"name" example:"Packaging Materials"`
	ParentID    *int64    `json:"parent_id,string" example:"null"`
	Description string    `json:"description"`
	Status      Status    `json:"status" example:"ACTIVE"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
} //	@name	ProductCategoryResponse

// CreateRequest is the POST /api/v1/product-categories body.
type CreateRequest struct {
	Code        string `json:"code" binding:"required" example:"CAT-PKG"`
	Name        string `json:"name" binding:"required" example:"Packaging Materials"`
	ParentID    *int64 `json:"parent_id,string" example:"null"`
	Description string `json:"description" example:"Boxes, wraps, and void fill"`
	Status      Status `json:"status" example:"ACTIVE"`
} //	@name	CreateProductCategoryRequest

// UpdateRequest is the PUT /api/v1/product-categories/:id body.
type UpdateRequest struct {
	Code        string `json:"code" binding:"required" example:"CAT-PKG"`
	Name        string `json:"name" binding:"required" example:"Packaging Materials"`
	ParentID    *int64 `json:"parent_id,string" example:"null"`
	Description string `json:"description" example:"Boxes, wraps, and void fill"`
	Status      Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateProductCategoryRequest

// ListParams narrows GET /api/v1/product-categories to the Product
// Category screen's search box, parent filter, and status filter, with
// pagination and sorting. SortBy/SortDir are the raw query values —
// Service.List is what validates them against allowedSortColumns and
// defaults them (code/ASC) before Repository ever sees them.
type ListParams struct {
	Search   string
	ParentID *int64
	Status   Status
	Page     int
	Limit    int
	SortBy   string
	SortDir  string
}

func toResponse(c Category) Response {
	return Response{
		ID: c.ID, Code: c.Code, Name: c.Name, ParentID: c.ParentID,
		Description: c.Description, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
