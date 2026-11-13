package product

import "time"

// Response is what every product-returning endpoint sends back.
type Response struct {
	ID          int64    `json:"id,string" example:"1"`
	SKU         string   `json:"sku" example:"SKU-0001"`
	Name        string   `json:"name" example:"Barcode Label Roll"`
	NameTH      *string  `json:"name_th"`
	Description string   `json:"description"`
	Category    Category `json:"category" example:"LABEL"`
	CategoryID  *int64   `json:"category_id,string" example:"null"`
	BrandID     *int64   `json:"brand_id,string" example:"null"`
	FamilyID    *int64   `json:"family_id,string" example:"null"`
	VariantName string   `json:"variant_name"`

	BaseUOM      UOM     `json:"base_uom" example:"ROLL"`
	PackSize     int     `json:"pack_size" example:"1"`
	PackSizeUnit string  `json:"pack_size_unit"`
	Barcode      *string `json:"barcode"`

	LotRequired    bool         `json:"lot_required"`
	SerialRequired bool         `json:"serial_required"`
	ExpiryRequired bool         `json:"expiry_required"`
	StorageType    StorageType  `json:"storage_type"`
	RotationRule   RotationRule `json:"rotation_rule"`

	DefaultSupplierID    *int64 `json:"default_supplier_id,string" example:"null"`
	CountryOfOrigin      string `json:"country_of_origin"`
	PreferredWarehouseID *int64 `json:"preferred_warehouse_id,string" example:"null"`
	PreferredZoneID      *int64 `json:"preferred_zone_id,string" example:"null"`
	DefaultZone          string `json:"default_zone" example:"A"`

	Manufacturer string     `json:"manufacturer"`
	MinStock     int        `json:"min_stock" example:"10"`
	MaxStock     int        `json:"max_stock" example:"1000"`
	GrossWeight  *float64   `json:"gross_weight"`
	NetWeight    *float64   `json:"net_weight"`
	WeightUnit   WeightUnit `json:"weight_unit"`
	LengthMM     *float64   `json:"length_mm"`
	WidthMM      *float64   `json:"width_mm"`
	HeightMM     *float64   `json:"height_mm"`

	// HasImage/ImageURL are derived, not stored columns — see
	// toResponse's own comment for why ImageURL is always the stable,
	// handler-mediated serve path, never the raw disk path.
	HasImage          bool    `json:"has_image"`
	ImageURL          *string `json:"image_url"`
	ImageOriginalName *string `json:"image_original_name"`

	Status    Status    `json:"status" example:"ACTIVE"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
} //	@name	ProductResponse

// CreateRequest is the POST /api/v1/products body. Category keeps its
// json tag for full backward compatibility with any existing caller
// that still sends it, but is no longer binding:"required" — see
// model.go's own Category doc for why; Service.Create defaults an
// empty value to CategoryPackaging rather than rejecting the request.
type CreateRequest struct {
	SKU         string   `json:"sku" binding:"required" example:"SKU-0003"`
	Name        string   `json:"name" binding:"required" example:"Cheddar Block 150g"`
	NameTH      *string  `json:"name_th"`
	Description string   `json:"description" example:"Imported cheddar cheese block"`
	Category    Category `json:"category" example:"PACKAGING"`
	// CategoryID is the one category control the EWTA-redesigned form's
	// Basic Information section shows, and is required at the Service
	// layer for new products (not at JSON-binding level, since "required"
	// can't express "required unless omitted for a nil pointer" the way
	// Service's own explicit check can, and Service's error message is
	// more specific than gin's generic binding failure).
	CategoryID  *int64 `json:"category_id,string" example:"null"`
	BrandID     *int64 `json:"brand_id,string" example:"null"`
	FamilyID    *int64 `json:"family_id,string" example:"null"`
	VariantName string `json:"variant_name" example:"150g Block"`

	BaseUOM      UOM     `json:"base_uom" binding:"required" example:"G"`
	PackSize     int     `json:"pack_size" example:"150"`
	PackSizeUnit string  `json:"pack_size_unit" example:"G"`
	Barcode      *string `json:"barcode" example:"885000001234"`

	LotRequired    bool         `json:"lot_required"`
	SerialRequired bool         `json:"serial_required"`
	ExpiryRequired bool         `json:"expiry_required"`
	StorageType    StorageType  `json:"storage_type" example:"CHILLED"`
	RotationRule   RotationRule `json:"rotation_rule" example:"FEFO"`

	DefaultSupplierID    *int64 `json:"default_supplier_id,string" example:"null"`
	CountryOfOrigin      string `json:"country_of_origin" example:"New Zealand"`
	PreferredWarehouseID *int64 `json:"preferred_warehouse_id,string" example:"null"`
	PreferredZoneID      *int64 `json:"preferred_zone_id,string" example:"null"`
	DefaultZone          string `json:"default_zone" example:"A"`

	Manufacturer string     `json:"manufacturer"`
	MinStock     int        `json:"min_stock" example:"10"`
	MaxStock     int        `json:"max_stock" example:"1000"`
	GrossWeight  *float64   `json:"gross_weight"`
	NetWeight    *float64   `json:"net_weight"`
	WeightUnit   WeightUnit `json:"weight_unit" example:"G"`
	LengthMM     *float64   `json:"length_mm"`
	WidthMM      *float64   `json:"width_mm"`
	HeightMM     *float64   `json:"height_mm"`

	Status Status `json:"status" example:"ACTIVE"`
} //	@name	CreateProductRequest

// UpdateRequest is the PUT /api/v1/products/:id body — same shape as
// CreateRequest, see that type's own field docs.
type UpdateRequest struct {
	SKU         string   `json:"sku" binding:"required" example:"SKU-0003"`
	Name        string   `json:"name" binding:"required" example:"Cheddar Block 150g"`
	NameTH      *string  `json:"name_th"`
	Description string   `json:"description" example:"Imported cheddar cheese block"`
	Category    Category `json:"category" example:"PACKAGING"`
	CategoryID  *int64   `json:"category_id,string" example:"null"`
	BrandID     *int64   `json:"brand_id,string" example:"null"`
	FamilyID    *int64   `json:"family_id,string" example:"null"`
	VariantName string   `json:"variant_name" example:"150g Block"`

	BaseUOM      UOM     `json:"base_uom" binding:"required" example:"G"`
	PackSize     int     `json:"pack_size" example:"150"`
	PackSizeUnit string  `json:"pack_size_unit" example:"G"`
	Barcode      *string `json:"barcode" example:"885000001234"`

	LotRequired    bool         `json:"lot_required"`
	SerialRequired bool         `json:"serial_required"`
	ExpiryRequired bool         `json:"expiry_required"`
	StorageType    StorageType  `json:"storage_type" example:"CHILLED"`
	RotationRule   RotationRule `json:"rotation_rule" example:"FEFO"`

	DefaultSupplierID    *int64 `json:"default_supplier_id,string" example:"null"`
	CountryOfOrigin      string `json:"country_of_origin" example:"New Zealand"`
	PreferredWarehouseID *int64 `json:"preferred_warehouse_id,string" example:"null"`
	PreferredZoneID      *int64 `json:"preferred_zone_id,string" example:"null"`
	DefaultZone          string `json:"default_zone" example:"A"`

	Manufacturer string     `json:"manufacturer"`
	MinStock     int        `json:"min_stock" example:"10"`
	MaxStock     int        `json:"max_stock" example:"1000"`
	GrossWeight  *float64   `json:"gross_weight"`
	NetWeight    *float64   `json:"net_weight"`
	WeightUnit   WeightUnit `json:"weight_unit" example:"G"`
	LengthMM     *float64   `json:"length_mm"`
	WidthMM      *float64   `json:"width_mm"`
	HeightMM     *float64   `json:"height_mm"`

	Status Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateProductRequest

// ListParams narrows GET /api/v1/products to the Product Master
// screen's actual search box plus category/status/lot-rule filters,
// with pagination.
type ListParams struct {
	Search      string
	Category    Category
	CategoryID  *int64
	BrandID     *int64
	Status      Status
	LotRequired *bool
	Page        int
	Limit       int
}

// toResponse builds the response the EWTA-redesigned form's image
// widget needs: HasImage is a plain boolean (no path/filename leaked),
// and ImageURL — when set — is always the handler-mediated
// GET /api/v1/products/:id/image path, never p.ImagePath itself (that's
// a server-internal disk path, not something any client should ever
// see or be able to request directly — see image.go's Serve doc for
// why the real file lives behind an ownership-checked handler, not a
// static mount).
func toResponse(p Product) Response {
	resp := Response{
		ID: p.ID, SKU: p.SKU, Name: p.Name, NameTH: p.NameTH, Description: p.Description,
		Category: p.Category, CategoryID: p.CategoryID, BrandID: p.BrandID, FamilyID: p.FamilyID, VariantName: p.VariantName,
		BaseUOM: p.BaseUOM, PackSize: p.PackSize, PackSizeUnit: p.PackSizeUnit, Barcode: p.Barcode,
		LotRequired: p.LotRequired, SerialRequired: p.SerialRequired, ExpiryRequired: p.ExpiryRequired,
		StorageType: p.StorageType, RotationRule: p.RotationRule,
		DefaultSupplierID: p.DefaultSupplierID, CountryOfOrigin: p.CountryOfOrigin,
		PreferredWarehouseID: p.PreferredWarehouseID, PreferredZoneID: p.PreferredZoneID, DefaultZone: p.DefaultZone,
		Manufacturer: p.Manufacturer, MinStock: p.MinStock, MaxStock: p.MaxStock,
		GrossWeight: p.GrossWeight, NetWeight: p.NetWeight, WeightUnit: p.WeightUnit,
		LengthMM: p.LengthMM, WidthMM: p.WidthMM, HeightMM: p.HeightMM,
		ImageOriginalName: p.ImageOriginalName,
		Status:            p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if p.ImagePath != nil && *p.ImagePath != "" {
		resp.HasImage = true
		url := imageURL(p.ID)
		resp.ImageURL = &url
	}
	return resp
}

// BarcodeResponse is what every barcode-mapping-returning endpoint
// sends back.
type BarcodeResponse struct {
	ID            int64       `json:"id,string" example:"1"`
	ProductID     int64       `json:"product_id,string" example:"1"`
	BarcodeValue  string      `json:"barcode_value" example:"885000001234"`
	BarcodeType   BarcodeType `json:"barcode_type" example:"SUPPLIER_BARCODE"`
	UOM           string      `json:"uom" example:"BOX"`
	QtyPerBarcode int         `json:"qty_per_barcode" example:"24"`
	IsPrimary     bool        `json:"is_primary"`
	Status        Status      `json:"status" example:"ACTIVE"`
} //	@name	ProductBarcodeResponse

// CreateBarcodeRequest is the POST /api/v1/products/:id/barcodes body.
type CreateBarcodeRequest struct {
	BarcodeValue  string      `json:"barcode_value" binding:"required" example:"885000001234"`
	BarcodeType   BarcodeType `json:"barcode_type" example:"SUPPLIER_BARCODE"`
	UOM           string      `json:"uom" example:"BOX"`
	QtyPerBarcode int         `json:"qty_per_barcode" example:"24"`
	IsPrimary     bool        `json:"is_primary"`
	Status        Status      `json:"status" example:"ACTIVE"`
} //	@name	CreateProductBarcodeRequest

// UpdateBarcodeRequest is the PUT /api/v1/products/:id/barcodes/:barcodeId body.
type UpdateBarcodeRequest struct {
	BarcodeValue  string      `json:"barcode_value" binding:"required" example:"885000001234"`
	BarcodeType   BarcodeType `json:"barcode_type" example:"SUPPLIER_BARCODE"`
	UOM           string      `json:"uom" example:"BOX"`
	QtyPerBarcode int         `json:"qty_per_barcode" example:"24"`
	IsPrimary     bool        `json:"is_primary"`
	Status        Status      `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateProductBarcodeRequest

func toBarcodeResponse(b ProductBarcode) BarcodeResponse {
	return BarcodeResponse{
		ID: b.ID, ProductID: b.ProductID, BarcodeValue: b.BarcodeValue, BarcodeType: b.BarcodeType,
		UOM: b.UOM, QtyPerBarcode: b.QtyPerBarcode, IsPrimary: b.IsPrimary, Status: b.Status,
	}
}
