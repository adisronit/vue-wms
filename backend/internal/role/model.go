// Package role owns the roles and permissions catalog and the
// role_permissions grant table. It is the read side (plus the one grant
// mutation, PUT .../permissions) the Roles & Permissions screen needs;
// it does not implement role create/delete because the existing frontend
// never actually persists those actions either (see PROJECT_STATUS.md).
package role

import "time"

// Role is the GORM model for the roles table (migrations/0001).
type Role struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	IsActive    bool      `gorm:"column:is_active"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Role) TableName() string { return "roles" }

// Permission is the GORM model for the permissions table
// (migrations/0002) — the flat catalog the Roles & Permissions screen's
// module x action matrix is rendered from.
type Permission struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	Code        string `gorm:"column:code"`
	Module      string `gorm:"column:module"`
	Action      string `gorm:"column:action"`
	Description string `gorm:"column:description"`
}

func (Permission) TableName() string { return "permissions" }

// RolePermission is the GORM model for the role_permissions join table
// (migrations/0003).
type RolePermission struct {
	RoleID       int64 `gorm:"column:role_id"`
	PermissionID int64 `gorm:"column:permission_id"`
}

func (RolePermission) TableName() string { return "role_permissions" }

// RoleWarehouseAccess is the GORM model for the role_warehouse_access
// join table (migrations/0040) — which warehouses a role's users may
// operate against. A plain link table like RolePermission; created_at/
// created_by exist for audit traceability since warehouse-scoped access
// is a more security-sensitive grant than an individual permission
// checkbox.
type RoleWarehouseAccess struct {
	RoleID      int64     `gorm:"column:role_id"`
	WarehouseID int64     `gorm:"column:warehouse_id"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	CreatedBy   *int64    `gorm:"column:created_by"`
}

func (RoleWarehouseAccess) TableName() string { return "role_warehouse_access" }
