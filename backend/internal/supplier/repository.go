package supplier

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("supplier: not found")
var ErrDuplicateCode = errors.New("supplier: code already exists")

// ListFilter narrows GET /api/v1/suppliers to what the Suppliers
// screen's search box and status filter actually send. SortBy/SortDir
// arrive here already validated by Service.List against
// allowedSortColumns — sortColumns below is a second, independent
// allowlist (defense in depth, not the primary check) so this method
// can never build an ORDER BY clause from an unvalidated string even if
// a future caller skips Service — see internal/productcategory's own
// ListFilter doc for the original rationale.
type ListFilter struct {
	Search  string // matched against code, company_name (LIKE)
	Status  Status
	Page    int
	Limit   int
	SortBy  string // one of sortColumns' keys; falls back to "code" if not
	SortDir string // "ASC" or "DESC"; falls back to "ASC" otherwise
}

// sortColumns maps List's logical SortBy key to the actual column name
// to order by — every key/value pair here is a literal, never built
// from request input, so interpolating the value into the Order()
// string below can never become a SQL-injection vector.
var sortColumns = map[string]string{
	"code":        "code",
	"companyName": "company_name",
	"status":      "status",
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging) live in Service. See internal/productcategory/repository.go's
// doc for why every method here takes an explicit tenantID and applies
// it to every query, unlike internal/product/internal/warehouse.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, s *Supplier) error {
	s.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Supplier, error) {
	var s Supplier
	err := r.db.WithContext(ctx).First(&s, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Exists reports whether id exists within tenantID — the ownership
// check internal/product.Service uses to confirm a product's optional
// default_supplier_id belongs to the same tenant as the product
// itself, mirroring internal/productcategory.Repository's own Exists.
func (r *Repository) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Supplier{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, s *Supplier) error {
	result := r.db.WithContext(ctx).Model(&Supplier{}).
		Where("id = ? AND tenant_id = ?", s.ID, tenantID).
		Updates(map[string]any{
			"code":           s.Code,
			"company_name":   s.CompanyName,
			"tax_id":         s.TaxID,
			"contact_person": s.ContactPerson,
			"contact_phone":  s.ContactPhone,
			"address":        s.Address,
			"email":          s.Email,
			"phone":          s.Phone,
			"status":         s.Status,
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
	result := r.db.WithContext(ctx).Delete(&Supplier{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Supplier, int64, error) {
	query := r.db.WithContext(ctx).Model(&Supplier{}).Where("tenant_id = ?", tenantID)

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("code LIKE ? OR company_name LIKE ?", like, like)
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
	// ", id ASC" is a deterministic tie-breaker: company_name/status can
	// repeat (status especially — only two values), and without a
	// stable secondary key SQL Server is free to return tied rows in a
	// different order on every call, which would make pagination (page
	// 2 silently re-showing or skipping a row page 1 already showed)
	// and the frontend's sort-indicator UX both flaky.
	order := fmt.Sprintf("%s %s, id ASC", column, dir)

	var suppliers []Supplier
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order(order).Offset(offset).Limit(filter.Limit).Find(&suppliers).Error; err != nil {
		return nil, 0, err
	}

	return suppliers, total, nil
}

func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateCode
}
