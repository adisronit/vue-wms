package productcategory

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("productcategory: not found")
var ErrDuplicateCode = errors.New("productcategory: code already exists")

// ListFilter narrows GET /api/v1/product-categories to what the
// Product Category screen's search box, parent filter, and status
// filter actually send.
type ListFilter struct {
	Search   string // matched against code, name (LIKE)
	ParentID *int64
	Status   Status
	Page     int
	Limit    int
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging, circular-hierarchy prevention, referential-safety checks)
// live in Service. Unlike internal/product and internal/warehouse's
// repositories (which never filter by tenant at all — a pre-existing,
// documented gap, see internal/platform/tenantaudit's own package doc
// and backend/test/isolation/README.md), every method here takes an
// explicit tenantID and applies it to every query, so a guessed or
// enumerated ID can never return another tenant's row. This also keeps
// every exported method here compliant with
// TestNoRepositoryMethodOmitsTenantID (internal/platform/tenantaudit),
// the AST-based mechanical gate that fails the build if a Repository
// method with a context.Context first parameter has no parameter whose
// name mentions "tenant".
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, c *Category) error {
	c.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all", so
// a guessed ID can't be distinguished from a nonexistent one
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Category, error) {
	var c Category
	err := r.db.WithContext(ctx).First(&c, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, c *Category) error {
	result := r.db.WithContext(ctx).Model(&Category{}).
		Where("id = ? AND tenant_id = ?", c.ID, tenantID).
		Updates(map[string]any{
			"code":        c.Code,
			"name":        c.Name,
			"parent_id":   c.ParentID,
			"description": c.Description,
			"status":      c.Status,
		})
	if result.Error != nil {
		return translateUniqueViolation(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Category{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Category, int64, error) {
	query := r.db.WithContext(ctx).Model(&Category{}).Where("tenant_id = ?", tenantID)

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	if filter.ParentID != nil {
		query = query.Where("parent_id = ?", *filter.ParentID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var categories []Category
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("name ASC").Offset(offset).Limit(filter.Limit).Find(&categories).Error; err != nil {
		return nil, 0, err
	}

	return categories, total, nil
}

// Exists reports whether id exists within tenantID — the ownership
// check internal/product.Service.validateCategoryID uses to confirm a
// product's optional category_id belongs to the same tenant as the
// product itself, rather than a bare existence check that would let
// one tenant's product reference another tenant's category.
func (r *Repository) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Category{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CountByParent reports how many categories, within tenantID, currently
// have parentID as their parent — used by Service.Delete to block
// deleting a category that still has children (the same
// "CountByWarehouse"/"CountByZone" referential-safety shape
// internal/warehouse and internal/zone already use before their own
// Delete).
func (r *Repository) CountByParent(ctx context.Context, tenantID, parentID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Category{}).
		Where("tenant_id = ? AND parent_id = ?", tenantID, parentID).
		Count(&count).Error
	return count, err
}

func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateCode
}
