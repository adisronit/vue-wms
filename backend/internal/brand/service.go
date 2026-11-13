package brand

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
// one of the values the Brand selector's sortable columns can actually
// send — see internal/productcategory/service.go's own ErrInvalidSort
// doc for why this is a 400, not a silent fallback.
var ErrInvalidSort = errors.New("brand: invalid sort_by or sort_dir")

const (
	defaultSortBy  = "code"
	defaultSortDir = "ASC"
	defaultLimit   = 25
)

var allowedSortColumns = map[string]bool{"code": true, "name": true, "status": true}

// Service holds Brand master data's business rules: validation and
// uniqueness-error mapping. Handlers never call Repository directly.
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

	brands, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, Status: params.Status, Page: page, Limit: limit,
		SortBy: sortBy, SortDir: sortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(brands))
	for _, b := range brands {
		responses = append(responses, toResponse(b))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	b, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*b), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	b := &Brand{Code: req.Code, Name: req.Name, Status: status}

	if err := s.repo.Create(ctx, tenantID, b); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, b.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	b := &Brand{ID: id, Code: req.Code, Name: req.Name, Status: req.Status}

	if err := s.repo.Update(ctx, tenantID, b); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the brand. Nothing in this codebase currently
// references brands by foreign key except products.brand_id itself —
// unlike product_category/warehouse/zone, no referential-safety guard
// is added here yet; a brand still referenced by a product would leave
// that product's brand_id dangling... but FK_products_brand has no
// ON DELETE CASCADE/SET NULL, so SQL Server itself rejects the delete
// with a FK violation rather than silently orphaning the reference —
// deactivation (status=INACTIVE) remains the safe, non-destructive way
// to retire a brand still in use.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, tenantID, id)
}
