package warehouse

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("warehouse: not found")
var ErrDuplicate = errors.New("warehouse: code already exists")

// ListFilter narrows GET /api/v1/warehouses to what the Warehouse Setup
// screen's search box and type/status filters actually send.
type ListFilter struct {
	Search string // matched against code, name, address (LIKE)
	Type   string
	Status Status
	Page   int
	Limit  int
}

const countSelect = `warehouses.*,
	(SELECT COUNT(*) FROM zones WHERE zones.warehouse_id = warehouses.id) AS zone_count,
	(SELECT COUNT(*) FROM locations
		JOIN zones ON locations.zone_id = zones.id
		WHERE zones.warehouse_id = warehouses.id) AS location_count`

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging, referential-safety checks) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, w *Warehouse) error {
	if err := r.db.WithContext(ctx).Create(w).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Warehouse, error) {
	var w Warehouse
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// FindWithCounts returns id's warehouse plus its live zone/location
// counts, for GET/POST/PUT single-resource responses.
func (r *Repository) FindWithCounts(ctx context.Context, id int64) (*WithCounts, error) {
	var w WithCounts
	err := r.db.WithContext(ctx).Table("warehouses").
		Select(countSelect).
		Where("warehouses.id = ?", id).
		Take(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// FindByIDs returns the warehouse rows matching ids (order not
// guaranteed to follow ids — callers needing a stable display order get
// one via the code ASC ordering here), so callers can detect IDs that
// don't exist (len(result) < len(ids)). Used by internal/role's
// warehouse-access validation/resolution — see its warehouseLookup
// interface.
func (r *Repository) FindByIDs(ctx context.Context, ids []int64) ([]Warehouse, error) {
	var warehouses []Warehouse
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("code ASC").Find(&warehouses).Error
	return warehouses, err
}

func (r *Repository) Update(ctx context.Context, w *Warehouse) error {
	err := r.db.WithContext(ctx).Model(&Warehouse{}).Where("id = ?", w.ID).Updates(map[string]any{
		"code":           w.Code,
		"name":           w.Name,
		"type":           w.Type,
		"address":        w.Address,
		"contact_person": w.ContactPerson,
		"phone":          w.Phone,
		"capacity":       w.Capacity,
		"status":         w.Status,
	}).Error
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Warehouse{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]WithCounts, int64, error) {
	base := r.db.WithContext(ctx).Table("warehouses")

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		base = base.Where("code LIKE ? OR name LIKE ? OR address LIKE ?", like, like, like)
	}
	if filter.Type != "" {
		base = base.Where("type = ?", filter.Type)
	}
	if filter.Status != "" {
		base = base.Where("status = ?", filter.Status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var warehouses []WithCounts
	offset := (filter.Page - 1) * filter.Limit
	if err := base.Select(countSelect).Order("name ASC").Offset(offset).Limit(filter.Limit).Find(&warehouses).Error; err != nil {
		return nil, 0, err
	}

	return warehouses, total, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — see that package's
// doc.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}
