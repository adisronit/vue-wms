package warehouselayout

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/user"
)

// ErrValidation mirrors every other package's own validation error
// shape (e.g. internal/qualitycheck.ErrValidation) — the handler maps
// it to HTTP 422.
type ErrValidation struct{ Message string }

func (e ErrValidation) Error() string { return e.Message }

type userFinder interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service orchestrates Repository with the one cross-package lookup
// this feature needs (resolving CreatedBy to a display name) — every
// warehouse/zone/location tenant-boundary check is this package's own
// (see Repository's own doc), never delegated to those packages'
// un-tenant-scoped FindByID methods.
type Service struct {
	repo  *Repository
	users userFinder

	imageStorageDir string
	imageMaxBytes   int64
}

func NewService(repo *Repository, users userFinder, imageStorageDir string, imageMaxBytes int64) *Service {
	return &Service{repo: repo, users: users, imageStorageDir: imageStorageDir, imageMaxBytes: imageMaxBytes}
}

// GetLayout returns the latest saved version for warehouseID, or nil
// (not an error) if nothing has ever been saved yet — a brand-new
// warehouse legitimately has no layout, which is not a failure.
func (s *Service) GetLayout(ctx context.Context, tenantID, warehouseID int64) (*VersionResponse, error) {
	if _, err := s.repo.WarehouseName(ctx, tenantID, warehouseID); err != nil {
		return nil, err
	}
	v, err := s.repo.LatestVersion(ctx, tenantID, warehouseID)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return s.buildVersionResponse(ctx, tenantID, v)
}

// GetVersion returns one specific historical version, read-only.
func (s *Service) GetVersion(ctx context.Context, tenantID, warehouseID, versionID int64) (*VersionResponse, error) {
	if _, err := s.repo.WarehouseName(ctx, tenantID, warehouseID); err != nil {
		return nil, err
	}
	v, err := s.repo.VersionByID(ctx, tenantID, warehouseID, versionID)
	if err != nil {
		return nil, err
	}
	return s.buildVersionResponse(ctx, tenantID, v)
}

// ListVersions returns the version history, newest first.
func (s *Service) ListVersions(ctx context.Context, tenantID, warehouseID int64) ([]VersionSummaryResponse, error) {
	if _, err := s.repo.WarehouseName(ctx, tenantID, warehouseID); err != nil {
		return nil, err
	}
	versions, err := s.repo.ListVersions(ctx, tenantID, warehouseID)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(versions))
	for _, v := range versions {
		ids = append(ids, v.ID)
	}
	counts, err := s.repo.ObjectCountsByVersions(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}

	userIDs := make([]int64, 0, len(versions))
	for _, v := range versions {
		if v.CreatedBy != nil {
			userIDs = append(userIDs, *v.CreatedBy)
		}
	}
	names := s.resolveUserNames(ctx, userIDs)

	out := make([]VersionSummaryResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, VersionSummaryResponse{
			ID: v.ID, VersionNo: v.VersionNo, Remark: v.Remark, CreatedAt: v.CreatedAt,
			CreatedByName: nameFor(v.CreatedBy, names),
			ObjectCount:   counts[v.ID],
		})
	}
	return out, nil
}

// SaveLayout validates req and persists it as a brand-new version —
// see package doc for why this always creates a new row rather than
// updating the previous one in place.
func (s *Service) SaveLayout(ctx context.Context, tenantID, warehouseID int64, req SaveLayoutRequest, userID int64) (*VersionResponse, error) {
	if _, err := s.repo.WarehouseName(ctx, tenantID, warehouseID); err != nil {
		return nil, err
	}
	if req.ScalePxPerMeter != nil && *req.ScalePxPerMeter <= 0 {
		return nil, ErrValidation{Message: "scale_px_per_meter must be greater than zero"}
	}
	if req.BackgroundImageID != nil {
		if _, err := s.repo.ImageByID(ctx, tenantID, warehouseID, *req.BackgroundImageID); err != nil {
			if err == ErrNotFound {
				return nil, ErrValidation{Message: "background_image_id does not belong to this warehouse"}
			}
			return nil, err
		}
	}

	objects := make([]Object, 0, len(req.Objects))
	for i, o := range req.Objects {
		objType := ObjectType(o.ObjectType)
		if !objType.IsValid() {
			return nil, ErrValidation{Message: fmt.Sprintf("objects[%d]: invalid object_type %q", i, o.ObjectType)}
		}
		if o.Width <= 0 || o.Height <= 0 {
			return nil, ErrValidation{Message: fmt.Sprintf("objects[%d]: width and height must be greater than zero", i)}
		}
		var refType *string
		if o.ReferenceType != nil {
			rt := ReferenceType(*o.ReferenceType)
			if !rt.IsValid() {
				return nil, ErrValidation{Message: fmt.Sprintf("objects[%d]: invalid reference_type %q", i, *o.ReferenceType)}
			}
			refType = o.ReferenceType
		}
		objects = append(objects, Object{
			TenantID: tenantID, ObjectType: objType,
			ReferenceType: refType, ReferenceID: o.ReferenceID,
			X: o.X, Y: o.Y, Width: o.Width, Height: o.Height, RotationDegrees: o.RotationDegrees,
			FillColor: o.FillColor, StrokeColor: o.StrokeColor, Label: o.Label, ZIndex: o.ZIndex,
		})
	}

	nextNo, err := s.repo.NextVersionNo(ctx, tenantID, warehouseID)
	if err != nil {
		return nil, err
	}
	v := &Version{
		TenantID: tenantID, WarehouseID: warehouseID, VersionNo: nextNo,
		BackgroundImageID: req.BackgroundImageID, ScalePxPerMeter: req.ScalePxPerMeter, Remark: req.Remark,
		CreatedBy: &userID, CreatedAt: time.Now().UTC(),
	}
	if req.BackgroundImageID != nil {
		img, err := s.repo.ImageByID(ctx, tenantID, warehouseID, *req.BackgroundImageID)
		if err != nil {
			return nil, err
		}
		w, h := img.WidthPx, img.HeightPx
		v.BackgroundImageWidthPx = &w
		v.BackgroundImageHeightPx = &h
	}

	if err := s.repo.CreateVersionWithObjects(ctx, v, objects); err != nil {
		return nil, err
	}
	return s.buildVersionResponse(ctx, tenantID, v)
}

