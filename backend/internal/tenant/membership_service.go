package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/role"
	"wms-backend/internal/user"
)

// tenantAdminRoleCode is the role code this codebase treats as "the
// tenant's own administrator" — the per-tenant equivalent of the
// Platform Admin persona, but implemented as an ordinary tenant-scoped
// role (rbac.RoleCatalog's "ADMIN") rather than a separate table, since
// every tenant is seeded with its own ADMIN row (see
// Repository.seedRoleCatalog). EnsureActingTenantAdmin and
// guardLastActiveAdmin both key off this exact code.
const tenantAdminRoleCode = "ADMIN"

// minMembershipPasswordLength mirrors internal/user's own minimum —
// kept as a separate constant (not an import of user.minPasswordLength,
// which is unexported) so this package's validation doesn't silently
// drift if user's own minimum ever changes without this phase's
// author's attention.
const minMembershipPasswordLength = 8

// ErrLastActiveAdmin is returned when a role or status change would
// leave a tenant with zero ACTIVE memberships holding the ADMIN role —
// WMS-29E Phase 4's "protect the last active Tenant Admin from
// accidental suspension or removal" rule.
var ErrLastActiveAdmin = errors.New("tenant: cannot remove the tenant's last active administrator")

// ErrNotTenantAdmin is returned by EnsureActingTenantAdmin when the
// caller does not themselves hold an ACTIVE ADMIN membership in the
// target tenant — the tenant-admin-scoped handler's extra authorization
// gate on top of RequirePermission (see that handler's own doc for why
// a permission grant alone is not sufficient here).
var ErrNotTenantAdmin = errors.New("tenant: caller is not this tenant's administrator")

// ErrCannotModifySelf is returned by the tenant-admin-scoped handler's
// self-modification guard — see EnsureNotSelf.
var ErrCannotModifySelf = errors.New("tenant: cannot change your own membership through this endpoint")

// userLookup is the narrow slice of user.Repository the membership
// service needs: resolving a brand-new user's uniqueness (handled by
// CreateMembershipWithNewUser's own insert) and resolving an existing
// user by exact identifier for the "link existing user" flow.
type userLookup interface {
	FindByUsernameOrEmail(ctx context.Context, identifier string) (*user.User, error)
}

// roleLookup is the narrow slice of role.Repository the membership
// service needs to resolve a role_id back to its code/name for the
// last-active-admin guard and for EnsureActingTenantAdmin.
type roleLookup interface {
	FindRoleByID(ctx context.Context, id int64) (*role.Role, error)
}

// membershipManagementRepository is the slice of *Repository the
// membership service needs — its own interface (mirroring
// membershipRepository/managementRepository's existing split) so a unit
// test can supply a fake without a real SQL Server connection.
type membershipManagementRepository interface {
	ListMemberships(ctx context.Context, tenantID int64, filter MembershipListFilter) ([]MembershipRow, int64, error)
	FindMembershipRow(ctx context.Context, tenantID, userID int64) (*MembershipRow, error)
	FindMembership(ctx context.Context, tenantID, userID int64) (*TenantUser, error)
	FindTenantRoleByCode(ctx context.Context, tenantID int64, code string) (*role.Role, error)
	CreateMembershipWithNewUser(ctx context.Context, tenantID int64, newUser *user.User, membership *TenantUser, buildLog func(int64) *TenantAuditLog) error
	CreateMembershipWithExistingUser(ctx context.Context, membership *TenantUser, buildLog func(int64) *TenantAuditLog) error
	UpdateMembershipRole(ctx context.Context, tenantID, userID, roleID int64, log *TenantAuditLog) error
	UpdateMembershipStatus(ctx context.Context, tenantID, userID int64, status string, joinedAt *time.Time, log *TenantAuditLog) error
	CountActiveAdmins(ctx context.Context, tenantID int64) (int64, error)
}

