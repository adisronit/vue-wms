package product

import (
	"context"

	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// categoryChecker/brandChecker/familyChecker/supplierChecker are the
// narrow, tenant-scoped existence checks Create/Update need to confirm
// an optional FK belongs to the same tenant as the product itself —
// satisfied by *productcategory.Repository/*brand.Repository/
// *productfamily.Repository/*supplier.Repository's own Exists. Kept as
// local interfaces, the same depend-on-behavior-not-packages pattern
// internal/warehouse's own zones dependency already uses, so this
// package never needs to import those packages' full surface (nil-safe:
// a Service built without one, e.g. existing tests' &Service{}
// literals, simply skips that field's validation).
type categoryChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

type brandChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

type familyChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

type supplierChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

// warehouseChecker/zoneChecker are deliberately NOT tenant-scoped,
// unlike every checker above: internal/warehouse and internal/zone are
// themselves pre-existing, documented gaps (see
// internal/productcategory/repository.go's doc) with no tenant-scoped
// FindByID to call instead. preferred_warehouse_id/preferred_zone_id
// validation here can only confirm the ID exists at all, not that it
// belongs to the caller's tenant — an honest, inherited limitation,
// not a new one this package introduces. Closing it requires retrofitting
// warehouse/zone themselves, out of scope for this change.
type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

type zoneChecker interface {
	FindByID(ctx context.Context, id int64) (*zone.Zone, error)
}

// Service holds Product Master's business rules: validation and
// uniqueness-error mapping. Handlers never call Repository directly.
type Service struct {
	repo       *Repository
	categories categoryChecker
	brands     brandChecker
	families   familyChecker
	suppliers  supplierChecker
	warehouses warehouseChecker
	zones      zoneChecker

	// imageStorageDir/imageMaxBytes configure image.go's upload/serve
	// methods — see that file's own doc. Zero-value imageStorageDir
	// ("") means image upload is not configured; UploadImage returns
	// ErrValidation rather than writing to an empty/relative path.
	imageStorageDir string
	imageMaxBytes   int64
}

func NewService(repo *Repository, categories categoryChecker, brands brandChecker, families familyChecker, suppliers supplierChecker, warehouses warehouseChecker, zones zoneChecker, imageStorageDir string, imageMaxBytes int64) *Service {
	return &Service{
		repo: repo, categories: categories, brands: brands, families: families,
		suppliers: suppliers, warehouses: warehouses, zones: zones,
		imageStorageDir: imageStorageDir, imageMaxBytes: imageMaxBytes,
	}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}

	products, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, Category: params.Category, CategoryID: params.CategoryID, BrandID: params.BrandID,
		Status: params.Status, LotRequired: params.LotRequired, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(products))
	for _, p := range products {
		responses = append(responses, toResponse(p))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	p, err := s.repo.FindByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*p), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	if err := validate(req.Category, req.BaseUOM, req.Status, req.StorageType, req.RotationRule, req.WeightUnit, req.PackSize, req.MinStock, req.MaxStock); err != nil {
		return Response{}, err
	}
	if req.CategoryID == nil {
		return Response{}, ErrValidation{Message: "category_id is required"}
	}
	if err := s.validateCategoryID(ctx, tenantID, req.CategoryID); err != nil {
		return Response{}, err
	}
	if err := s.validateBrandID(ctx, tenantID, req.BrandID); err != nil {
		return Response{}, err
	}
	if err := s.validateFamilyID(ctx, tenantID, req.FamilyID); err != nil {
		return Response{}, err
	}
	if err := s.validateSupplierID(ctx, tenantID, req.DefaultSupplierID); err != nil {
		return Response{}, err
	}
	if err := s.validateWarehouseID(ctx, req.PreferredWarehouseID); err != nil {
		return Response{}, err
	}
	if err := s.validateZoneID(ctx, req.PreferredZoneID); err != nil {
		return Response{}, err
	}

	status := req.Status
	if status == "" {
		status = StatusActive
	}
	packSize := req.PackSize
	if packSize == 0 {
		packSize = 1
	}
	// Category (the legacy fixed enum) is no longer a user-facing
	// control in the redesigned form — see model.go's own doc. An
	// empty value here means the caller is the new frontend (which
	// never sends it); defaulting to PACKAGING preserves the column's
	// NOT NULL + CK_products_category contract without reintroducing
	// the control. A caller that DOES still send a value (any existing
	// integration) gets it validated and stored exactly as before.
	category := req.Category
	if category == "" {
		category = CategoryPackaging
	}

	p := &Product{
		SKU: req.SKU, Name: req.Name, NameTH: req.NameTH, Description: req.Description,
		Category: category, CategoryID: req.CategoryID, BrandID: req.BrandID, FamilyID: req.FamilyID, VariantName: req.VariantName,
		BaseUOM: req.BaseUOM, PackSize: packSize, PackSizeUnit: req.PackSizeUnit, Barcode: normalizeBarcode(req.Barcode),
		LotRequired: req.LotRequired, SerialRequired: req.SerialRequired, ExpiryRequired: req.ExpiryRequired,
		StorageType: req.StorageType, RotationRule: req.RotationRule,
		DefaultSupplierID: req.DefaultSupplierID, CountryOfOrigin: req.CountryOfOrigin,
		PreferredWarehouseID: req.PreferredWarehouseID, PreferredZoneID: req.PreferredZoneID, DefaultZone: req.DefaultZone,
		Manufacturer: req.Manufacturer, MinStock: req.MinStock, MaxStock: req.MaxStock,
		GrossWeight: req.GrossWeight, NetWeight: req.NetWeight, WeightUnit: req.WeightUnit,
		LengthMM: req.LengthMM, WidthMM: req.WidthMM, HeightMM: req.HeightMM,
		Status: status,
	}

	if err := s.repo.Create(ctx, tenantID, p); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, p.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByIDForTenant(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if err := validate(req.Category, req.BaseUOM, req.Status, req.StorageType, req.RotationRule, req.WeightUnit, req.PackSize, req.MinStock, req.MaxStock); err != nil {
		return Response{}, err
	}
	if err := s.validateCategoryID(ctx, tenantID, req.CategoryID); err != nil {
		return Response{}, err
	}
	if err := s.validateBrandID(ctx, tenantID, req.BrandID); err != nil {
		return Response{}, err
	}
	if err := s.validateFamilyID(ctx, tenantID, req.FamilyID); err != nil {
		return Response{}, err
	}
	if err := s.validateSupplierID(ctx, tenantID, req.DefaultSupplierID); err != nil {
		return Response{}, err
	}
	if err := s.validateWarehouseID(ctx, req.PreferredWarehouseID); err != nil {
		return Response{}, err
	}
	if err := s.validateZoneID(ctx, req.PreferredZoneID); err != nil {
		return Response{}, err
	}

	category := req.Category
	if category == "" {
		category = CategoryPackaging
	}

	p := &Product{
		ID: id, SKU: req.SKU, Name: req.Name, NameTH: req.NameTH, Description: req.Description,
		Category: category, CategoryID: req.CategoryID, BrandID: req.BrandID, FamilyID: req.FamilyID, VariantName: req.VariantName,
		BaseUOM: req.BaseUOM, PackSize: req.PackSize, PackSizeUnit: req.PackSizeUnit, Barcode: normalizeBarcode(req.Barcode),
		LotRequired: req.LotRequired, SerialRequired: req.SerialRequired, ExpiryRequired: req.ExpiryRequired,
		StorageType: req.StorageType, RotationRule: req.RotationRule,
		DefaultSupplierID: req.DefaultSupplierID, CountryOfOrigin: req.CountryOfOrigin,
		PreferredWarehouseID: req.PreferredWarehouseID, PreferredZoneID: req.PreferredZoneID, DefaultZone: req.DefaultZone,
		Manufacturer: req.Manufacturer, MinStock: req.MinStock, MaxStock: req.MaxStock,
		GrossWeight: req.GrossWeight, NetWeight: req.NetWeight, WeightUnit: req.WeightUnit,
		LengthMM: req.LengthMM, WidthMM: req.WidthMM, HeightMM: req.HeightMM,
		Status: req.Status,
	}

	if err := s.repo.Update(ctx, tenantID, p); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the product. There is no UI button for this today
// (Product Master only ever toggles status) and no Inventory table
// exists yet to protect against — once Inventory lands, this must gain
// the same referential-safety check warehouse/zone delete already have.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByIDForTenant(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *Service) validateCategoryID(ctx context.Context, tenantID int64, categoryID *int64) error {
	if categoryID == nil || s.categories == nil {
		return nil
	}
	exists, err := s.categories.Exists(ctx, tenantID, *categoryID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrValidation{Message: "category_id does not reference an existing product category"}
	}
	return nil
}

