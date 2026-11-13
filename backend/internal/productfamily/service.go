package productfamily

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

// ErrInvalidSort is returned when sort_by/sort_dir is present but not
// one of the values the Family selector's sortable columns can
// actually send.
var ErrInvalidSort = errors.New("productfamily: invalid sort_by or sort_dir")

const (
	defaultSortBy  = "code"
	defaultSortDir = "ASC"
	defaultLimit   = 25
)

var allowedSortColumns = map[string]bool{"code": true, "name": true, "status": true}

// Service holds Product Family master data's business rules:
// validation and uniqueness-error mapping. Handlers never call
// Repository directly.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
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

	families, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, Status: params.Status, Page: page, Limit: limit,
		SortBy: sortBy, SortDir: sortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(families))
	for _, f := range families {
		responses = append(responses, toResponse(f))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	f, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*f), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	f := &Family{Code: req.Code, Name: req.Name, Status: status}

	if err := s.repo.Create(ctx, tenantID, f); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, f.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	f := &Family{ID: id, Code: req.Code, Name: req.Name, Status: req.Status}

	if err := s.repo.Update(ctx, tenantID, f); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the family. See internal/brand.Service.Delete's
// own doc — the same shape applies: FK_products_family has no CASCADE/
// SET NULL, so SQL Server rejects deleting a family still referenced
// by a product rather than orphaning the reference.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, tenantID, id)
}
