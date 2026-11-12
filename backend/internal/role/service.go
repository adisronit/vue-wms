package role

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/warehouse"
)

// ErrInvalidPermissionCodes is returned when SetPermissions is asked to
// grant a code that doesn't exist in the permission catalog.
var ErrInvalidPermissionCodes = errors.New("role: one or more permission codes are invalid")

// ErrInvalidWarehouseIDs is returned when SetRoleWarehouseAccess is asked
// to grant a warehouse_id that doesn't exist in the warehouse master.
var ErrInvalidWarehouseIDs = errors.New("role: one or more warehouse IDs are invalid")

// warehouseLookup is the narrow slice of warehouse.Repository the role
// service needs: bulk-resolving warehouse_ids to their code/name (to
// embed in responses) and to validate a Set request. Satisfied directly
// by *warehouse.Repository's FindByIDs — no adapter needed.
type warehouseLookup interface {
	FindByIDs(ctx context.Context, ids []int64) ([]warehouse.Warehouse, error)
}

// Service adds validation on top of Repository; handlers never touch the
// repository directly.
type Service struct {
	repo       *Repository
	warehouses warehouseLookup
}

func NewService(repo *Repository, warehouses warehouseLookup) *Service {
	return &Service{repo: repo, warehouses: warehouses}
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, id int64) (*Role, error) {
	return s.repo.FindRoleByID(ctx, id)
}

func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	return s.repo.ListPermissions(ctx)
}

// GetRolePermissions returns the role plus the permission codes currently
// granted to it.
func (s *Service) GetRolePermissions(ctx context.Context, roleID int64) (*Role, []string, error) {
	r, err := s.repo.FindRoleByID(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}

	codes, err := s.repo.RolePermissionCodes(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}

	return r, codes, nil
}

// SetRolePermissions replaces roleID's granted permissions with exactly
// codes, after confirming every code exists in the catalog. Returns
// ErrNotFound if the role doesn't exist, ErrInvalidPermissionCodes if any
// code is unrecognized.
func (s *Service) SetRolePermissions(ctx context.Context, roleID int64, codes []string) ([]string, error) {
	if _, err := s.repo.FindRoleByID(ctx, roleID); err != nil {
		return nil, err
	}

	// ReplaceRolePermissions is a delete-then-insert transaction (see
	// its doc) — the same class of multi-statement mutation Phase 8
	// hardened in Receiving/Putaway/Inventory. Detach before it opens;
	// the reads below (PermissionsByCodes, RolePermissionCodes) piggy-
	// back on the same bounded context rather than switching back,
	// matching the simpler shape used elsewhere in this phase.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	if len(codes) == 0 {
		start := time.Now()
		err := s.repo.ReplaceRolePermissions(ctx, roleID, nil)
		dbctx.LogTransaction(ctx, "role.set_permissions", start, err)
		if err != nil {
			return nil, err
		}
		return []string{}, nil
	}

	permissions, err := s.repo.PermissionsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	if len(permissions) != len(uniqueStrings(codes)) {
		return nil, fmt.Errorf("%w", ErrInvalidPermissionCodes)
	}

	ids := make([]int64, 0, len(permissions))
	for _, p := range permissions {
		ids = append(ids, p.ID)
	}

	start := time.Now()
	err = s.repo.ReplaceRolePermissions(ctx, roleID, ids)
	dbctx.LogTransaction(ctx, "role.set_permissions", start, err)
	if err != nil {
		return nil, err
	}

	return s.repo.RolePermissionCodes(ctx, roleID)
}

// GetRoleWarehouseAccess returns the role plus the warehouses currently
// granted to it (resolved to code/name for display).
func (s *Service) GetRoleWarehouseAccess(ctx context.Context, roleID int64) (*Role, []WarehouseSummary, error) {
	r, err := s.repo.FindRoleByID(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}

	summaries, err := s.warehouseSummaries(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}

	return r, summaries, nil
}

// SetRoleWarehouseAccess replaces roleID's warehouse access with exactly
// warehouseIDs (deduplicated), after confirming every ID exists in the
// warehouse master. Returns ErrNotFound if the role doesn't exist,
// ErrInvalidWarehouseIDs if any ID is unrecognized. An empty/nil
// warehouseIDs is valid input — it explicitly means "no warehouse
// access" (see ReplaceRoleWarehouseAccess), never "all warehouses"; see
// package role's own doc on Admin/global access for how that case is
// represented instead. createdBy is the acting user (from the JWT),
// attributed to every grant row this call creates.
func (s *Service) SetRoleWarehouseAccess(ctx context.Context, roleID int64, warehouseIDs []int64, createdBy int64) ([]WarehouseSummary, error) {
	if _, err := s.repo.FindRoleByID(ctx, roleID); err != nil {
		return nil, err
	}

	// ReplaceRoleWarehouseAccess is a delete-then-insert transaction (see
	// its doc) — the same class of multi-statement mutation
	// ReplaceRolePermissions above already detaches for.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	ids := uniqueInt64s(warehouseIDs)

	if len(ids) > 0 {
		found, err := s.warehouses.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		if len(found) != len(ids) {
			return nil, fmt.Errorf("%w", ErrInvalidWarehouseIDs)
		}
	}

	start := time.Now()
	err := s.repo.ReplaceRoleWarehouseAccess(ctx, roleID, ids, &createdBy)
	dbctx.LogTransaction(ctx, "role.set_warehouse_access", start, err)
	if err != nil {
		return nil, err
	}

	return s.warehouseSummaries(ctx, roleID)
}

// warehouseSummaries resolves roleID's currently granted warehouse_ids
// (role_warehouse_access) to their code/name for display, shared by
// GetRoleWarehouseAccess and SetRoleWarehouseAccess so both return
// identically-shaped, identically-ordered data.
func (s *Service) warehouseSummaries(ctx context.Context, roleID int64) ([]WarehouseSummary, error) {
	ids, err := s.repo.RoleWarehouseAccessIDs(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []WarehouseSummary{}, nil
	}

	warehouses, err := s.warehouses.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	summaries := make([]WarehouseSummary, 0, len(warehouses))
	for _, w := range warehouses {
		summaries = append(summaries, WarehouseSummary{ID: w.ID, Code: w.Code, Name: w.Name})
	}
	return summaries, nil
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