// MembershipService implements the Tenant User & Membership Management
// business rules (WMS-29E) — validation, user provisioning/linking,
// role/status transitions, the last-active-admin guard, and audit
// logging. It is deliberately a separate type from Service (rather than
// extra fields bolted onto it) so NewService's existing two-argument
// signature — used by a dozen existing tests across management_test.go
// and service_test.go — never needs to change for this phase's work.
// Both Platform Admin and Tenant Admin handlers share one
// MembershipService instance; the difference between those two personas
// is entirely in which middleware/handler calls it and with what
// ActorType, never in this type itself.
type MembershipService struct {
	repo  membershipManagementRepository
	users userLookup
	roles roleLookup
	// tenants resolves whether a tenant row exists at all, for the
	// Platform Admin create/list path's "tenant not found" 404 — the
	// same managementRepository slice Service.management already uses.
	tenants managementRepository
}

func NewMembershipService(repo membershipManagementRepository, users userLookup, roles roleLookup, tenants managementRepository) *MembershipService {
	return &MembershipService{repo: repo, users: users, roles: roles, tenants: tenants}
}

// ListMemberships returns tenantID's membership list for the Tenant
// Users tab, applying the same page/limit defaulting/clamping
// convention every other module's List already uses. Returns ErrNotFound
// if tenantID doesn't exist.
func (s *MembershipService) ListMemberships(ctx context.Context, tenantID int64, params MembershipListParams) ([]MembershipResponse, int64, error) {
	if _, err := s.tenants.FindByID(ctx, tenantID); err != nil {
		return nil, 0, err
	}

	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	rows, total, err := s.repo.ListMemberships(ctx, tenantID, MembershipListFilter{
		Search: params.Search, RoleCode: params.RoleCode, Status: params.Status, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]MembershipResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, toMembershipResponse(row))
	}
	return responses, total, nil
}

// GetMembership returns one membership's detail. Returns
// ErrMembershipRowNotFound if (tenantID, userID) has no row.
func (s *MembershipService) GetMembership(ctx context.Context, tenantID, userID int64) (MembershipResponse, error) {
	row, err := s.repo.FindMembershipRow(ctx, tenantID, userID)
	if err != nil {
		return MembershipResponse{}, err
	}
	return toMembershipResponse(*row), nil
}

// CreateMembership validates req and dispatches to the NEW or EXISTING
// provisioning flow — see those methods' own docs. actorType is stamped
// onto the audit log (AuditActorPlatformAdmin or AuditActorTenantAdmin);
// it is never client-supplied.
func (s *MembershipService) CreateMembership(ctx context.Context, tenantID int64, req CreateMembershipRequest, actorUserID int64, actorType string) (MembershipResponse, error) {
	if _, err := s.tenants.FindByID(ctx, tenantID); err != nil {
		return MembershipResponse{}, err
	}

	targetRole, err := s.repo.FindTenantRoleByCode(ctx, tenantID, req.RoleCode)
	if errors.Is(err, role.ErrNotFound) {
		return MembershipResponse{}, ErrValidation{Message: "role_code does not match any role in this tenant"}
	}
	if err != nil {
		return MembershipResponse{}, err
	}

	switch req.Mode {
	case MembershipModeNew:
		return s.createWithNewUser(ctx, tenantID, req, targetRole, actorUserID, actorType)
	case MembershipModeExisting:
		return s.createWithExistingUser(ctx, tenantID, req, targetRole, actorUserID, actorType)
	default:
		return MembershipResponse{}, ErrValidation{Message: "mode must be NEW or EXISTING"}
	}
}

