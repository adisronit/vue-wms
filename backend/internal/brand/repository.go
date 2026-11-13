package brand

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("brand: not found")
var ErrDuplicateCode = errors.New("brand: code already exists")

// ListFilter narrows GET /api/v1/brands to the Brand selector's search
// box and status filter, with pagination and sorting.
type ListFilter struct {
	Search  string // matched against code, name (LIKE)
	Status  Status
	Page    int
	Limit   int
	SortBy  string // one of sortColumns' keys; falls back to "code" if not
	SortDir string // "ASC" or "DESC"; falls back to "ASC" otherwise
}

// sortColumns maps List's logical SortBy key to the actual column name
// to order by — see internal/productcategory/repository.go's own doc
// for why this exists as an independent allowlist from Service's.
var sortColumns = map[string]string{
	"code":   "code",
	"name":   "name",
	"status": "status",
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging) live in Service. Every method takes an explicit tenantID
// and applies it to every query, the same discipline
// internal/productcategory/repository.go's own doc documents.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, b *Brand) error {
	b.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(b).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Brand, error) {
	var b Brand
	err := r.db.WithContext(ctx).First(&b, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, b *Brand) error {
	result := r.db.WithContext(ctx).Model(&Brand{}).
		Where("id = ? AND tenant_id = ?", b.ID, tenantID).
		Updates(map[string]any{"code": b.Code, "name": b.Name, "status": b.Status})
	if result.Error != nil {
		return translateUniqueViolation(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Brand{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Brand, int64, error) {
	query := r.db.WithContext(ctx).Model(&Brand{}).Where("tenant_id = ?", tenantID)

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	column, ok := sortColumns[filter.SortBy]
	if !ok {
		column = sortColumns["code"]
	}
	dir := "ASC"
	if filter.SortDir == "DESC" {
		dir = "DESC"
	}
	order := fmt.Sprintf("%s %s, id ASC", column, dir)

	var brands []Brand
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order(order).Offset(offset).Limit(filter.Limit).Find(&brands).Error; err != nil {
		return nil, 0, err
	}

	return brands, total, nil
}

// Exists reports whether id exists within tenantID — the ownership
// check internal/product.Service uses to confirm a product's optional
// brand_id belongs to the same tenant as the product itself, mirroring
// internal/productcategory.Repository's own Exists.
func (r *Repository) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Brand{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateCode
}
