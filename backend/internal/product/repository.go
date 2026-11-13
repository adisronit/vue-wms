package product

import (
	"context"
	"errors"
	"strings"

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
	Status      Status
	LotRequired *bool
	Page        int
	Limit       int
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, p *Product) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

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

func (r *Repository) Update(ctx context.Context, p *Product) error {
	err := r.db.WithContext(ctx).Model(&Product{}).Where("id = ?", p.ID).Updates(map[string]any{
		"sku":             p.SKU,
		"name":            p.Name,
		"description":     p.Description,
		"category":        p.Category,
		"category_id":     p.CategoryID,
		"base_uom":        p.BaseUOM,
		"pack_size":       p.PackSize,
		"barcode":         p.Barcode,
		"lot_required":    p.LotRequired,
		"serial_required": p.SerialRequired,
		"expiry_required": p.ExpiryRequired,
		"default_zone":    p.DefaultZone,
		"min_stock":       p.MinStock,
		"max_stock":       p.MaxStock,
		"status":          p.Status,
	}).Error
	return translateUniqueViolation(err)
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Product{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Product, int64, error) {
	query := r.db.WithContext(ctx).Model(&Product{})

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("sku LIKE ? OR name LIKE ? OR barcode LIKE ?", like, like, like)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
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
	if err := query.Order("name ASC").Offset(offset).Limit(filter.Limit).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// CountByCategory reports how many products, within tenantID, currently
// have categoryID set as their category_id — used by
// internal/productcategory.Service.Delete's referential-safety check
// before deleting a category. Unlike every other method on this
// Repository (which, consistent with this module's pre-existing,
// documented lack of tenant enforcement — see
// internal/productcategory/repository.go's own doc — ignore tenant_id
// entirely), this one new method filters by it: it exists specifically
// to answer a question posed by a module that IS correctly
// tenant-scoped, and must not leak a count across tenants back to it.
func (r *Repository) CountByCategory(ctx context.Context, tenantID, categoryID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Product{}).
		Where("tenant_id = ? AND category_id = ?", tenantID, categoryID).
		Count(&count).Error
	return count, err
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
