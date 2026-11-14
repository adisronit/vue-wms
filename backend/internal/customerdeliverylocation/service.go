package customerdeliverylocation

import (
	"context"
	"errors"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrCustomerNotFound is returned when the request's customer_id does
// not exist within the caller's own tenant — covers both "doesn't
// exist at all" and "belongs to a different tenant" with the same 404,
// never distinguishing the two (tenant-isolation-security.md §6).
var ErrCustomerNotFound = errors.New("customerdeliverylocation: customer not found")

// customerChecker is the narrow slice of *customer.Repository this
// package needs — the same narrow-interface pattern internal/product
// uses for its own default_supplier_id ownership check against
// *supplier.Repository.Exists. No import of internal/customer is
// needed: Exists' signature uses only primitive types, so this
// interface is satisfied structurally.
type customerChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

// Service holds Customer Delivery Locations' business rules:
// validation, cross-tenant ownership, and the "exactly one default
// ship-to per customer when locations exist" invariant. Handlers never
// call Repository directly.
type Service struct {
	repo      *Repository
	customers customerChecker
}

func NewService(repo *Repository, customers customerChecker) *Service {
	return &Service{repo: repo, customers: customers}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, error) {
	if ok, err := s.customers.Exists(ctx, tenantID, params.CustomerID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrCustomerNotFound
	}

	locations, _, err := s.repo.List(ctx, tenantID, ListFilter{
		CustomerID: params.CustomerID, Search: params.Search, Status: params.Status,
	})
	if err != nil {
		return nil, err
	}

	responses := make([]Response, 0, len(locations))
	for _, l := range locations {
		responses = append(responses, toResponse(l))
	}
	return responses, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (Response, error) {
	l, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*l), nil
}

// Create validates ownership and the new row's fields, then — inside
// one transaction — forces is_default=true when this is the
// customer's very first location (guaranteeing "a customer with
// locations always has exactly one default" from the first row
// onward) and, whenever the request explicitly asks for is_default
// regardless, clears any prior default for that customer before
// inserting. The filtered unique index (migrations/0059) backs this up
// at the DB level; this is what keeps it from ever being asked to hold
// two defaults for the same customer even momentarily.
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest) (Response, error) {
	if ok, err := s.customers.Exists(ctx, tenantID, req.CustomerID); err != nil {
		return Response{}, err
	} else if !ok {
		return Response{}, ErrCustomerNotFound
	}

	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be BRANCH, DISTRIBUTION_CENTER, or DELIVERY_ADDRESS"}
	}
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	loc := &Location{
		CustomerID: req.CustomerID, Code: req.Code, Name: req.Name, Type: req.Type,
		Address: req.Address, Province: req.Province, District: req.District, PostalCode: req.PostalCode,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		ReceivingWindow: req.ReceivingWindow, Instructions: req.Instructions,
		IsDefault: req.IsDefault, Status: status,
	}

	if err := s.repo.Transaction(ctx, func(txRepo *Repository) error {
		existingCount, err := txRepo.CountByCustomer(ctx, tenantID, req.CustomerID)
		if err != nil {
			return err
		}
		if existingCount == 0 {
			loc.IsDefault = true
		} else if loc.IsDefault {
			if err := txRepo.ClearDefault(ctx, tenantID, req.CustomerID, 0); err != nil {
				return err
			}
		}
		return txRepo.Create(ctx, tenantID, loc)
	}); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, loc.ID)
}

// Update validates the edited fields and — when the request would
// leave the customer's current default without a replacement while
// other ACTIVE locations still exist (either by un-checking is_default
// on the default row, or by deactivating it) — rejects with
// ErrValidation rather than silently guessing which other location
// should become the new default. Otherwise, promoting a different row
// to default clears the prior one inside the same transaction, exactly
// like Create.
func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (Response, error) {
	existing, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return Response{}, err
	}

	if !req.Type.IsValid() {
		return Response{}, ErrValidation{Message: "type must be BRANCH, DISTRIBUTION_CENTER, or DELIVERY_ADDRESS"}
	}
	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}

	losingDefault := existing.IsDefault && (!req.IsDefault || (existing.Status == StatusActive && req.Status == StatusInactive))
	if losingDefault {
		activeOthers, err := s.repo.CountActiveExcluding(ctx, tenantID, existing.CustomerID, id)
		if err != nil {
			return Response{}, err
		}
		if activeOthers > 0 {
			return Response{}, ErrValidation{Message: "assign another location as this customer's default before removing default from, or deactivating, this one"}
		}
	}

	loc := &Location{
		ID: id, CustomerID: existing.CustomerID, Code: req.Code, Name: req.Name, Type: req.Type,
		Address: req.Address, Province: req.Province, District: req.District, PostalCode: req.PostalCode,
		ContactPerson: req.ContactPerson, ContactPhone: req.ContactPhone,
		ReceivingWindow: req.ReceivingWindow, Instructions: req.Instructions,
		IsDefault: req.IsDefault, Status: req.Status,
	}

	promotingToDefault := req.IsDefault && !existing.IsDefault
	if err := s.repo.Transaction(ctx, func(txRepo *Repository) error {
		if promotingToDefault {
			if err := txRepo.ClearDefault(ctx, tenantID, existing.CustomerID, id); err != nil {
				return err
			}
		}
		return txRepo.Update(ctx, tenantID, loc)
	}); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, tenantID, id)
}

// Delete hard-deletes the location — refused with the same
// ErrValidation as Update whenever it is the customer's current
// default and other ACTIVE locations still exist. There is nothing
// else to check referentially this phase: no other table holds a
// foreign key to customer_delivery_locations yet (Order/Shipping
// integration was audited and deferred — see PR description).
func (s *Service) Delete(ctx context.Context, tenantID, id int64) error {
	existing, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if existing.IsDefault {
		activeOthers, err := s.repo.CountActiveExcluding(ctx, tenantID, existing.CustomerID, id)
		if err != nil {
			return err
		}
		if activeOthers > 0 {
			return ErrValidation{Message: "assign another location as this customer's default before deleting this one"}
		}
	}

	return s.repo.Delete(ctx, tenantID, id)
}