func (s *Service) validateBrandID(ctx context.Context, tenantID int64, brandID *int64) error {
	if brandID == nil || s.brands == nil {
		return nil
	}
	exists, err := s.brands.Exists(ctx, tenantID, *brandID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrValidation{Message: "brand_id does not reference an existing brand"}
	}
	return nil
}

func (s *Service) validateFamilyID(ctx context.Context, tenantID int64, familyID *int64) error {
	if familyID == nil || s.families == nil {
		return nil
	}
	exists, err := s.families.Exists(ctx, tenantID, *familyID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrValidation{Message: "family_id does not reference an existing product family"}
	}
	return nil
}

func (s *Service) validateSupplierID(ctx context.Context, tenantID int64, supplierID *int64) error {
	if supplierID == nil || s.suppliers == nil {
		return nil
	}
	exists, err := s.suppliers.Exists(ctx, tenantID, *supplierID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrValidation{Message: "default_supplier_id does not reference an existing supplier"}
	}
	return nil
}

func (s *Service) validateWarehouseID(ctx context.Context, warehouseID *int64) error {
	if warehouseID == nil || s.warehouses == nil {
		return nil
	}
	if _, err := s.warehouses.FindByID(ctx, *warehouseID); err != nil {
		return ErrValidation{Message: "preferred_warehouse_id does not reference an existing warehouse"}
	}
	return nil
}

