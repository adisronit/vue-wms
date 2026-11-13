package supplier

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("supplier: not found")
var ErrDuplicateCode = errors.New("supplier: code already exists")

// ListFilter narrows GET /api/v1/suppliers to what the Suppliers
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

	var suppliers []Supplier
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("company_name ASC").Offset(offset).Limit(filter.Limit).Find(&suppliers).Error; err != nil {
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
