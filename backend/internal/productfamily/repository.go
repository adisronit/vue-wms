package productfamily

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("productfamily: not found")
var ErrDuplicateCode = errors.New("productfamily: code already exists")

// ListFilter narrows GET /api/v1/product-families to the Family
// selector's search box and status filter, with pagination and sorting.
type ListFilter struct {
	Search  string // matched against code, name (LIKE)
	Status  Status
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

var sortColumns = map[string]string{
	"code":   "code",
	"name":   "name",
	"status": "status",
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging) live in Service. Every method takes an explicit tenantID
// and applies it to every query, mirroring internal/brand.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, f *Family) error {
	f.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(f).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Family, error) {
	var f Family
	err := r.db.WithContext(ctx).First(&f, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, f *Family) error {
	result := r.db.WithContext(ctx).Model(&Family{}).
		Where("id = ? AND tenant_id = ?", f.ID, tenantID).
		Updates(map[string]any{"code": f.Code, "name": f.Name, "status": f.Status})
	if result.Error != nil {
		return translateUniqueViolation(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Family{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Family, int64, error) {
	query := r.db.WithContext(ctx).Model(&Family{}).Where("tenant_id = ?", tenantID)

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

	var families []Family
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order(order).Offset(offset).Limit(filter.Limit).Find(&families).Error; err != nil {
		return nil, 0, err
	}

	return families, total, nil
}

// Exists reports whether id exists within tenantID — the ownership
// check internal/product.Service uses to confirm a product's optional
// family_id belongs to the same tenant as the product itself.
func (r *Repository) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Family{}).
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
