package tenant

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrMembershipNotFound covers "no such tenant_users row", "row exists but
// is not ACTIVE", "role no longer exists/active", and "tenant is not
// operational" identically and deliberately — the same same-error-either-
// way rule docs/proposals/wms-saas/tenant-isolation-security.md §6 applies
// to cross-tenant resource lookups applies here too: a caller must not be
// able to distinguish "you were removed" from "that tenant never existed".
var ErrMembershipNotFound = errors.New("tenant: active membership not found")

// operationalStatuses are the tenants.status values a membership may be
// resolved against. SUSPENDED/CANCELLED tenants never grant access, even to
// a user whose own tenant_users row is still ACTIVE — tenant-isolation-
// security.md §10's fail-closed table.
var operationalStatuses = []string{TenantStatusTrial, TenantStatusActive}

// Membership is one resolved (tenant_users ACTIVE + tenant operational +
// role active) row — everything auth needs to issue or re-verify a
// tenant-scoped session, without exposing the raw TenantUser/Tenant/Role
// GORM models to callers outside this package.
type Membership struct {
	TenantID   int64
	TenantSlug string
	TenantName string
	RoleID     int64
}

// Repository is a thin GORM wrapper over tenants/tenant_users, scoped to
// what membership resolution needs — it does not touch tenant_settings,
// tenant_subscriptions, or tenant_audit_logs (schema-only per Phase 28,
// none of auth's concern).
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// membershipQuery is the shared join behind both ActiveMemberships and
// ActiveMembership: tenant_users.status = ACTIVE, tenants.status
// operational, and the membership's role still exists and is active — the
// three conditions tenant-isolation-security.md §3 step 2 requires before
// any downstream code trusts a tenant context.
func (r *Repository) membershipQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("tenant_users").
		Select("tenant_users.tenant_id AS tenant_id, tenants.slug AS tenant_slug, tenants.name AS tenant_name, tenant_users.role_id AS role_id").
		Joins("JOIN tenants ON tenants.id = tenant_users.tenant_id").
		Joins("JOIN roles ON roles.id = tenant_users.role_id AND roles.is_active = 1").
		Where("tenant_users.status = ? AND tenants.status IN ?", TenantUserStatusActive, operationalStatuses)
}

// ActiveMemberships returns every tenant userID currently holds an ACTIVE,
// operational-tenant membership in, ordered by tenant_id ascending so a
// caller that needs one deterministic choice (e.g. login auto-select, see
// Service.DefaultMembership) can simply take the first entry.
func (r *Repository) ActiveMemberships(ctx context.Context, userID int64) ([]Membership, error) {
	var rows []Membership
	err := r.membershipQuery(ctx).
		Where("tenant_users.user_id = ?", userID).
		Order("tenant_users.tenant_id ASC").
		Scan(&rows).Error
	return rows, err
}

// ActiveMembership resolves exactly one (userID, tenantID) pair — the
// server-side re-check tenant-isolation-security.md §2/§3 requires before
// trusting a client-supplied tenant_id (switch-tenant) or re-validating one
// already embedded in a JWT (RequireTenantContext).
func (r *Repository) ActiveMembership(ctx context.Context, userID, tenantID int64) (*Membership, error) {
	var m Membership
	err := r.membershipQuery(ctx).
		Where("tenant_users.user_id = ? AND tenant_users.tenant_id = ?", userID, tenantID).
		Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMembershipNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
