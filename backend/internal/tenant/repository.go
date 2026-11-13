package tenant

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

// ErrMembershipNotFound covers "no such tenant_users row", "row exists but
// is not ACTIVE", "role no longer exists/active", and "tenant is not
// operational" identically and deliberately — the same same-error-either-
// way rule docs/proposals/wms-saas/tenant-isolation-security.md §6 applies
// to cross-tenant resource lookups applies here too: a caller must not be
// able to distinguish "you were removed" from "that tenant never existed".
var ErrMembershipNotFound = errors.New("tenant: active membership not found")

// ErrNotFound covers "no tenant with this id" for the Platform Admin
// tenant-management API (WMS-29) — distinct from ErrMembershipNotFound,
// which is about one (user, tenant) pair's membership, not the tenant
// row's own existence.
var ErrNotFound = errors.New("tenant: not found")

// ErrDuplicate is returned when a Create/Update would violate
// UQ_tenants_slug (migrations/0041) — a platform admin trying to create
// or rename a tenant onto a slug another tenant already holds.
var ErrDuplicate = errors.New("tenant: slug already exists")

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

// Repository is a thin GORM wrapper over tenants/tenant_users/
// tenant_audit_logs. Through Phase 29B this type only ever read
// tenant_users for membership resolution; WMS-29's Platform Admin API
// adds the tenant-row CRUD and audit-log writes below. It still does
// not touch tenant_settings or tenant_subscriptions — those remain
// schema-only, no code path in this codebase reads or writes them yet.
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

// ListFilter narrows GET /api/v1/platform/tenants to the Tenant
// Management screen's search box and status filter, with pagination —
// the same ListFilter/pagination shape warehouse.Repository.List and
// product.Repository.List already use.
type ListFilter struct {
	Search string // matched against slug, name (LIKE)
	Status string
	Page   int
	Limit  int
}

// FindByID returns tenantID's row, or ErrNotFound. Deliberately not
// "not found or belongs to another tenant" like every other resource's
// FindByID in this codebase (tenant-isolation-security.md §6) — a
// tenant row has no parent tenant to be scoped by; the Platform Admin
// API is the one place in this system that legitimately operates across
// every tenant at once, gated by middleware.RequirePlatformAdmin instead.
func (r *Repository) FindByID(ctx context.Context, id int64) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List returns the tenants matching filter plus the total matching
// count (ignoring pagination), for the list envelope's meta.total.
func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Tenant, int64, error) {
	base := r.db.WithContext(ctx).Model(&Tenant{})

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		base = base.Where("slug LIKE ? OR name LIKE ?", like, like)
	}
	if filter.Status != "" {
		base = base.Where("status = ?", filter.Status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tenants []Tenant
	offset := (filter.Page - 1) * filter.Limit
	if err := base.Order("name ASC").Offset(offset).Limit(filter.Limit).Find(&tenants).Error; err != nil {
		return nil, 0, err
	}

	return tenants, total, nil
}

// InsertAuditLog appends one tenant_audit_logs row. Called only from
// inside the *WithAudit transactional methods below — never on its own
// — so a tenant mutation and the audit record of it either both commit
// or both roll back together (CLAUDE.md's "Database Transactions"
// rule), never one without the other.
func (r *Repository) insertAuditLog(tx *gorm.DB, log *TenantAuditLog) error {
	return tx.Create(log).Error
}

// CreateWithAudit inserts t and, in the same transaction, an audit log
// row describing the creation — buildLog receives the newly assigned
// t.ID so it can set TargetID without a second round trip. Returns
// ErrDuplicate if t.Slug collides with UQ_tenants_slug.
func (r *Repository) CreateWithAudit(ctx context.Context, t *Tenant, buildLog func(tenantID int64) *TenantAuditLog) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		return r.insertAuditLog(tx, buildLog(t.ID))
	})
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

// UpdateWithAudit updates t's Slug/Name (the only platform-admin-
// editable fields — Status transitions go through UpdateStatusWithAudit
// instead, since those carry their own timestamp side effects) and
// inserts log in the same transaction. Returns ErrNotFound if t.ID
// doesn't exist, ErrDuplicate on a slug collision.
func (r *Repository) UpdateWithAudit(ctx context.Context, t *Tenant, log *TenantAuditLog) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Tenant{}).Where("id = ?", t.ID).Updates(map[string]any{
			"slug": t.Slug,
			"name": t.Name,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return r.insertAuditLog(tx, log)
	})
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

// UpdateStatusWithAudit transitions id's status and, in the same
// transaction, stamps suspendedAt/cancelledAt (each nil clears the
// corresponding column — e.g. reactivating a SUSPENDED tenant back to
// ACTIVE clears suspended_at rather than leaving a stale timestamp on an
// otherwise-active tenant) and inserts log. Returns ErrNotFound if id
// doesn't exist.
func (r *Repository) UpdateStatusWithAudit(ctx context.Context, id int64, status string, suspendedAt, cancelledAt *time.Time, log *TenantAuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Tenant{}).Where("id = ?", id).Updates(map[string]any{
			"status":       status,
			"suspended_at": suspendedAt,
			"cancelled_at": cancelledAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return r.insertAuditLog(tx, log)
	})
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier every other module's repository already uses (see
// internal/warehouse/repository.go).
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}
