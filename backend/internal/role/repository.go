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

func (r *Repository) ListRoles(ctx context.Context) ([]Role, error) {
	var roles []Role
	err := r.db.WithContext(ctx).Order("name ASC").Find(&roles).Error
	return roles, err
}

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
func (r *Repository) ReplaceRolePermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
// roleID — pure role_warehouse_access data, with no join into
// warehouses itself (see Service.warehouseSummaries for the resolution
// step, mirroring how RolePermissionCodes/ReplaceRolePermissions keep
// package role's repository self-contained to its own tables).
func (r *Repository) RoleWarehouseAccessIDs(ctx context.Context, roleID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&RoleWarehouseAccess{}).
		Where("role_id = ?", roleID).
		Order("warehouse_id ASC").
		Pluck("warehouse_id", &ids).Error
	return ids, err
}

// ReplaceRoleWarehouseAccess atomically swaps roleID's full warehouse
// access set for warehouseIDs — the same delete-then-insert-in-one-
// transaction shape as ReplaceRolePermissions, for the same reason: the
// Warehouse Access panel always submits the complete desired set, not a
// delta. createdBy is attributed to every grant row this call creates.
func (r *Repository) ReplaceRoleWarehouseAccess(ctx context.Context, roleID int64, warehouseIDs []int64, createdBy *int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
