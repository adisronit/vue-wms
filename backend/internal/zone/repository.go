package zone

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("zone: not found")
var ErrDuplicate = errors.New("zone: code already exists in this warehouse")

// ListFilter scopes GET /api/v1/zones to a warehouse (required — zones
// are always viewed within the Warehouse Setup drawer) plus an optional
// status filter.
type ListFilter struct {
	WarehouseID int64
	Status      Status
	Page        int
	Limit       int
}

// Repository is a thin GORM wrapper — business rules (uniqueness
// messaging, warehouse-existence checks) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, z *Zone) error {
	if err := r.db.WithContext(ctx).Create(z).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Zone, error) {
	var z Zone
	err := r.db.WithContext(ctx).First(&z, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &z, nil
}

func (r *Repository) Update(ctx context.Context, z *Zone) error {
	err := r.db.WithContext(ctx).Model(&Zone{}).Where("id = ?", z.ID).Updates(map[string]any{
		"code":   z.Code,
		"name":   z.Name,
		"type":   z.Type,
		"status": z.Status,
	}).Error
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Zone{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Zone, int64, error) {
	query := r.db.WithContext(ctx).Model(&Zone{}).Where("warehouse_id = ?", filter.WarehouseID)
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var zones []Zone
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("code ASC").Offset(offset).Limit(filter.Limit).Find(&zones).Error; err != nil {
		return nil, 0, err
	}

	return zones, total, nil
}

// CountByWarehouse satisfies warehouse.zoneCounter — used to refuse
// deleting a warehouse that still has zones.
func (r *Repository) CountByWarehouse(ctx context.Context, warehouseID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Zone{}).Where("warehouse_id = ?", warehouseID).Count(&count).Error
	return count, err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — see that package's
// doc.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}
