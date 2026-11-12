package user

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/role"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response —
// distinct from a plain Go error so the handler can tell "bad input" apart
// from "unexpected failure" without string matching.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrRoleNotFound is returned when a request's role_code doesn't match
// any seeded role.
var ErrRoleNotFound = errors.New("user: role not found")

// ErrWarehouseNotFound is returned when a request's warehouse_id doesn't
// match any warehouse row.
var ErrWarehouseNotFound = errors.New("user: warehouse not found")

const minPasswordLength = 8

// roleLookup is the narrow slice of role.Repository the user service
// needs: resolving a role_code to its row (to validate + get the ID) and
// a role_id back to its summary (to embed in responses).
type roleLookup interface {
	FindRoleByCode(ctx context.Context, code string) (*role.Role, error)
	FindRoleByID(ctx context.Context, id int64) (*role.Role, error)
}

// warehouseLookup is the narrow slice of warehouse.Repository the user
// service needs: resolving a warehouse_id to its row, both to validate a
// Create/Update request and to resolve the code/name pair embedded in
// responses (see toResponse). Satisfied directly by *warehouse.Repository
// — no adapter needed, since its FindByID already has this exact shape.
type warehouseLookup interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// Service holds the User Management business rules: password hashing,
// role_code<->role_id resolution, warehouse_id validation, and
// uniqueness/validation error mapping. Handlers never call Repository
// directly.
type Service struct {
	repo       *Repository
	roles      roleLookup
	warehouses warehouseLookup
}

func NewService(repo *Repository, roles roleLookup, warehouses warehouseLookup) *Service {
	return &Service{repo: repo, roles: roles, warehouses: warehouses}
}

// resolveWarehouseID validates an optional warehouse_id against the
// warehouse master, returning ErrWarehouseNotFound if it doesn't match
// any row. A nil id (no warehouse selected) is always valid and passes
// through unchanged — see migrations/0039 for why NULL is a normal,
// non-error state here.
func (s *Service) resolveWarehouseID(ctx context.Context, id *int64) (*int64, error) {
	if id == nil {
		return nil, nil
	}
	if _, err := s.warehouses.FindByID(ctx, *id); err != nil {
		if errors.Is(err, warehouse.ErrNotFound) {
			return nil, ErrWarehouseNotFound
		}
		return nil, err
	}
	return id, nil
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	filter := ListFilter{Search: params.Search, Status: params.Status, Page: page, Limit: limit}

	if params.RoleCode != "" {
		r, err := s.roles.FindRoleByCode(ctx, params.RoleCode)
		if errors.Is(err, role.ErrNotFound) {
			// An unrecognized role filter matches nothing, rather than
			// erroring — the same way an empty search result would.
			return []Response{}, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		filter.RoleID = r.ID
	}

	users, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(users))
	for i := range users {
		resp, err := s.toResponse(ctx, &users[i])
		if err != nil {
			return nil, 0, err
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return s.toResponse(ctx, u)
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Response, error) {
	if len(req.Password) < minPasswordLength {
		return Response{}, ErrValidation{Message: fmt.Sprintf("password must be at least %d characters", minPasswordLength)}
	}

	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE, INACTIVE, or LOCKED"}
	}

	r, err := s.roles.FindRoleByCode(ctx, req.RoleCode)
	if errors.Is(err, role.ErrNotFound) {
		return Response{}, ErrRoleNotFound
	}
	if err != nil {
		return Response{}, err
	}

	warehouseID, err := s.resolveWarehouseID(ctx, req.WarehouseID)
	if err != nil {
		return Response{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return Response{}, err
	}

	u := &User{
		Username:     req.Username,
		PasswordHash: string(hash),
		FullName:     req.FullName,
		Email:        req.Email,
		RoleID:       r.ID,
		WarehouseID:  warehouseID,
		Status:       status,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return Response{}, err
	}

	return s.toResponse(ctx, u)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (Response, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}

	if !req.Status.IsValid() {
		return Response{}, ErrValidation{Message: "status must be ACTIVE, INACTIVE, or LOCKED"}
	}

	r, err := s.roles.FindRoleByCode(ctx, req.RoleCode)
	if errors.Is(err, role.ErrNotFound) {
		return Response{}, ErrRoleNotFound
	}
	if err != nil {
		return Response{}, err
	}

	warehouseID, err := s.resolveWarehouseID(ctx, req.WarehouseID)
	if err != nil {
		return Response{}, err
	}

	passwordHash := existing.PasswordHash
	if req.Password != "" {
		if len(req.Password) < minPasswordLength {
			return Response{}, ErrValidation{Message: fmt.Sprintf("password must be at least %d characters", minPasswordLength)}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return Response{}, err
		}
		passwordHash = string(hash)
	}

	existing.FullName = req.FullName
	existing.Email = req.Email
	existing.RoleID = r.ID
	existing.WarehouseID = warehouseID
	existing.Status = req.Status
	existing.PasswordHash = passwordHash

	if err := s.repo.Update(ctx, existing); err != nil {
		return Response{}, err
	}

	return s.toResponse(ctx, existing)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) toResponse(ctx context.Context, u *User) (Response, error) {
	r, err := s.roles.FindRoleByID(ctx, u.RoleID)
	if err != nil {
		return Response{}, err
	}

	resp := Response{
		ID:          u.ID,
		Username:    u.Username,
		FullName:    u.FullName,
		Email:       u.Email,
		Role:        RoleSummary{Code: r.Code, Name: r.Name},
		WarehouseID: u.WarehouseID,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}

	// A user's warehouse_id is a validated FK (see resolveWarehouseID) —
	// when set, the referenced warehouse always exists, so any lookup
	// error here is a genuine failure, not a "not found" to swallow.
	if u.WarehouseID != nil {
		w, err := s.warehouses.FindByID(ctx, *u.WarehouseID)
		if err != nil {
			return Response{}, err
		}
		resp.WarehouseCode = &w.Code
		resp.WarehouseName = &w.Name
	}

	return resp, nil
}
