package supplier

import (
	"context"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// Service holds Supplier master data's business rules: validation and
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
		limit = 20
	}

	suppliers, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, Status: params.Status, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(suppliers))
	for _, sup := range suppliers {
		responses = append(responses, toResponse(sup))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	sup, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*sup), nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	sup := &Supplier{
		Code: req.Code, CompanyName: req.CompanyName, TaxID: req.TaxID,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		Address: req.Address, Email: req.Email, Phone: req.Phone, Status: status,
	}

	if err := s.repo.Create(ctx, tenantID, sup); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, sup.ID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	sup := &Supplier{
		ID: id, Code: req.Code, CompanyName: req.CompanyName, TaxID: req.TaxID,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		Address: req.Address, Email: req.Email, Phone: req.Phone, Status: req.Status,
	}

	if err := s.repo.Update(ctx, tenantID, sup); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the supplier. Nothing in this codebase currently
// references suppliers by foreign key — Receiving's Vendor is a plain
// free-text column, not a supplier_id FK (the task's own instruction:
// "reuse customer/supplier relationships in existing workflows only
// where supported by current API/data contracts", and today's contract
// doesn't) — so there is no child row to protect against, the same
// starting point internal/product.Service.Delete itself documents for
// the same reason. Deactivation (status=INACTIVE) is the safe,
// non-destructive way to retire a supplier; this hard-delete endpoint
// exists for completeness/admin use. If a future phase adds a real
// supplier_id FK elsewhere, this must gain the same referential-safety
// check internal/warehouse/zone/productcategory's own Delete already
// have.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, tenantID, id)
}
