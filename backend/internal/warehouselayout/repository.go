package warehouselayout

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("warehouselayout: not found")

// Repository runs every warehouselayout query — writes go through
// GORM struct Create/Transaction (the version/object/image rows this
// package owns); reads that need a cross-table join (warehouse name,
// zone/location reference labels) use raw SQL scoped explicitly by
// tenant_id, since warehouse.Repository.FindByID/zone.Repository.
// FindByID are NOT themselves tenant-scoped (confirmed by direct
// inspection) — this package never trusts them for a tenant boundary
// check, only its own explicitly-scoped queries.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// WarehouseName returns tenantID's own warehouse's name, or
// ErrNotFound if warehouseID does not exist or belongs to a different
// tenant — the one tenant-boundary check every write/read in this
// package gates on first.
func (r *Repository) WarehouseName(ctx context.Context, tenantID, warehouseID int64) (string, error) {
	var name string
	err := r.db.WithContext(ctx).Raw(
		"SELECT name FROM warehouses WHERE id = ? AND tenant_id = ?", warehouseID, tenantID,
	).Scan(&name).Error
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", ErrNotFound
	}
	return name, nil
}

// NextVersionNo returns the next version_no to use for a new Save —
// 1 if no version exists yet for this warehouse.
func (r *Repository) NextVersionNo(ctx context.Context, tenantID, warehouseID int64) (int, error) {
	var maxNo int
	err := r.db.WithContext(ctx).Raw(
		"SELECT COALESCE(MAX(version_no), 0) FROM warehouse_layout_versions WHERE tenant_id = ? AND warehouse_id = ?",
		tenantID, warehouseID,
	).Scan(&maxNo).Error
	if err != nil {
		return 0, err
	}
	return maxNo + 1, nil
}

// CreateVersionWithObjects creates version and every object in
// objects (Objection.LayoutVersionID is overwritten to the new
// version's ID) inside one transaction — a Save either produces a
// complete new version or nothing at all.
func (r *Repository) CreateVersionWithObjects(ctx context.Context, version *Version, objects []Object) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		for i := range objects {
			objects[i].LayoutVersionID = version.ID
		}
		if len(objects) > 0 {
			if err := tx.CreateInBatches(objects, 100).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// LatestVersion returns the highest-version_no row for this
// warehouse, or ErrNotFound if none has ever been saved.
func (r *Repository) LatestVersion(ctx context.Context, tenantID, warehouseID int64) (*Version, error) {
	var v Version
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND warehouse_id = ?", tenantID, warehouseID).
		Order("version_no DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// VersionByID returns a specific historical version, scoped to tenant
// and warehouse.
func (r *Repository) VersionByID(ctx context.Context, tenantID, warehouseID, versionID int64) (*Version, error) {
	var v Version
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND warehouse_id = ? AND id = ?", tenantID, warehouseID, versionID).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVersions returns every version saved for this warehouse, newest
// first — the history list.
func (r *Repository) ListVersions(ctx context.Context, tenantID, warehouseID int64) ([]Version, error) {
	var versions []Version
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND warehouse_id = ?", tenantID, warehouseID).
		Order("version_no DESC").Find(&versions).Error
	return versions, err
}

// ObjectsByVersion returns every shape belonging to versionID.
func (r *Repository) ObjectsByVersion(ctx context.Context, tenantID, versionID int64) ([]Object, error) {
	var objects []Object
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND layout_version_id = ?", tenantID, versionID).
		Order("z_index ASC, id ASC").Find(&objects).Error
	return objects, err
}

// ResolveReferenceLabels returns each zone/location ID's own real
// display label ("code - name"), scoped to tenantID — never trusting
// a layout object's ReferenceID alone without this tenant check, the
// same reasoning WarehouseName's own doc explains. An ID that no
// longer resolves (deleted, or belongs to another tenant) is simply
// absent from the returned map — callers fall back to the object's
// own free-text Label, never an error.
func (r *Repository) ResolveReferenceLabels(ctx context.Context, tenantID int64, zoneIDs, locationIDs []int64) (zoneLabels, locationLabels map[int64]string, err error) {
	zoneLabels = map[int64]string{}
	locationLabels = map[int64]string{}

	if len(zoneIDs) > 0 {
		var rows []struct {
			ID   int64
			Code string
			Name string
		}
		if err = r.db.WithContext(ctx).Raw(
			"SELECT id, code, name FROM zones WHERE tenant_id = ? AND id IN (?)", tenantID, zoneIDs,
		).Scan(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			zoneLabels[row.ID] = row.Code + " - " + row.Name
		}
	}

	if len(locationIDs) > 0 {
		var rows []struct {
			ID   int64
			Code string
			Name string
		}
		if err = r.db.WithContext(ctx).Raw(
			"SELECT id, code, name FROM locations WHERE tenant_id = ? AND id IN (?)", tenantID, locationIDs,
		).Scan(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			locationLabels[row.ID] = row.Code + " - " + row.Name
		}
	}

	return zoneLabels, locationLabels, nil
}

// ObjectCountsByVersions returns how many objects each version in
// versionIDs has — used by the history list so it never needs to
// fetch every object row just to show a count.
func (r *Repository) ObjectCountsByVersions(ctx context.Context, tenantID int64, versionIDs []int64) (map[int64]int64, error) {
	counts := map[int64]int64{}
	if len(versionIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		LayoutVersionID int64
		Count           int64
	}
	err := r.db.WithContext(ctx).Raw(
		"SELECT layout_version_id, COUNT(*) AS count FROM warehouse_layout_objects WHERE tenant_id = ? AND layout_version_id IN (?) GROUP BY layout_version_id",
		tenantID, versionIDs,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.LayoutVersionID] = row.Count
	}
	return counts, nil
}

// CreateImage persists a new background image's metadata.
func (r *Repository) CreateImage(ctx context.Context, img *Image) error {
	return r.db.WithContext(ctx).Create(img).Error
}

// ImageByID returns a background image's metadata, scoped to tenant
// and warehouse.
func (r *Repository) ImageByID(ctx context.Context, tenantID, warehouseID, imageID int64) (*Image, error) {
	var img Image
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND warehouse_id = ? AND id = ?", tenantID, warehouseID, imageID).
		First(&img).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &img, nil
}
