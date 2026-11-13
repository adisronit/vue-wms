package tenant

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/role"
	"wms-backend/internal/user"
)

// ErrMembershipRowNotFound is returned by the membership-management
// lookups (Get/ChangeRole/ChangeStatus) when no tenant_users row exists
// for a given (tenantID, userID) pair. Deliberately distinct from
// ErrMembershipNotFound above: that error exists specifically to look
// identical to "tenant doesn't exist" for the fail-closed auth re-check
// path; this one is a plain, precise 404 for an admin who already knows
// both the tenant and the user exist and is asking about their
// membership specifically.
var ErrMembershipRowNotFound = errors.New("tenant: membership not found")

// ErrMembershipDuplicate is returned when a (tenant_id, user_id) pair
// already has a tenant_users row (UQ_tenant_users_tenant_user,
// migrations/0042) — a user cannot hold two memberships in the same
// tenant.
var ErrMembershipDuplicate = errors.New("tenant: user already has a membership in this tenant")

// ErrLinkUserNotFound is returned by the "link an existing user" create
// flow when no users row matches the exact identifier supplied — kept
// distinct from ErrMembershipRowNotFound/ErrNotFound so handlers report
// the precise failure.
var ErrLinkUserNotFound = errors.New("tenant: no matching user found")

// MembershipRow is one tenant_users row joined to its user and role for
// display — what ListMemberships/FindMembershipRow return, so callers
// never need to juggle three separate GORM models to render one row of
// the Tenant Users screen.
type MembershipRow struct {
	ID        int64
	TenantID  int64
	UserID    int64
	Username  string
	FullName  string
	Email     string
	RoleID    int64
	RoleCode  string
	RoleName  string
	Status    string
	InvitedBy *int64
	InvitedAt *time.Time
	JoinedAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MembershipListFilter narrows ListMemberships to the Tenant Users tab's
// search box and role/status filters, with pagination — the same
// ListFilter/pagination shape every other module's Repository.List
// already uses.
type MembershipListFilter struct {
	Search   string // matched against full_name, username, email (LIKE)
	RoleCode string
	Status   string
	Page     int
	Limit    int
	SortBy   string // one of membershipSortColumns' keys; falls back to "name" if not
	SortDir  string // "ASC" or "DESC"; falls back to "ASC" otherwise
}

// membershipSortColumns maps ListMemberships' logical SortBy key to a
// fully table-qualified column name — required because this query joins
// tenant_users/users/roles, and "status"/"id" exist on more than one of
// those tables (an unqualified ORDER BY would be ambiguous to SQL
// Server, not just wrong). A literal allowlist, never built from
// request input, mirroring internal/productcategory's repository.go.
var membershipSortColumns = map[string]string{
	"name":      "users.full_name",
	"role":      "roles.name",
	"status":    "tenant_users.status",
	"joinedAt":  "tenant_users.joined_at",
}

// membershipRowQuery is the shared join behind ListMemberships and
// FindMembershipRow: tenant_users joined to users (for display name/
// email) and roles (for the role code/name shown in the UI) — never
// exposing the raw TenantUser/User/Role GORM models to callers outside
// this package.
func (r *Repository) membershipRowQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("tenant_users").
		Joins("JOIN users ON users.id = tenant_users.user_id").
		Joins("JOIN roles ON roles.id = tenant_users.role_id")
}

func applyMembershipFilter(q *gorm.DB, filter MembershipListFilter) *gorm.DB {
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("users.full_name LIKE ? OR users.username LIKE ? OR users.email LIKE ?", like, like, like)
	}
	if filter.RoleCode != "" {
		q = q.Where("roles.code = ?", filter.RoleCode)
	}
	if filter.Status != "" {
		q = q.Where("tenant_users.status = ?", filter.Status)
	}
	return q
}

const membershipRowSelect = `tenant_users.id AS id, tenant_users.tenant_id AS tenant_id, tenant_users.user_id AS user_id,
	users.username AS username, users.full_name AS full_name, users.email AS email,
	tenant_users.role_id AS role_id, roles.code AS role_code, roles.name AS role_name,
	tenant_users.status AS status, tenant_users.invited_by AS invited_by,
	tenant_users.invited_at AS invited_at, tenant_users.joined_at AS joined_at,
	tenant_users.created_at AS created_at, tenant_users.updated_at AS updated_at`

