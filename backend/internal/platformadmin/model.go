// Package platformadmin owns the platform_admins grant table — the
// Platform Super Admin persona, kept deliberately separate from any
// tenant's RBAC (see docs/proposals/wms-saas/tenant-er-design.md §4: a
// bug in tenant-role resolution must never also be a platform-privilege
// bug). Schema-only for Phase 28; the RequirePlatformAdmin middleware
// and grant/revoke handlers are a later phase
// (docs/proposals/wms-saas/saas-module-roadmap.md's Phase 33).
package platformadmin

import "time"

// Admin is the GORM model for the platform_admins table
// (migrations/0041). Presence of a non-null RevokedAt means the row's
// subject is not currently a platform admin — the row is kept for audit
// history rather than deleted.
type Admin struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	UserID    int64      `gorm:"column:user_id"`
	GrantedBy *int64     `gorm:"column:granted_by"`
	GrantedAt time.Time  `gorm:"column:granted_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

func (Admin) TableName() string { return "platform_admins" }
