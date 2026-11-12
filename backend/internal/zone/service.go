package zone

import (
	"context"
	"errors"

	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrWarehouseNotFound is returned when a request's warehouse_id doesn't
// match any warehouse.
var ErrWarehouseNotFound = errors.New("zone: warehouse not found")

// ErrHasLocations is returned when deleting a zone that still has
// locations — 409, same referential-safety rule as warehouse deletion.
var ErrHasLocations = errors.New("zone: cannot delete — locations still exist")

// warehouseChecker is the narrow slice of *warehouse.Repository this
// package needs: confirming warehouse_id exists before creating a zone
// under it. Importing package warehouse here is safe — the dependency
// only goes one way (zone -> warehouse); warehouse avoids importing
// zone back by declaring its own narrow zoneCounter interface instead.
type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// locationCounter is the narrow slice of *location.Repository this
// package needs, for the same reason warehouse needs zoneCounter.
type locationCounter interface {
	CountByZone(ctx context.Context, zoneID int64) (int64, error)
}

// Service holds Zone Setup's business rules: validation, warehouse
// existence, and referential-safety checks.
type Service struct {
	repo       *Repository
	warehouses warehouseChecker
	locations  locationCounter
}

func NewService(repo *Repository, warehouses warehouseChecker, locations locationCounter) *Service {
	return &Service{repo: repo, warehouses: warehouses, locations: locations}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 200
	}

	zones, total, err := s.repo.List(ctx, ListFilter{
		WarehouseID: params.WarehouseID, Status: params.Status, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(zones))
	for _, z := range zones {
		responses = append(responses, toResponse(z))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	z, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*z), nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Response, error) {
	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be one of RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD"}
	}

	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return Response{}, ErrWarehouseNotFound
	}

	z := &Zone{WarehouseID: req.WarehouseID, Code: req.Code, Name: req.Name, Type: req.Type, Status: status}
	if err := s.repo.Create(ctx, z); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, z.ID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return Response{}, err
	}

	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be one of RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD"}
	}
	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	z := &Zone{ID: id, Code: req.Code, Name: req.Name, Type: req.Type, Status: req.Status}
	if err := s.repo.Update(ctx, z); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}

	count, err := s.locations.CountByZone(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrHasLocations
	}

	return s.repo.Delete(ctx, id)
}
