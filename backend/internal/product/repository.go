package product

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("product: not found")
var ErrDuplicateSKU = errors.New("product: sku already exists")
var ErrDuplicateBarcode = errors.New("product: barcode already exists")

// ListFilter narrows GET /api/v1/products to what the Product Master
// screen's search box and category/status/lot-rule filters actually
// send.
type ListFilter struct {
	Search      string // matched against sku, name, barcode (LIKE)
	Category    Category
	CategoryID  *int64
	BrandID     *int64
	Status      Status
	LotRequired *bool
	Page        int
	Limit       int
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging) live in Service.
//
// The EWTA Product Master enhancement closes this package's own,
// previously pre-existing and documented tenant-enforcement gap (see
// internal/productcategory/repository.go's doc, and this package's own
// CountByCategory, the one method that already filtered by tenant_id
// before this change): every method below now takes an explicit
// tenantID and applies it to every query, the same discipline
// internal/productcategory/internal/supplier/internal/brand already
// use. This was required for the EWTA requirement that product SKUs be
// uniquely scoped to tenant to actually hold at the application layer,
// not just in the database's own UQ_products_tenant_sku constraint
// (migrations/0048) — a caller could otherwise still read/update/
// delete another tenant's product by guessing its ID.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, p *Product) error {
	p.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID looks up id with NO tenant filter — kept only for
