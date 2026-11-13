package location

import (
	"context"
	"errors"
	"strings"

	"wms-backend/internal/zone"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrZoneNotFound is returned when a request's zone_id doesn't match any
// zone — including a zone that exists but belongs to a different
// tenant, which must read identically to "doesn't exist at all"
// (tenant-isolation-security.md §6: never 403, only 404, so a guessed
// ID can't be distinguished from a nonexistent one).
var ErrZoneNotFound = errors.New("location: zone not found")

// ErrHasStock is returned when deleting a location that still carries
// on-hand inventory (Location Master Management addition — this
// package had no referential-safety check on Delete at all before this
// phase, unlike internal/warehouse/internal/zone/internal/productcategory's
// own child-count checks).
var ErrHasStock = errors.New("location: cannot delete — stock still exists in this location")

// ErrInvalidSort is returned when sort_by/sort_dir is present but not
// one of the values the Location Master screen's sortable column
// headers can actually send — a 400, not a silent fallback to the
// default sort, so a frontend bug (or a hand-crafted request) surfaces
// immediately instead of quietly returning results in the wrong order.
// Mirrors internal/productcategory.ErrInvalidSort exactly.
var ErrInvalidSort = errors.New("location: invalid sort_by or sort_dir")

// defaultSortBy/defaultSortDir match the Location Master screen's
// default view and this package's own pre-existing fixed "code ASC"
// order, so an omitted sort_by/sort_dir changes nothing about today's
// behavior.
const (
	defaultSortBy  = "code"
	defaultSortDir = "ASC"
)

// allowedSortColumns is the sort-column allowlist GET
// /api/v1/locations' sort_by validates against — the same set
// repository.go's sortColumns maps to real column names. "position"
// (aisle/rack/bin) is deliberately excluded: it's a composite display
// value assembled client-side from three separate columns, not a
// single real column the database can ORDER BY. Kept here, not just in
// Repository, so an invalid value is rejected with ErrInvalidSort
// before Service.List does anything else, rather than silently falling
// back deep in the Repository call.
var allowedSortColumns = map[string]bool{
	"code":     true,
	"name":     true,
	"type":     true,
	"capacity": true,
	"status":   true,
}

// zoneChecker is the narrow slice of *zone.Repository this package
// needs: confirming zone_id exists (and, per Location Master
// Management, belongs to the caller's own tenant) before creating a
// location under it. Importing package zone here is safe — the
// dependency chain is warehouse <- zone <- location, one direction
// only.
type zoneChecker interface {
	FindByID(ctx context.Context, id int64) (*zone.Zone, error)
}

// inventoryCounter is the narrow capability Service.Delete needs from
// internal/inventory — satisfied by *inventory.Repository's
// CountByLocation. Kept as a local interface, the same
// depend-on-behavior-not-packages pattern internal/product's own
// categoryChecker already uses, and nil-safe (Delete skips the check
// when this Service was built without one) for the same reason: so
// existing/future test code constructing a bare &Service{} doesn't
// need to satisfy it.
type inventoryCounter interface {
	CountByLocation(ctx context.Context, tenantID, locationID int64) (int64, error)
}

// Service holds Location Structure's business rules: validation,
// tenant-scoped zone-ownership checks, and (new) a referential-safety
// check before delete. Handlers never call Repository directly.
type Service struct {
	repo      *Repository
	zones     zoneChecker
	inventory inventoryCounter
}

func NewService(repo *Repository, zones zoneChecker, inventory inventoryCounter) *Service {
	return &Service{repo: repo, zones: zones, inventory: inventory}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 500
	}

	sortBy := defaultSortBy
	if raw := strings.ToLower(strings.TrimSpace(params.SortBy)); raw != "" {
		if !allowedSortColumns[raw] {
			return nil, 0, ErrInvalidSort
		}
		sortBy = raw
	}

	sortDir := defaultSortDir
	if raw := strings.ToUpper(strings.TrimSpace(params.SortDir)); raw != "" {
		if raw != "ASC" && raw != "DESC" {
			return nil, 0, ErrInvalidSort
		}
		sortDir = raw
	}

	locations, total, err := s.repo.List(ctx, tenantID, ListFilter{
		ZoneID: params.ZoneID, Search: params.Search, Status: params.Status,
		Page: page, Limit: limit, SortBy: sortBy, SortDir: sortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(locations))
	for _, l := range locations {
		responses = append(responses, toResponse(l))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	l, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*l), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}
	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be one of RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD"}
	}
	if req.Capacity != nil && *req.Capacity < 0 {
		return Response{}, ErrValidation{Message: "capacity must not be negative"}
	}

	if err := s.validateZoneOwnership(ctx, tenantID, req.ZoneID); err != nil {
		return Response{}, err
	}

	l := &Location{
		ZoneID: req.ZoneID, Code: req.Code, Name: req.Name, Aisle: req.Aisle, Rack: req.Rack, Bin: req.Bin,
		Type: req.Type, Capacity: req.Capacity, Status: status,
	}
	if err := s.repo.Create(ctx, tenantID, l); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, l.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}
	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be one of RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD"}
	}
	if req.Capacity != nil && *req.Capacity < 0 {
		return Response{}, ErrValidation{Message: "capacity must not be negative"}
	}

	l := &Location{
		ID: id, Code: req.Code, Name: req.Name, Aisle: req.Aisle, Rack: req.Rack, Bin: req.Bin,
		Type: req.Type, Capacity: req.Capacity, Status: req.Status,
	}
	if err := s.repo.Update(ctx, tenantID, l); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete blocks deletion while on-hand inventory still exists at this
// location (Location Master Management's own addition — see
// ErrHasStock's doc).
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}

	if s.inventory != nil {
		stockCount, err := s.inventory.CountByLocation(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if stockCount > 0 {
			return ErrHasStock
		}
	}

	return s.repo.Delete(ctx, tenantID, id)
}

// validateZoneOwnership confirms zoneID exists AND belongs to tenantID
// — an ownership check, not a bare existence check (tenant-isolation-
// security.md §6's "existence checker vs ownership checker"
// distinction), so Tenant A can never create a location under Tenant
// B's zone by guessing its ID. Returns the same ErrZoneNotFound either
// way a bare existence check would for a truly nonexistent zone, so
// the response never reveals that a given ID belongs to another
// tenant.
func (s *Service) validateZoneOwnership(ctx context.Context, tenantID, zoneID int64) error {
	z, err := s.zones.FindByID(ctx, zoneID)
	if err != nil {
		return ErrZoneNotFound
	}
	if z.TenantID != tenantID {
		return ErrZoneNotFound
	}
	return nil
}
