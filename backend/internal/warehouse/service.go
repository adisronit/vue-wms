package warehouse

import (
	"context"
	"errors"
	"strings"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrHasZones is returned when deleting a warehouse that still has
// zones — referential safety per CLAUDE.md's delete rules, surfaced as
// 409 Conflict rather than a raw FK violation.
var ErrHasZones = errors.New("warehouse: cannot delete — zones still exist")

// ErrInvalidSort is returned when sort_by/sort_dir is present but not
// one of the values the Warehouse Setup screen's sortable column
// headers can actually send — a 400, not a silent fallback to the
// default sort, so a frontend bug (or a hand-crafted request) surfaces
// immediately instead of quietly returning results in the wrong order.
var ErrInvalidSort = errors.New("warehouse: invalid sort_by or sort_dir")

// defaultSortBy/defaultSortDir match the Warehouse Setup screen's
// default view: code is the field users scan to find a row, so that is
// what an omitted sort_by defaults to.
const (
	defaultSortBy  = "code"
	defaultSortDir = "ASC"
)

// allowedSortColumns is the sort-column allowlist GET
// /api/v1/warehouses' sort_by validates against — the same set
// repository.go's sortColumns maps to real column names. Kept here, not
// just in Repository, so an invalid value is rejected with ErrInvalidSort
// before Service.List does anything else, rather than silently falling
// back deep in the Repository call. zone_count/location_count/
// utilization are deliberately excluded — see repository.go's
// sortColumns doc.
var allowedSortColumns = map[string]bool{
	"code":   true,
	"name":   true,
	"type":   true,
	"status": true,
}

// defaultLimit is the page size applied when the caller omits limit (or
// sends one outside [1,100]) — matches the Warehouse Setup screen's own
// default of 25 rows per page.
const defaultLimit = 25

// zoneCounter is the narrow slice of *zone.Repository this package
// needs — just enough to refuse deleting a warehouse that still has
// zones, without importing package zone (which itself imports package
// warehouse to validate warehouse_id, so a direct import back would
// cycle). main.go wires the concrete *zone.Repository in here; it
// structurally satisfies this interface.
type zoneCounter interface {
	CountByWarehouse(ctx context.Context, warehouseID int64) (int64, error)
}

// Service holds Warehouse Setup's business rules: validation and
// referential-safety checks. Handlers never call Repository directly.
type Service struct {
	repo  *Repository
	zones zoneCounter
}

func NewService(repo *Repository, zones zoneCounter) *Service {
	return &Service{repo: repo, zones: zones}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = defaultLimit
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

	rows, total, err := s.repo.List(ctx, ListFilter{
		Search: params.Search, Type: params.Type, Status: params.Status,
		Page: page, Limit: limit, SortBy: sortBy, SortDir: sortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(rows))
	for _, w := range rows {
		responses = append(responses, toResponse(w))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	w, err := s.repo.FindWithCounts(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*w), nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	w := &Warehouse{
		Code: req.Code, Name: req.Name, Type: req.Type, Address: req.Address,
		ContactPerson: req.ContactPerson, Phone: req.Phone, Capacity: req.Capacity,
		Status: status,
	}

	if err := s.repo.Create(ctx, w); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, w.ID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	w := &Warehouse{
		ID: id, Code: req.Code, Name: req.Name, Type: req.Type, Address: req.Address,
		ContactPerson: req.ContactPerson, Phone: req.Phone, Capacity: req.Capacity,
		Status: req.Status,
	}

	if err := s.repo.Update(ctx, w); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}

	count, err := s.zones.CountByWarehouse(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrHasZones
	}

	return s.repo.Delete(ctx, id)
}