// createWithNewUser is the "Create a user and assign the user to a
// tenant" flow (WMS-29E Phase 1, capability 2). It reuses internal/
// user's own password-hashing convention (bcrypt, DefaultCost) rather
// than inventing a second one, and the new membership starts INVITED —
// matching tenant_users' own DF_tenant_users_status default
// (migrations/0042) — because no email-invitation/password-setup
// infrastructure exists in this codebase to actually deliver an
// invitation (CLAUDE.md Phase 5: "if email delivery or password setup
// infrastructure is not available, do not fabricate a working
// invitation feature"). The admin creating this account must choose and
// share the initial password themselves, exactly as internal/user's own
// CreateRequest already requires for the legacy Users screen; it is
// never logged or echoed back in the response. The account's own
// user.Status is set ACTIVE immediately (it is a real, usable login the
// moment it's created) — INVITED only describes this specific tenant
// membership, not the underlying account.
func (s *MembershipService) createWithNewUser(ctx context.Context, tenantID int64, req CreateMembershipRequest, targetRole *role.Role, actorUserID int64, actorType string) (MembershipResponse, error) {
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.FullName) == "" || strings.TrimSpace(req.Email) == "" {
		return MembershipResponse{}, ErrValidation{Message: "username, full_name, and email are required for mode=NEW"}
	}
	if len(req.Password) < minMembershipPasswordLength {
		return MembershipResponse{}, ErrValidation{Message: fmt.Sprintf("password must be at least %d characters", minMembershipPasswordLength)}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return MembershipResponse{}, err
	}

	newUser := &user.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		FullName:     req.FullName,
		Email:        req.Email,
		RoleID:       targetRole.ID,
		Status:       user.StatusActive,
	}

	now := time.Now().UTC()
	membership := &TenantUser{
		RoleID:    targetRole.ID,
		Status:    TenantUserStatusInvited,
		InvitedBy: &actorUserID,
		InvitedAt: &now,
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.repo.CreateMembershipWithNewUser(ctx, tenantID, newUser, membership, func(membershipID int64) *TenantAuditLog {
		return &TenantAuditLog{
			TenantID: &tenantID, ActorUserID: actorUserID, ActorType: actorType,
			Action: "tenant_user.create", TargetType: strPtr("tenant_user"), TargetID: &membershipID,
			Metadata: auditMetadata(map[string]any{
				"user_id": fmt.Sprint(newUser.ID), "username": req.Username, "role_code": req.RoleCode, "mode": MembershipModeNew,
			}),
		}
	})
	dbctx.LogTransaction(ctx, "tenant_user.create", start, err)
	if err != nil {
		return MembershipResponse{}, err
	}

	row, err := s.repo.FindMembershipRow(ctx, tenantID, newUser.ID)
	if err != nil {
		return MembershipResponse{}, err
	}
	return toMembershipResponse(*row), nil
}

// createWithExistingUser is the "assign an existing user to another
// tenant" flow (WMS-29E Phase 1, capability 3 / Phase 5's "link an
// existing user" flow). Resolves req.Identifier by exact username/email
// match only — see user.Repository.FindByUsernameOrEmail's own doc for
// why this must never become a broad search. The linked user's OTHER
// tenant memberships are never read or exposed here (Phase 5: "do not
// expose that user's other tenant data") — only the one new tenant_users
// row this call creates.
func (s *MembershipService) createWithExistingUser(ctx context.Context, tenantID int64, req CreateMembershipRequest, targetRole *role.Role, actorUserID int64, actorType string) (MembershipResponse, error) {
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		return MembershipResponse{}, ErrValidation{Message: "identifier is required for mode=EXISTING"}
	}

	target, err := s.users.FindByUsernameOrEmail(ctx, identifier)
	if errors.Is(err, user.ErrNotFound) {
		return MembershipResponse{}, ErrLinkUserNotFound
	}
	if err != nil {
		return MembershipResponse{}, err
	}
	if target.Status != user.StatusActive {
		return MembershipResponse{}, ErrValidation{Message: "target user account is not active"}
	}

	now := time.Now().UTC()
	membership := &TenantUser{
		TenantID:  tenantID,
		UserID:    target.ID,
		RoleID:    targetRole.ID,
		Status:    TenantUserStatusInvited,
		InvitedBy: &actorUserID,
		InvitedAt: &now,
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.repo.CreateMembershipWithExistingUser(ctx, membership, func(membershipID int64) *TenantAuditLog {
		return &TenantAuditLog{
			TenantID: &tenantID, ActorUserID: actorUserID, ActorType: actorType,
			Action: "tenant_user.link", TargetType: strPtr("tenant_user"), TargetID: &membershipID,
			Metadata: auditMetadata(map[string]any{
				"user_id": fmt.Sprint(target.ID), "role_code": req.RoleCode, "mode": MembershipModeExisting,
			}),
		}
	})
	dbctx.LogTransaction(ctx, "tenant_user.link", start, err)
	if err != nil {
		return MembershipResponse{}, err
	}

	row, err := s.repo.FindMembershipRow(ctx, tenantID, target.ID)
	if err != nil {
		return MembershipResponse{}, err
	}
	return toMembershipResponse(*row), nil
}

