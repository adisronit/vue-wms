package product

import "time"

// Response is what every product-returning endpoint sends back.
type Response struct {
	ID             int64     `json:"id,string" example:"1"`
	SKU            string    `json:"sku" example:"SKU-0001"`
	Name           string    `json:"name" example:"Barcode Label Roll"`
	Description    string    `json:"description"`
	Category       Category  `json:"category" example:"LABEL"`
	CategoryID     *int64    `json:"category_id,string" example:"null"`
	BaseUOM        UOM       `json:"base_uom" example:"ROLL"`
	PackSize       int       `json:"pack_size" example:"1"`
	Barcode        *string   `json:"barcode"`
	LotRequired    bool      `json:"lot_required"`
	SerialRequired bool      `json:"serial_required"`
	ExpiryRequired bool      `json:"expiry_required"`
	DefaultZone    string    `json:"default_zone" example:"A"`
	MinStock       int       `json:"min_stock" example:"10"`
	MaxStock       int       `json:"max_stock" example:"1000"`
	Status         Status    `json:"status" example:"ACTIVE"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
} //	@name	ProductResponse

// CreateRequest is the POST /api/v1/products body.
type CreateRequest struct {
	SKU            string   `json:"sku" binding:"required" example:"SKU-0003"`
	Name           string   `json:"name" binding:"required" example:"Shrink Wrap Roll"`
	Description    string   `json:"description" example:"Industrial shrink wrap, 500mm"`
	Category       Category `json:"category" binding:"required" example:"PACKAGING"`
	CategoryID     *int64   `json:"category_id,string" example:"null"`
	BaseUOM        UOM      `json:"base_uom" binding:"required" example:"ROLL"`
	PackSize       int      `json:"pack_size" example:"1"`
	Barcode        *string  `json:"barcode" example:"885000001234"`
	LotRequired    bool     `json:"lot_required"`
	SerialRequired bool     `json:"serial_required"`
	ExpiryRequired bool     `json:"expiry_required"`
	DefaultZone    string   `json:"default_zone" example:"A"`
	MinStock       int      `json:"min_stock" example:"10"`
	MaxStock       int      `json:"max_stock" example:"1000"`
	Status         Status   `json:"status" example:"ACTIVE"`
} //	@name	CreateProductRequest

// UpdateRequest is the PUT /api/v1/products/:id body.
type UpdateRequest struct {
	SKU            string   `json:"sku" binding:"required" example:"SKU-0003"`
	Name           string   `json:"name" binding:"required" example:"Shrink Wrap Roll"`
	Description    string   `json:"description" example:"Industrial shrink wrap, 500mm"`
	Category       Category `json:"category" binding:"required" example:"PACKAGING"`
	CategoryID     *int64   `json:"category_id,string" example:"null"`
	BaseUOM        UOM      `json:"base_uom" binding:"required" example:"ROLL"`
	PackSize       int      `json:"pack_size" example:"1"`
	Barcode        *string  `json:"barcode" example:"885000001234"`
	LotRequired    bool     `json:"lot_required"`
	SerialRequired bool     `json:"serial_required"`
	ExpiryRequired bool     `json:"expiry_required"`
	DefaultZone    string   `json:"default_zone" example:"A"`
	MinStock       int      `json:"min_stock" example:"10"`
	MaxStock       int      `json:"max_stock" example:"1000"`
	Status         Status   `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateProductRequest

// ListParams narrows GET /api/v1/products to the Product Master
// screen's actual search box plus category/status/lot-rule filters,
// with pagination.
type ListParams struct {
	Search      string
	Category    Category
	Status      Status
	LotRequired *bool
	Page        int
	Limit       int
}

func toResponse(p Product) Response {
	return Response{
		ID: p.ID, SKU: p.SKU, Name: p.Name, Description: p.Description,
		Category: p.Category, CategoryID: p.CategoryID, BaseUOM: p.BaseUOM, PackSize: p.PackSize, Barcode: p.Barcode,
		LotRequired: p.LotRequired, SerialRequired: p.SerialRequired, ExpiryRequired: p.ExpiryRequired,
		DefaultZone: p.DefaultZone, MinStock: p.MinStock, MaxStock: p.MaxStock, Status: p.Status,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
