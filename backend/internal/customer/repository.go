package customer

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("customer: not found")
var ErrDuplicateCode = errors.New("customer: code already exists")

// ListFilter narrows GET /api/v1/customers to what the Customers
// screen's search box and status filter actually send.
type ListFilter struct {
	Search string // matched against code, company_name (LIKE)
	Status Status
	Page   int
	Limit  int
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

func (r *Repository) Create(ctx context.Context, tenantID int64, c *Customer) error {
	c.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Customer, error) {
	var c Customer
	err := r.db.WithContext(ctx).First(&c, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, c *Customer) error {
	result := r.db.WithContext(ctx).Model(&Customer{}).
		Where("id = ? AND tenant_id = ?", c.ID, tenantID).
		Updates(map[string]any{
			"code":           c.Code,
			"company_name":   c.CompanyName,
			"tax_id":         c.TaxID,
			"contact_person": c.ContactPerson,
			"contact_phone":  c.ContactPhone,
			"address":        c.Address,
			"email":          c.Email,
			"phone":          c.Phone,
			"status":         c.Status,
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
	result := r.db.WithContext(ctx).Delete(&Customer{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Customer, int64, error) {
	query := r.db.WithContext(ctx).Model(&Customer{}).Where("tenant_id = ?", tenantID)

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

	var customers []Customer
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("company_name ASC").Offset(offset).Limit(filter.Limit).Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateCode
}