// ChangeRole validates req and transitions (tenantID, userID)'s role,
// refusing if that would leave the tenant with no active administrator
// (guardLastActiveAdmin).
func (s *MembershipService) ChangeRole(ctx context.Context, tenantID, userID int64, req ChangeMembershipRoleRequest, actorUserID int64, actorType string) (MembershipResponse, error) {
	current, err := s.repo.FindMembership(ctx, tenantID, userID)
	if err != nil {
		return MembershipResponse{}, err
	}

	newRole, err := s.repo.FindTenantRoleByCode(ctx, tenantID, req.RoleCode)
	if errors.Is(err, role.ErrNotFound) {
		return MembershipResponse{}, ErrValidation{Message: "role_code does not match any role in this tenant"}
	}
	if err != nil {
		return MembershipResponse{}, err
	}

	if err := s.guardLastActiveAdmin(ctx, tenantID, current, newRole.Code, current.Status); err != nil {
		return MembershipResponse{}, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.repo.UpdateMembershipRole(ctx, tenantID, userID, newRole.ID, &TenantAuditLog{
		TenantID: &tenantID, ActorUserID: actorUserID, ActorType: actorType,
		Action: "tenant_user.role_change", TargetType: strPtr("tenant_user"), TargetID: &current.ID,
		Metadata: auditMetadata(map[string]any{
			"user_id": fmt.Sprint(userID), "from_role_id": fmt.Sprint(current.RoleID), "to_role_code": req.RoleCode,
		}),
	})
	dbctx.LogTransaction(ctx, "tenant_user.role_change", start, err)
	if err != nil {
		return MembershipResponse{}, err
	}

	row, err := s.repo.FindMembershipRow(ctx, tenantID, userID)
	if err != nil {
		return MembershipResponse{}, err
	}
	return toMembershipResponse(*row), nil
}

// ChangeStatus validates req and transitions (tenantID, userID)'s
// status to ACTIVE (reactivate) or DISABLED (suspend), refusing if that
// would leave the tenant with no active administrator
// (guardLastActiveAdmin). Stamps joined_at the first time a membership
// ever becomes ACTIVE (it stays NULL for an INVITED row that is
// suspended before ever being accepted).
func (s *MembershipService) ChangeStatus(ctx context.Context, tenantID, userID int64, req ChangeMembershipStatusRequest, actorUserID int64, actorType string) (MembershipResponse, error) {
	if req.Status != TenantUserStatusActive && req.Status != TenantUserStatusDisabled {
		return MembershipResponse{}, ErrValidation{Message: "status must be ACTIVE or DISABLED"}
	}

	current, err := s.repo.FindMembership(ctx, tenantID, userID)
	if err != nil {
		return MembershipResponse{}, err
	}

	currentRole, err := s.roles.FindRoleByID(ctx, current.RoleID)
	if err != nil {
		return MembershipResponse{}, err
	}

	if err := s.guardLastActiveAdmin(ctx, tenantID, current, currentRole.Code, req.Status); err != nil {
		return MembershipResponse{}, err
	}

	joinedAt := current.JoinedAt
	if req.Status == TenantUserStatusActive && joinedAt == nil {
		now := time.Now().UTC()
		joinedAt = &now
	}

	action := "tenant_user.suspend"
	if req.Status == TenantUserStatusActive {
		action = "tenant_user.reactivate"
	}

	metadata := map[string]any{"user_id": fmt.Sprint(userID), "from_status": current.Status, "to_status": req.Status}
	if req.Reason != nil {
		metadata["reason"] = *req.Reason
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.repo.UpdateMembershipStatus(ctx, tenantID, userID, req.Status, joinedAt, &TenantAuditLog{
		TenantID: &tenantID, ActorUserID: actorUserID, ActorType: actorType,
		Action: action, TargetType: strPtr("tenant_user"), TargetID: &current.ID,
		Metadata: auditMetadata(metadata),
	})
	dbctx.LogTransaction(ctx, "tenant_user.change_status", start, err)
	if err != nil {
		return MembershipResponse{}, err
	}

	row, err := s.repo.FindMembershipRow(ctx, tenantID, userID)
	if err != nil {
		return MembershipResponse{}, err
	}
	return toMembershipResponse(*row), nil
}

// guardLastActiveAdmin blocks a role or status change that would leave
// tenantID with zero ACTIVE memberships holding the ADMIN role. Only
// checks when current is itself an ACTIVE ADMIN membership about to
// lose one of those two properties — promoting someone, demoting a
// non-admin, or suspending a non-admin are all unaffected and never pay
// the extra CountActiveAdmins query.
func (s *MembershipService) guardLastActiveAdmin(ctx context.Context, tenantID int64, current *TenantUser, newRoleCode, newStatus string) error {
	currentRole, err := s.roles.FindRoleByID(ctx, current.RoleID)
	if err != nil {
		return err
	}

	wasActiveAdmin := current.Status == TenantUserStatusActive && currentRole.Code == tenantAdminRoleCode
	staysActiveAdmin := newStatus == TenantUserStatusActive && newRoleCode == tenantAdminRoleCode
	if !wasActiveAdmin || staysActiveAdmin {
		return nil
	}

	count, err := s.repo.CountActiveAdmins(ctx, tenantID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastActiveAdmin
	}
	return nil
}

// EnsureActingTenantAdmin requires that actorUserID itself holds an
// ACTIVE ADMIN membership in tenantID — the Tenant-Admin-scoped
// handler's extra authorization gate on top of RequirePermission
// (tenant_users.*). A permission grant alone is not sufficient here: per
// CLAUDE.md Phase 4 ("do not allow a Tenant Admin to... grant roles
// beyond their authority"), only the tenant's own ADMIN-role holder may
// mutate another member's role or status through the tenant-admin-scoped
// endpoints, regardless of what a broader custom role's permission
// grants might otherwise include (e.g. WAREHOUSE_MANAGER's generic
// "everything except delete" default — see internal/rbac/catalog.go).
func (s *MembershipService) EnsureActingTenantAdmin(ctx context.Context, tenantID, actorUserID int64) error {
	actor, err := s.repo.FindMembership(ctx, tenantID, actorUserID)
	if err != nil {
		if errors.Is(err, ErrMembershipRowNotFound) {
			return ErrNotTenantAdmin
		}
		return err
	}
	if actor.Status != TenantUserStatusActive {
		return ErrNotTenantAdmin
	}

	actorRole, err := s.roles.FindRoleByID(ctx, actor.RoleID)
	if err != nil {
		return err
	}
	if actorRole.Code != tenantAdminRoleCode {
		return ErrNotTenantAdmin
	}
	return nil
}

// EnsureNotSelf rejects a mutation where the target membership's own
// user is the caller — the tenant-admin-scoped handler's self-
// modification guard (CLAUDE.md Phase 4: "do not allow a Tenant Admin
// to... elevate their own permissions"). Platform Admin is not subject
// to this restriction (RequirePlatformAdmin's own authorization does not
// depend on tenant-level RBAC at all, so there is no self-elevation
// channel to close there); only the tenant-admin-scoped handler calls
// this.
func EnsureNotSelf(actorUserID, targetUserID int64) error {
	if actorUserID == targetUserID {
		return ErrCannotModifySelf
	}
	return nil
}
