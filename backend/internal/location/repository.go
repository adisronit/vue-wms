package location

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("location: not found")
var ErrDuplicate = errors.New("location: code already exists in this zone")

// ListFilter scopes GET /api/v1/locations to a zone (required — the
// Location Structure tab only ever renders the tree for one open
// warehouse's zones).
type ListFilter struct {
	ZoneID int64
	Status Status
	Page   int
	Limit  int
}

// Repository is a thin GORM wrapper — business rules live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, l *Location) error {
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Location, error) {
	var l Location
	err := r.db.WithContext(ctx).First(&l, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repository) Update(ctx context.Context, l *Location) error {
	err := r.db.WithContext(ctx).Model(&Location{}).Where("id = ?", l.ID).Updates(map[string]any{
		"code":   l.Code,
		"aisle":  l.Aisle,
		"rack":   l.Rack,
		"bin":    l.Bin,
		"status": l.Status,
	}).Error
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Location{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Location, int64, error) {
	query := r.db.WithContext(ctx).Model(&Location{}).Where("zone_id = ?", filter.ZoneID)
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var locations []Location
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("code ASC").Offset(offset).Limit(filter.Limit).Find(&locations).Error; err != nil {
		return nil, 0, err
	}

	return locations, total, nil
}

// CountByZone satisfies zone.locationCounter — used to refuse deleting a
// zone that still has locations.
func (r *Repository) CountByZone(ctx context.Context, zoneID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Location{}).Where("zone_id = ?", zoneID).Count(&count).Error
	return count, err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) instead of matching
// substrings in the error message — see that package's doc for why
// (Phase 6 found live that a PRIMARY KEY violation's real message text
// matches neither "2627" nor "unique").
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}
