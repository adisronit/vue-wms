package location

import (
	"context"
	"errors"

	"wms-backend/internal/zone"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrZoneNotFound is returned when a request's zone_id doesn't match any
// zone.
var ErrZoneNotFound = errors.New("location: zone not found")

// zoneChecker is the narrow slice of *zone.Repository this package
// needs: confirming zone_id exists before creating a location under it.
// Importing package zone here is safe — the dependency chain is
// warehouse <- zone <- location, one direction only.
type zoneChecker interface {
	FindByID(ctx context.Context, id int64) (*zone.Zone, error)
}

// Service holds Location Structure's business rules: validation and
// zone-existence checks.
type Service struct {
	repo  *Repository
	zones zoneChecker
}

func NewService(repo *Repository, zones zoneChecker) *Service {
	return &Service{repo: repo, zones: zones}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 500
	}

	locations, total, err := s.repo.List(ctx, ListFilter{
		ZoneID: params.ZoneID, Status: params.Status, Page: page, Limit: limit,
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

func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	l, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*l), nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	if _, err := s.zones.FindByID(ctx, req.ZoneID); err != nil {
		return Response{}, ErrZoneNotFound
	}

	l := &Location{ZoneID: req.ZoneID, Code: req.Code, Aisle: req.Aisle, Rack: req.Rack, Bin: req.Bin, Status: status}
	if err := s.repo.Create(ctx, l); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, l.ID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	l := &Location{ID: id, Code: req.Code, Aisle: req.Aisle, Rack: req.Rack, Bin: req.Bin, Status: req.Status}
	if err := s.repo.Update(ctx, l); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