// ListMemberships returns the page of tenant_users rows matching filter
// for tenantID, plus the total matching count (ignoring pagination), for
// the Tenant Users tab's list envelope.
func (r *Repository) ListMemberships(ctx context.Context, tenantID int64, filter MembershipListFilter) ([]MembershipRow, int64, error) {
	base := applyMembershipFilter(r.membershipRowQuery(ctx).Where("tenant_users.tenant_id = ?", tenantID), filter)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	column, ok := membershipSortColumns[filter.SortBy]
	if !ok {
		column = membershipSortColumns["name"]
	}
	dir := "ASC"
	if filter.SortDir == "DESC" {
		dir = "DESC"
	}
	// "tenant_users.id ASC" (qualified for the same ambiguity reason as
	// above) is a deterministic tie-breaker.
	order := column + " " + dir + ", tenant_users.id ASC"

	var rows []MembershipRow
	offset := (filter.Page - 1) * filter.Limit
	err := base.Select(membershipRowSelect).
		Order(order).
		Offset(offset).Limit(filter.Limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// FindMembershipRow returns tenantID/userID's joined display row, or
// ErrMembershipRowNotFound.
func (r *Repository) FindMembershipRow(ctx context.Context, tenantID, userID int64) (*MembershipRow, error) {
	var row MembershipRow
	err := r.membershipRowQuery(ctx).
		Select(membershipRowSelect).
		Where("tenant_users.tenant_id = ? AND tenant_users.user_id = ?", tenantID, userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMembershipRowNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// FindMembership returns the raw tenant_users row for tenantID/userID
// (no join) — used internally before a role/status mutation, where the
// service needs the bare RoleID/Status/ID to build an audit log and run
// the last-active-admin guard, not the display-resolved MembershipRow.
func (r *Repository) FindMembership(ctx context.Context, tenantID, userID int64) (*TenantUser, error) {
	var m TenantUser
	err := r.db.WithContext(ctx).Take(&m, "tenant_id = ? AND user_id = ?", tenantID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMembershipRowNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindTenantRoleByCode resolves code to tenantID's own role row.
// Deliberately NOT role.Repository.FindRoleByCode, which looks up by
// code alone and would be ambiguous now that roles.tenant_id +
// UQ_roles_tenant_code (migrations/0048) means the same code exists
// once per tenant — using that method here would silently resolve to
// whichever tenant's row the database happens to return first. A
// tenant_users.role_id must always resolve to a role owned by that same
// tenant_id (migrations/0042's own header comment); this is the one
// lookup in this codebase that enforces that correctly from the start.
func (r *Repository) FindTenantRoleByCode(ctx context.Context, tenantID int64, code string) (*role.Role, error) {
	var rl role.Role
	err := r.db.WithContext(ctx).Take(&rl, "tenant_id = ? AND code = ?", tenantID, code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, role.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rl, nil
}

// CountActiveAdmins counts tenantID's ACTIVE tenant_users rows whose
// role currently has code "ADMIN" and belongs to tenantID itself (the
// join condition guards against a role_id that happens to share an ID
// with another tenant's differently-coded role, though that can't
// actually happen given FindTenantRoleByCode's own scoping — kept for
// defense in depth). Used by the last-active-admin protection guard
// before any role/status change that would remove a membership's
// ADMIN-ness.
func (r *Repository) CountActiveAdmins(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("tenant_users").
		Joins("JOIN roles ON roles.id = tenant_users.role_id AND roles.tenant_id = tenant_users.tenant_id").
		Where("tenant_users.tenant_id = ? AND tenant_users.status = ? AND roles.code = ?", tenantID, TenantUserStatusActive, tenantAdminRoleCode).
		Count(&count).Error
	return count, err
}

// CreateMembershipWithNewUser inserts newUser and, in the same
// transaction, membership (which must already carry RoleID — TenantID/
// UserID are set here once newUser.ID is known) and an audit log row.
// Returns user.ErrDuplicate if newUser's username/email collides
// (UQ_users_username/UQ_users_email), ErrMembershipDuplicate if the
// resulting (tenant_id, user_id) pair already has a row (can only
// happen for a race against a concurrent request — newUser.ID is brand
// new on every call).
func (r *Repository) CreateMembershipWithNewUser(ctx context.Context, tenantID int64, newUser *user.User, membership *TenantUser, buildLog func(membershipID int64) *TenantAuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(newUser).Error; err != nil {
			if isUniqueViolation(err) {
				return user.ErrDuplicate
			}
			return err
		}

		membership.TenantID = tenantID
		membership.UserID = newUser.ID
		if err := tx.Create(membership).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrMembershipDuplicate
			}
			return err
		}

		return r.insertAuditLog(tx, buildLog(membership.ID))
	})
}

// CreateMembershipWithExistingUser inserts membership (TenantID/UserID/
// RoleID already set by the caller) and, in the same transaction, an
// audit log row. Returns ErrMembershipDuplicate on a (tenant_id,
// user_id) collision.
func (r *Repository) CreateMembershipWithExistingUser(ctx context.Context, membership *TenantUser, buildLog func(membershipID int64) *TenantAuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(membership).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrMembershipDuplicate
			}
			return err
		}
		return r.insertAuditLog(tx, buildLog(membership.ID))
	})
}

// UpdateMembershipRole transitions (tenantID, userID)'s role_id and, in
// the same transaction, inserts log. Returns ErrMembershipRowNotFound if
// no such row exists.
func (r *Repository) UpdateMembershipRole(ctx context.Context, tenantID, userID, roleID int64, log *TenantAuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&TenantUser{}).
			Where("tenant_id = ? AND user_id = ?", tenantID, userID).
			Update("role_id", roleID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMembershipRowNotFound
		}
		return r.insertAuditLog(tx, log)
	})
}

// UpdateMembershipStatus transitions (tenantID, userID)'s status and, in
// the same transaction, stamps joinedAt (nil leaves the column
// unchanged — passing the existing value back is the caller's job, the
// same "caller computes, repository persists" split
// UpdateStatusWithAudit above already uses) and inserts log. Returns
// ErrMembershipRowNotFound if no such row exists.
func (r *Repository) UpdateMembershipStatus(ctx context.Context, tenantID, userID int64, status string, joinedAt *time.Time, log *TenantAuditLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&TenantUser{}).
			Where("tenant_id = ? AND user_id = ?", tenantID, userID).
			Updates(map[string]any{"status": status, "joined_at": joinedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMembershipRowNotFound
		}
		return r.insertAuditLog(tx, log)
	})
}
