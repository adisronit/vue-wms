package productcategory

import (
	"context"
	"errors"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrHasChildren is returned when deleting a category that still has
// child categories — the same "block delete while children exist"
// shape internal/warehouse.ErrHasZones and internal/zone.ErrHasLocations
// already use.
var ErrHasChildren = errors.New("productcategory: cannot delete — child categories still exist")

// ErrHasProducts is returned when deleting a category that is still
// referenced by at least one product's category_id.
var ErrHasProducts = errors.New("productcategory: cannot delete — products still reference this category")

// ErrInvalidParent covers both "parent category not found" and
// "parent would create a circular hierarchy" — Service.validateParent
// returns one or the other depending on which check fails.
type ErrInvalidParent struct {
	Message string
}

func (e ErrInvalidParent) Error() string { return e.Message }

// maxHierarchyDepth bounds the ancestor walk in detectCycle — a
// defensive limit (no real category tree should ever nest this deep),
// not a real product requirement, purely so a data-integrity bug
// elsewhere can never turn this walk into an infinite loop.
const maxHierarchyDepth = 50

// productCounter is the narrow capability Service.Delete needs from
// internal/product — satisfied by *product.Repository's
// CountByCategory — kept as a local interface so this package never
// imports internal/product directly, the same depend-on-behavior
// pattern internal/warehouse's Service already uses for its own zones
// dependency.
type productCounter interface {
	CountByCategory(ctx context.Context, tenantID, categoryID int64) (int64, error)
}

// Service holds Product Category's business rules: validation,
// circular-hierarchy prevention, and referential-safety checks before
// delete. Handlers never call Repository directly.
type Service struct {
	repo     *Repository
	products productCounter
}

func NewService(repo *Repository, products productCounter) *Service {
	return &Service{repo: repo, products: products}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	categories, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, ParentID: params.ParentID, Status: params.Status,
		Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(categories))
	for _, c := range categories {
		responses = append(responses, toResponse(c))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	c, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*c), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	if req.ParentID != nil {
		if _, err := s.repo.FindByID(ctx, tenantID, *req.ParentID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return Response{}, ErrInvalidParent{Message: "parent category not found"}
			}
			return Response{}, err
		}
	}

	c := &Category{
		Code: req.Code, Name: req.Name, ParentID: req.ParentID,
		Description: req.Description, Status: status,
	}

	if err := s.repo.Create(ctx, tenantID, c); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, c.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	if req.ParentID != nil {
		if err := s.validateParent(ctx, tenantID, id, *req.ParentID); err != nil {
			return Response{}, err
		}
	}

	c := &Category{
		ID: id, Code: req.Code, Name: req.Name, ParentID: req.ParentID,
		Description: req.Description, Status: req.Status,
	}

	if err := s.repo.Update(ctx, tenantID, c); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete blocks deletion while either child categories or products
// still reference id, matching internal/warehouse/zone's own
// referential-safety shape before Delete.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}

	childCount, err := s.repo.CountByParent(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if childCount > 0 {
		return ErrHasChildren
	}

	productCount, err := s.products.CountByCategory(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if productCount > 0 {
		return ErrHasProducts
	}

	return s.repo.Delete(ctx, tenantID, id)
}

// validateParent enforces the two rules a category's new parent must
// satisfy on update: it must exist within the same tenant (an
// ownership check, not a bare existence check — tenant-isolation-
// security.md §6's "existence checker vs ownership checker" distinction
// — so Tenant A can never nest one of its categories under Tenant B's),
// and it must not be id itself or any descendant of id, which would
// otherwise create a cycle a later List/Get tree-walk could loop on
// forever.
func (s *Service) validateParent(ctx context.Context, tenantID, id, newParentID int64) error {
	if newParentID == id {
		return ErrInvalidParent{Message: "a category cannot be its own parent"}
	}

	cycle, err := s.detectCycle(ctx, tenantID, id, newParentID)
	if err != nil {
		return err
	}
	if cycle {
		return ErrInvalidParent{Message: "this parent would create a circular category hierarchy"}
	}
	return nil
}

// detectCycle walks from newParentID up through successive ParentID
// links, within tenantID only, until it either reaches a root (ParentID
// == nil — no cycle) or encounters id again (a cycle). Bounded by
// maxHierarchyDepth so a pre-existing data-integrity bug elsewhere can
// never turn this into an infinite loop.
func (s *Service) detectCycle(ctx context.Context, tenantID, id, newParentID int64) (bool, error) {
	current := newParentID
	for i := 0; i < maxHierarchyDepth; i++ {
		if current == id {
			return true, nil
		}
		cat, err := s.repo.FindByID(ctx, tenantID, current)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return false, ErrInvalidParent{Message: "parent category not found"}
			}
			return false, err
		}
		if cat.ParentID == nil {
			return false, nil
		}
		current = *cat.ParentID
	}
	return false, ErrValidation{Message: "category hierarchy too deep"}
}
