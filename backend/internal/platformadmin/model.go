// Package platformadmin owns the platform_admins grant table — the
// Platform Super Admin persona, kept deliberately separate from any
// tenant's RBAC (see docs/proposals/wms-saas/tenant-er-design.md §4: a
// bug in tenant-role resolution must never also be a platform-privilege
// bug).
//
// Schema-only through Phase 28. This phase (the Tenant Management API,
// WMS-29) adds just enough to authorize that API without touching
// tenant-level RBAC at all: Repository.IsPlatformAdmin, consumed by
// middleware.RequirePlatformAdmin. Grant/revoke handlers for
// platform_admins itself (who gets to become a platform admin) remain a
// later phase (saas-module-roadmap.md's Phase 33) — today a row is
// granted by hand (direct SQL), the same bootstrapping shape the
// project's seeded dev admin user already uses.
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
