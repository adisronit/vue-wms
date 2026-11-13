package role

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a role lookup by ID/code finds no row.
var ErrNotFound = errors.New("role: not found")

// Repository is a thin GORM wrapper over roles/permissions/role_permissions.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListRoles returns tenantID's own roles — the Roles & Permissions
// screen's GET /roles, scoped so a tenant only ever sees its own 7-role
// catalog, never every tenant's rows concatenated together.
func (r *Repository) ListRoles(ctx context.Context, tenantID int64) ([]Role, error) {
	var roles []Role
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("name ASC").Find(&roles).Error
	return roles, err
}

// FindRoleByID looks up id with NO tenant filter — kept only for
// auth.Service/tenant.MembershipService's roleLookup/roleReader
// interfaces, whose callers already resolve id from a tenant_users row
// whose role_id is app-layer-guaranteed to belong to that membership's
// own tenant (migrations/0042's header comment), and for the legacy,
// platform-global /api/v1/users module (internal/user), which has no
// tenant context to filter by at all. New tenant-facing code — like
// this package's own Service — must use FindRoleByIDForTenant instead;
// see that method's doc for why an :id path/query parameter can never
// safely go through this one.
func (r *Repository) FindRoleByID(ctx context.Context, id int64) (*Role, error) {
	var role Role
	err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// FindRoleByIDForTenant returns id's row scoped to tenantID, or
// ErrNotFound — never "found but belongs to another tenant" vs.
// "doesn't exist at all", so a guessed ID can't be distinguished from a
// nonexistent one (tenant-isolation-security.md §6). Every role.Service
// method backing a user-supplied :id must go through this, not
// FindRoleByID.
func (r *Repository) FindRoleByIDForTenant(ctx context.Context, tenantID, id int64) (*Role, error) {
	var role Role
	err := r.db.WithContext(ctx).First(&role, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// FindRoleByCode looks up code alone, with NO tenant filter — since
// UQ_roles_tenant_code (migrations/0048) means the same code now exists
// once per tenant, this is ambiguous for any caller that doesn't already
// know which tenant's row it wants, and will silently return whichever
// tenant's row SQL Server happens to pick first. Kept only for the
// legacy, platform-global /api/v1/users module (internal/user), which
// has no tenant context to disambiguate with. New code needing a
// tenant's own role by code must query tenant.Repository's
// FindTenantRoleByCode(tenantID, code) instead — see that method's own
// doc for why it, not this one, is the correct lookup.
func (r *Repository) FindRoleByCode(ctx context.Context, code string) (*Role, error) {
	var role Role
	err := r.db.WithContext(ctx).First(&role, "code = ?", code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) ListPermissions(ctx context.Context) ([]Permission, error) {
	var permissions []Permission
	err := r.db.WithContext(ctx).Order("module ASC, action ASC").Find(&permissions).Error
	return permissions, err
}

// PermissionsByCodes returns only the catalog rows matching codes, so
// callers can detect codes that don't exist (len(result) < len(codes)).
func (r *Repository) PermissionsByCodes(ctx context.Context, codes []string) ([]Permission, error) {
	var permissions []Permission
	err := r.db.WithContext(ctx).Where("code IN ?", codes).Find(&permissions).Error
	return permissions, err
}

// RolePermissionCodes returns the permission codes currently granted to
// roleID, joined against the catalog so the caller gets codes rather than
// bare IDs.
func (r *Repository) RolePermissionCodes(ctx context.Context, roleID int64) ([]string, error) {
	var codes []string
	err := r.db.WithContext(ctx).
		Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.module ASC, permissions.action ASC").
		Pluck("permissions.code", &codes).Error
	return codes, err
}

// ReplaceRolePermissions atomically swaps roleID's full grant set for
// permissionIDs — the Roles & Permissions screen's "Save Permissions"
// button always submits the complete matrix state, not a delta.
// tenantID is re-verified against roles.tenant_id inside the same
// transaction as the delete+insert (defense in depth on top of
// Service's own FindRoleByIDForTenant existence check, which runs
// before this method is ever called) — returns ErrNotFound rather than
// silently mutating another tenant's role if the two ever disagree.
func (r *Repository) ReplaceRolePermissions(ctx context.Context, tenantID, roleID int64, permissionIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owned int64
		if err := tx.Model(&Role{}).Where("id = ? AND tenant_id = ?", roleID, tenantID).Count(&owned).Error; err != nil {
			return err
		}
		if owned == 0 {
			return ErrNotFound
		}

		if err := tx.Where("role_id = ?", roleID).Delete(&RolePermission{}).Error; err != nil {
			return err
		}

		if len(permissionIDs) == 0 {
			return nil
		}

		grants := make([]RolePermission, 0, len(permissionIDs))
		for _, pid := range permissionIDs {
			grants = append(grants, RolePermission{RoleID: roleID, PermissionID: pid})
		}
		return tx.Create(&grants).Error
	})
}

// RoleWarehouseAccessIDs returns the warehouse_ids currently granted to
// roleID — joined against roles so a roleID belonging to another tenant
// (it should never reach this method at all, given Service's own
// FindRoleByIDForTenant check, but role_warehouse_access carries no
// tenant_id of its own to filter by directly) resolves to an empty set
// rather than another tenant's grants.
func (r *Repository) RoleWarehouseAccessIDs(ctx context.Context, tenantID, roleID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&RoleWarehouseAccess{}).
		Joins("JOIN roles ON roles.id = role_warehouse_access.role_id").
		Where("role_warehouse_access.role_id = ? AND roles.tenant_id = ?", roleID, tenantID).
		Order("role_warehouse_access.warehouse_id ASC").
		Pluck("role_warehouse_access.warehouse_id", &ids).Error
	return ids, err
}

// ReplaceRoleWarehouseAccess atomically swaps roleID's full warehouse
// access set for warehouseIDs — the same delete-then-insert-in-one-
// transaction shape as ReplaceRolePermissions, for the same reason: the
// Warehouse Access panel always submits the complete desired set, not a
// delta. createdBy is attributed to every grant row this call creates.
// tenantID is re-verified against roles.tenant_id inside the same
// transaction, mirroring ReplaceRolePermissions' own defense-in-depth
// check.
func (r *Repository) ReplaceRoleWarehouseAccess(ctx context.Context, tenantID, roleID int64, warehouseIDs []int64, createdBy *int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owned int64
		if err := tx.Model(&Role{}).Where("id = ? AND tenant_id = ?", roleID, tenantID).Count(&owned).Error; err != nil {
			return err
		}
		if owned == 0 {
			return ErrNotFound
		}

		if err := tx.Where("role_id = ?", roleID).Delete(&RoleWarehouseAccess{}).Error; err != nil {
			return err
		}

		if len(warehouseIDs) == 0 {
			return nil
		}

		grants := make([]RoleWarehouseAccess, 0, len(warehouseIDs))
		for _, warehouseID := range warehouseIDs {
			grants = append(grants, RoleWarehouseAccess{RoleID: roleID, WarehouseID: warehouseID, CreatedBy: createdBy})
		}
		return tx.Create(&grants).Error
	})
}
