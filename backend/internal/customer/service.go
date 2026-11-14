package customer

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
// one of the values the Customers screen's sortable column headers can
// actually send — a 400, not a silent fallback to the default sort, so
// a frontend bug (or a hand-crafted request) surfaces immediately
// instead of quietly returning results in the wrong order. See
// internal/productcategory/service.go's doc for the original of this
// exact pattern.
var ErrInvalidSort = errors.New("customer: invalid sort_by or sort_dir")

// ErrHasDeliveryLocations is returned when deleting a customer that
// still has delivery locations (any status) — mirrors
// internal/warehouse.ErrHasZones exactly: customer_delivery_locations
// (migrations/0059) now holds a real customer_id FK, so a plain hard
// delete would otherwise fail on that FK with a raw SQL error instead
// of this clean, client-facing one.
var ErrHasDeliveryLocations = errors.New("customer: cannot delete — delivery locations still exist")

// defaultSortBy/defaultSortDir match the Customers screen's default
// view: code is the field users actually scan to find a row.
const (
	defaultSortBy  = "code"
	defaultSortDir = "ASC"
)

// allowedSortColumns is the sort-column allowlist GET
// /api/v1/customers' sort_by validates against — the same set
// repository.go's sortColumns maps to real column names. Kept here, not
// just in Repository, so an invalid value is rejected with
// ErrInvalidSort before Service.List does anything else, rather than
// silently falling back deep in the Repository call.
var allowedSortColumns = map[string]bool{
	"code":        true,
	"companyName": true,
	"status":      true,
}

// defaultLimit is the page size applied when the caller omits limit
// (or sends one outside [1,100]) — the Customers screen's own default
// is 25 rows per page.
const defaultLimit = 25

// deliveryLocationCounter is the narrow slice of
// *customerdeliverylocation.Repository this package needs for Delete's
// referential-safety check — the same narrow-interface pattern
// internal/warehouse established for its own zoneCounter dependency on
// internal/zone.
type deliveryLocationCounter interface {
	CountByCustomer(ctx context.Context, tenantID, customerID int64) (int64, error)
}

// Service holds Customer master data's business rules: validation and
// uniqueness-error mapping. Handlers never call Repository directly.
type Service struct {
	repo              *Repository
	deliveryLocations deliveryLocationCounter
}

func NewService(repo *Repository, deliveryLocations deliveryLocationCounter) *Service {
	return &Service{repo: repo, deliveryLocations: deliveryLocations}
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
	if raw := strings.TrimSpace(params.SortBy); raw != "" {
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

	customers, total, err := s.repo.List(ctx, tenantID, ListFilter{
		Search: params.Search, Status: params.Status, Page: page, Limit: limit,
		SortBy: sortBy, SortDir: sortDir,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(customers))
	for _, c := range customers {
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

	c := &Customer{
		Code: req.Code, CompanyName: req.CompanyName, LegalNameTH: req.LegalNameTH,
		CustomerType: req.CustomerType, CustomerGroup: req.CustomerGroup, TaxID: req.TaxID, BranchNo: req.BranchNo,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		Address: req.Address, Email: req.Email, Phone: req.Phone, Status: status,
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

	c := &Customer{
		ID: id, Code: req.Code, CompanyName: req.CompanyName, LegalNameTH: req.LegalNameTH,
		CustomerType: req.CustomerType, CustomerGroup: req.CustomerGroup, TaxID: req.TaxID, BranchNo: req.BranchNo,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		Address: req.Address, Email: req.Email, Phone: req.Phone, Status: req.Status,
	}

	if err := s.repo.Update(ctx, tenantID, c); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the customer. Order.Customer is still a plain
// free-text column (no customer_id FK — see internal/order/model.go's
// doc), but customer_delivery_locations (migrations/0059) now does
// hold a real FK, so this gained the same referential-safety check
// internal/warehouse.Service.Delete already has for zones — a clean
// ErrHasDeliveryLocations instead of letting the FK violation surface
// as a raw SQL error. Deactivation (status=INACTIVE) remains the safe,
// non-destructive way to retire a customer that still has locations;
// this hard-delete endpoint exists for completeness/admin use once none
// remain.
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	if _, err := s.repo.FindByID(ctx, tenantID, id); err != nil {
		return err
	}

	count, err := s.deliveryLocations.CountByCustomer(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrHasDeliveryLocations
	}

	return s.repo.Delete(ctx, tenantID, id)
}