// receiving/putaway/picking/stockadjustment/stockcount/replenishment/
// order's own narrow productChecker interfaces, none of which have
// tenant context themselves (out of this phase's scope, the same
// pre-existing gap internal/location.FindByIDUnscoped's own doc
// documents for the identical reason). New tenant-facing code — like
// this package's own Service — must use FindByIDForTenant instead.
func (r *Repository) FindByID(ctx context.Context, id int64) (*Product, error) {
	var p Product
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByIDForTenant returns id's row scoped to tenantID, or
// ErrNotFound — never "found but belongs to another tenant" vs.
// "doesn't exist at all" (tenant-isolation-security.md §6). Every
// product.Service method backing a user-supplied :id must go through
// this, not FindByID.
func (r *Repository) FindByIDForTenant(ctx context.Context, tenantID, id int64) (*Product, error) {
	var p Product
	err := r.db.WithContext(ctx).First(&p, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, p *Product) error {
	err := r.db.WithContext(ctx).Model(&Product{}).Where("id = ? AND tenant_id = ?", p.ID, tenantID).Updates(map[string]any{
		"sku":                    p.SKU,
		"name":                   p.Name,
		"name_th":                p.NameTH,
		"description":            p.Description,
		"category":               p.Category,
		"category_id":            p.CategoryID,
		"brand_id":               p.BrandID,
		"family_id":              p.FamilyID,
		"variant_name":           p.VariantName,
		"base_uom":               p.BaseUOM,
		"pack_size":              p.PackSize,
		"pack_size_unit":         p.PackSizeUnit,
		"barcode":                p.Barcode,
		"lot_required":           p.LotRequired,
		"serial_required":        p.SerialRequired,
		"expiry_required":        p.ExpiryRequired,
		"storage_type":           p.StorageType,
		"rotation_rule":          p.RotationRule,
		"default_supplier_id":    p.DefaultSupplierID,
		"country_of_origin":      p.CountryOfOrigin,
		"preferred_warehouse_id": p.PreferredWarehouseID,
		"preferred_zone_id":      p.PreferredZoneID,
		"default_zone":           p.DefaultZone,
		"manufacturer":           p.Manufacturer,
		"min_stock":              p.MinStock,
		"max_stock":              p.MaxStock,
		"gross_weight":           p.GrossWeight,
		"net_weight":             p.NetWeight,
		"weight_unit":            p.WeightUnit,
		"length_mm":              p.LengthMM,
		"width_mm":               p.WidthMM,
		"height_mm":              p.HeightMM,
		"status":                 p.Status,
	}).Error
	return translateUniqueViolation(err)
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Product{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Product, int64, error) {
	query := r.db.WithContext(ctx).Model(&Product{}).Where("tenant_id = ?", tenantID)

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("sku LIKE ? OR name LIKE ? OR barcode LIKE ?", like, like, like)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.CategoryID != nil {
		query = query.Where("category_id = ?", *filter.CategoryID)
	}
	if filter.BrandID != nil {
		query = query.Where("brand_id = ?", *filter.BrandID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.LotRequired != nil {
		query = query.Where("lot_required = ?", *filter.LotRequired)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []Product
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("name ASC, id ASC").Offset(offset).Limit(filter.Limit).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// CountByCategory reports how many products, within tenantID, currently
// have categoryID set as their category_id — used by
// internal/productcategory.Service.Delete's referential-safety check
// before deleting a category.
func (r *Repository) CountByCategory(ctx context.Context, tenantID, categoryID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Product{}).
		Where("tenant_id = ? AND category_id = ?", tenantID, categoryID).
		Count(&count).Error
	return count, err
}

// CountByBrand reports how many products, within tenantID, currently
// have brandID set — exists for a future Brand-delete referential-
// safety check; FK_products_brand itself already rejects deleting a
// brand still referenced (see internal/brand.Service.Delete's doc), so
// this is not yet called anywhere, kept only for symmetry with
// CountByCategory should that change.
func (r *Repository) CountByBrand(ctx context.Context, tenantID, brandID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Product{}).
		Where("tenant_id = ? AND brand_id = ?", tenantID, brandID).
		Count(&count).Error
	return count, err
}

// SetImage atomically replaces id's image metadata and returns the
// previous image_path (nil if it had none), so the caller can delete
// the old file from disk only after this DB update has actually
// committed — never the other way around, which could leave a product
// pointing at a file that no longer exists if the DB write then failed.
func (r *Repository) SetImage(ctx context.Context, tenantID, id int64, path, originalName, mimeType string, sizeBytes int, uploadedAt time.Time) (*string, error) {
	var previous *string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Product
		if err := tx.Select("image_path").First(&p, "id = ? AND tenant_id = ?", id, tenantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		previous = p.ImagePath

		result := tx.Model(&Product{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(map[string]any{
			"image_path":          path,
			"image_original_name": originalName,
			"image_mime_type":     mimeType,
			"image_size_bytes":    sizeBytes,
			"image_uploaded_at":   uploadedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return previous, nil
}

// ClearImage removes id's image metadata and returns the previous
// image_path (nil if it already had none) for the same
// delete-file-after-commit reason SetImage documents.
func (r *Repository) ClearImage(ctx context.Context, tenantID, id int64) (*string, error) {
	var previous *string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Product
		if err := tx.Select("image_path").First(&p, "id = ? AND tenant_id = ?", id, tenantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		previous = p.ImagePath

		result := tx.Model(&Product{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(map[string]any{
			"image_path": nil, "image_original_name": nil, "image_mime_type": nil,
			"image_size_bytes": nil, "image_uploaded_at": nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return previous, nil
}

// ListBarcodes, CreateBarcode, UpdateBarcode, DeleteBarcode all join
// against products so a productID belonging to another tenant can
// never return or mutate that tenant's barcode rows — product_barcodes
// itself carries no tenant_id column (see barcode_model.go's own doc),
// so this JOIN is the sole enforcement point, mirroring
// internal/role.Repository.RoleWarehouseAccessIDs' identical shape.

func (r *Repository) ListBarcodes(ctx context.Context, tenantID, productID int64) ([]ProductBarcode, error) {
	var barcodes []ProductBarcode
	err := r.db.WithContext(ctx).Model(&ProductBarcode{}).
		Joins("JOIN products ON products.id = product_barcodes.product_id").
		Where("product_barcodes.product_id = ? AND products.tenant_id = ?", productID, tenantID).
		Order("product_barcodes.is_primary DESC, product_barcodes.id ASC").
		Find(&barcodes).Error
	return barcodes, err
}

func (r *Repository) productOwnedByTenant(tx *gorm.DB, tenantID, productID int64) (bool, error) {
	var count int64
	err := tx.Model(&Product{}).Where("id = ? AND tenant_id = ?", productID, tenantID).Count(&count).Error
	return count > 0, err
}

func (r *Repository) CreateBarcode(ctx context.Context, tenantID, productID int64, b *ProductBarcode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owned, err := r.productOwnedByTenant(tx, tenantID, productID)
		if err != nil {
			return err
		}
		if !owned {
			return ErrNotFound
		}
		b.ProductID = productID
		if err := tx.Create(b).Error; err != nil {
			return translateBarcodeUniqueViolation(err)
		}
		return nil
	})
}

func (r *Repository) UpdateBarcode(ctx context.Context, tenantID, productID, barcodeID int64, b *ProductBarcode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owned, err := r.productOwnedByTenant(tx, tenantID, productID)
		if err != nil {
			return err
		}
		if !owned {
			return ErrNotFound
		}
		result := tx.Model(&ProductBarcode{}).
			Where("id = ? AND product_id = ?", barcodeID, productID).
			Updates(map[string]any{
				"barcode_value":   b.BarcodeValue,
				"barcode_type":    b.BarcodeType,
				"uom":             b.UOM,
				"qty_per_barcode": b.QtyPerBarcode,
				"is_primary":      b.IsPrimary,
				"status":          b.Status,
			})
		if result.Error != nil {
			return translateBarcodeUniqueViolation(result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *Repository) DeleteBarcode(ctx context.Context, tenantID, productID, barcodeID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owned, err := r.productOwnedByTenant(tx, tenantID, productID)
		if err != nil {
			return err
		}
		if !owned {
			return ErrNotFound
		}
		result := tx.Delete(&ProductBarcode{}, "id = ? AND product_id = ?", barcodeID, productID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// translateUniqueViolation maps a SQL Server unique-constraint error to
// the specific sentinel the violated column implies. The error NUMBER
// (via internal/platform/database/sqlerr) gates whether this is a
// unique violation at all; the error number alone can't say *which*
// constraint fired (SQL Server doesn't have a separate number per named
// constraint), so which column is still resolved by inspecting the
// constraint name in the message — but only once IsUniqueViolation has
// already confirmed the error class, instead of an unguarded substring
// search that could match "barcode"/"sku" appearing in an unrelated
// error message.
func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "barcode") {
		return ErrDuplicateBarcode
	}
	if strings.Contains(msg, "sku") {
		return ErrDuplicateSKU
	}
	return err
}

// ErrDuplicateBarcodeMapping covers UQ_product_barcodes_product_value —
// a different constraint from the single-barcode column's own
// UQ_products_barcode (translateUniqueViolation above), so it needs its
// own sentinel and translator.
var ErrDuplicateBarcodeMapping = errors.New("product: this barcode is already mapped to this product")

func translateBarcodeUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateBarcodeMapping
}