func (s *Service) validateZoneID(ctx context.Context, zoneID *int64) error {
	if zoneID == nil || s.zones == nil {
		return nil
	}
	if _, err := s.zones.FindByID(ctx, *zoneID); err != nil {
		return ErrValidation{Message: "preferred_zone_id does not reference an existing zone"}
	}
	return nil
}

func validate(category Category, baseUOM UOM, status Status, storageType StorageType, rotationRule RotationRule, weightUnit WeightUnit, packSize, minStock, maxStock int) error {
	if category != "" && !category.IsValid() {
		return ErrValidation{Message: "category must be one of PACKAGING, LABEL, RFID, RAW_MATERIAL"}
	}
	if !baseUOM.IsValid() {
		return ErrValidation{Message: "base_uom must be one of PCS, BOX, ROLL, KG, G, L, ML"}
	}
	if status != "" && !status.IsValid() {
		return ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}
	if !storageType.IsValid() {
		return ErrValidation{Message: "storage_type must be AMBIENT, CHILLED, or FROZEN"}
	}
	if !rotationRule.IsValid() {
		return ErrValidation{Message: "rotation_rule must be FIFO or FEFO"}
	}
	if !weightUnit.IsValid() {
		return ErrValidation{Message: "weight_unit must be G or KG"}
	}
	if packSize < 0 {
		return ErrValidation{Message: "pack_size must not be negative"}
	}
	if minStock < 0 || maxStock < 0 {
		return ErrValidation{Message: "min_stock and max_stock must not be negative"}
	}
	if maxStock > 0 && minStock > maxStock {
		return ErrValidation{Message: "min_stock must not be greater than max_stock"}
	}
	return nil
}

// normalizeBarcode treats an empty string the same as "no barcode" —
// the frontend's barcode field is a plain text input with no
// distinction between "not filled in" and "".
func normalizeBarcode(barcode *string) *string {
	if barcode == nil || *barcode == "" {
		return nil
	}
	return barcode
}
