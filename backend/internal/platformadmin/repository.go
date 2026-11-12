package platformadmin

import (
	"context"

	"gorm.io/gorm"
)

// Repository is a thin GORM wrapper over platform_admins — intentionally
// just one query. Granting/revoking platform-admin status is not exposed
// through this phase's API (see model.go's package doc); this type only
// answers the one question middleware.RequirePlatformAdmin needs asked
// fresh on every request.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// IsPlatformAdmin reports whether userID currently holds a non-revoked
// platform_admins row. Deliberately a boolean, not a "find the row"
// lookup — middleware.RequirePlatformAdmin has no use for anything else
// on the row, and a boolean keeps the authorization check from ever
// being tempted to trust stale row data instead of re-querying.
//
// This is checked directly against the database on every call, with no
// caching layer — unlike middleware.RequireTenantContext's membership
// cache, platform-admin status gates the highest-privilege API in the
// system (cross-tenant tenant management), so a revocation must take
// effect on the very next request, not after a TTL window elapses.
func (r *Repository) IsPlatformAdmin(ctx context.Context, userID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Admin{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