func (s *Service) buildVersionResponse(ctx context.Context, tenantID int64, v *Version) (*VersionResponse, error) {
	objects, err := s.repo.ObjectsByVersion(ctx, tenantID, v.ID)
	if err != nil {
		return nil, err
	}

	var zoneIDs, locationIDs []int64
	for _, o := range objects {
		if o.ReferenceType == nil || o.ReferenceID == nil {
			continue
		}
		switch ReferenceType(*o.ReferenceType) {
		case ReferenceTypeZone:
			zoneIDs = append(zoneIDs, *o.ReferenceID)
		case ReferenceTypeLocation:
			locationIDs = append(locationIDs, *o.ReferenceID)
		}
	}
	zoneLabels, locationLabels, err := s.repo.ResolveReferenceLabels(ctx, tenantID, zoneIDs, locationIDs)
	if err != nil {
		return nil, err
	}

	objResponses := make([]ObjectResponse, 0, len(objects))
	for _, o := range objects {
		var label *string
		if o.ReferenceType != nil && o.ReferenceID != nil {
			var resolved string
			var ok bool
			switch ReferenceType(*o.ReferenceType) {
			case ReferenceTypeZone:
				resolved, ok = zoneLabels[*o.ReferenceID]
			case ReferenceTypeLocation:
				resolved, ok = locationLabels[*o.ReferenceID]
			}
			if ok {
				label = &resolved
			}
		}
		objResponses = append(objResponses, ObjectResponse{
			ID: o.ID, ObjectType: string(o.ObjectType),
			ReferenceType: o.ReferenceType, ReferenceID: o.ReferenceID, ReferenceLabel: label,
			X: o.X, Y: o.Y, Width: o.Width, Height: o.Height, RotationDegrees: o.RotationDegrees,
			FillColor: o.FillColor, StrokeColor: o.StrokeColor, Label: o.Label, ZIndex: o.ZIndex,
		})
	}

	var createdByName *string
	if v.CreatedBy != nil {
		names := s.resolveUserNames(ctx, []int64{*v.CreatedBy})
		createdByName = nameFor(v.CreatedBy, names)
	}

	var imageURL *string
	if v.BackgroundImageID != nil {
		u := fmt.Sprintf("/api/v1/warehouses/%d/layout/images/%d", v.WarehouseID, *v.BackgroundImageID)
		imageURL = &u
	}

	return &VersionResponse{
		ID: v.ID, WarehouseID: v.WarehouseID, VersionNo: v.VersionNo,
		BackgroundImageID: v.BackgroundImageID, BackgroundImageURL: imageURL,
		BackgroundImageWidthPx: v.BackgroundImageWidthPx, BackgroundImageHeightPx: v.BackgroundImageHeightPx,
		ScalePxPerMeter: v.ScalePxPerMeter, Remark: v.Remark,
		CreatedByName: createdByName, CreatedAt: v.CreatedAt,
		Objects: objResponses,
	}, nil
}

func (s *Service) resolveUserNames(ctx context.Context, ids []int64) map[int64]string {
	names := map[int64]string{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if u, err := s.users.FindByID(ctx, id); err == nil && u != nil {
			names[id] = u.FullName
		}
	}
	return names
}

func nameFor(id *int64, names map[int64]string) *string {
	if id == nil {
		return nil
	}
	if n, ok := names[*id]; ok {
		return &n
	}
	return nil
}
