package location

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("location: not found")
var ErrDuplicate = errors.New("location: code already exists in this zone")

// ListFilter scopes GET /api/v1/locations to a zone (required — the
// Location Structure tab and the standalone Location Management page
// both always render locations grouped by an already-chosen zone).
// SortBy/SortDir arrive here already validated by Service.List against
// allowedSortColumns — sortColumns below is a second, independent
// allowlist (defense in depth, not the primary check) so this method
// can never build an ORDER BY clause from an unvalidated string even if
// a future caller skips Service.
type ListFilter struct {
	ZoneID  int64
	Search  string
	Status  Status
	Page    int
	Limit   int
	SortBy  string // one of sortColumns' keys; falls back to "code" if not
	SortDir string // "ASC" or "DESC"; falls back to "ASC" otherwise
}

// sortColumns maps List's logical SortBy key to the actual column name
// to order by — every key/value pair here is a literal, never built
// from request input, so interpolating the value into the Order()
// string below can never become a SQL-injection vector. "position"
// (aisle/rack/bin) is deliberately excluded: it's a composite display
// value the frontend assembles from three separate columns, not a
// single real column this query can ORDER BY.
var sortColumns = map[string]string{
	"code":     "code",
	"name":     "name",
	"type":     "type",
	"capacity": "capacity",
	"status":   "status",
}

// Repository is a thin GORM wrapper — business rules live in Service.
// Location Master Management: unlike this package's own pre-existing
// shape (and unlike internal/warehouse/internal/zone/internal/product,
// which still have no tenant enforcement at all — a pre-existing,
// documented gap), every method here now takes an explicit tenantID
// and applies it to every query, so a guessed or enumerated ID can
// never return another tenant's row. This also keeps every exported
// method here compliant with TestNoRepositoryMethodOmitsTenantID
// (internal/platform/tenantaudit)'s AST-based mechanical gate.
//
// CountByZone below is deliberately NOT tenant-scoped — it exists
// solely for internal/zone.Service's own (tenant-oblivious today)
// child-count check before deleting a zone, a dependency this phase
// does not touch; see that method's own doc.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, tenantID int64, l *Location) error {
	l.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Location, error) {
	var l Location
	err := r.db.WithContext(ctx).First(&l, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// FindByIDUnscoped is the pre-Location-Master-Management FindByID,
// preserved byte-for-byte under a new name so internal/receiving,
// internal/stockadjustment, and internal/stockcount's own narrow
// locationChecker interfaces (each requiring exactly
// `FindByID(ctx, id) (*Location, error)`, still none of them
// tenant-scoped — a pre-existing, documented gap unrelated to this
// phase) keep compiling against this package unchanged. Retrofitting
// tenant isolation into those three modules is out of Location Master
// Management's scope; this method exists purely to avoid forcing that
// scope creep just because this package's own FindByID gained a
// tenantID parameter.
func (r *Repository) FindByIDUnscoped(ctx context.Context, id int64) (*Location, error) {
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

// UnscopedAdapter exposes FindByIDUnscoped as a bare FindByID(ctx, id) —
// the pre-Location-Master-Management signature — so
// internal/receiving/internal/stockadjustment/internal/stockcount's own
// narrow locationChecker interfaces (each requiring exactly that single
// method) keep compiling against *Repository unchanged. cmd/server/
// main.go (and testrouter) pass this instead of the raw *Repository to
// those three NewService calls.
type UnscopedAdapter struct {
	repo *Repository
}

func NewUnscopedAdapter(repo *Repository) *UnscopedAdapter {
	return &UnscopedAdapter{repo: repo}
}

func (a *UnscopedAdapter) FindByID(ctx context.Context, id int64) (*Location, error) {
	return a.repo.FindByIDUnscoped(ctx, id)
}

func (r *Repository) Update(ctx context.Context, tenantID int64, l *Location) error {
	result := r.db.WithContext(ctx).Model(&Location{}).Where("id = ? AND tenant_id = ?", l.ID, tenantID).Updates(map[string]any{
		"code":     l.Code,
		"name":     l.Name,
		"aisle":    l.Aisle,
		"rack":     l.Rack,
		"bin":      l.Bin,
		"type":     l.Type,
		"capacity": l.Capacity,
		"status":   l.Status,
	})
	if isUniqueViolation(result.Error) {
		return ErrDuplicate
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Location{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Location, int64, error) {
	query := r.db.WithContext(ctx).Model(&Location{}).Where("tenant_id = ? AND zone_id = ?", tenantID, filter.ZoneID)

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
	// ", id ASC" is a deterministic tie-breaker: status especially has
	// only two values, and without a stable secondary key SQL Server is
	// free to return tied rows in a different order on every call, which
	// would make pagination (page 2 silently re-showing or skipping a
	// row page 1 already showed) and the frontend's sort-indicator UX
	// both flaky.
	order := fmt.Sprintf("%s %s, id ASC", column, dir)

	var locations []Location
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order(order).Offset(offset).Limit(filter.Limit).Find(&locations).Error; err != nil {
		return nil, 0, err
	}

	return locations, total, nil
}

// CountByZone satisfies zone.locationCounter — used to refuse deleting a
// zone that still has locations. Deliberately unscoped by tenant: zone's
// own Service/Delete path has no tenant context today (the same
// pre-existing gap internal/warehouse/internal/product document), and
// retrofitting that is out of Location Master Management's scope — see
// this package's own doc comment. Left byte-for-byte as it was before
// this phase.
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
