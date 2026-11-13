package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/role"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// ErrInvalidCredentials covers both "no such user" and "wrong password" —
// deliberately the same error for both, so the handler can't be used to
// enumerate valid usernames.
var ErrInvalidCredentials = errors.New("auth: invalid username or password")

// ErrAccountNotActive is returned separately from ErrInvalidCredentials
// because the credentials themselves were correct; the caller uses this
// to return a distinct, more useful error (403 ACCOUNT_INACTIVE/ACCOUNT_LOCKED)
// once identity is already confirmed.
type ErrAccountNotActive struct {
	Status user.Status
}

func (e ErrAccountNotActive) Error() string {
	return "auth: account is " + string(e.Status)
}

// userFinder and roleReader are the narrow slices of user.Repository and
// role.Repository Service actually needs, so unit tests can supply a fake
// without a real SQL Server connection.
type userFinder interface {
	FindByUsername(ctx context.Context, username string) (*user.User, error)
	FindByID(ctx context.Context, id int64) (*user.User, error)
	UpdateLastLogin(ctx context.Context, id int64) error
}

type roleReader interface {
	FindRoleByID(ctx context.Context, id int64) (*role.Role, error)
	RolePermissionCodes(ctx context.Context, roleID int64) ([]string, error)
}

// tenantResolver is the narrow slice of tenant.Service Service needs to
// resolve and re-verify active tenant membership — never trusting a
// client-supplied tenant_id or a JWT's own embedded claim on its own (see
// docs/proposals/wms-saas/tenant-isolation-security.md §2).
type tenantResolver interface {
	DefaultMembership(ctx context.Context, userID int64) (*tenant.Membership, error)
	ActiveMembership(ctx context.Context, userID, tenantID int64) (*tenant.Membership, error)
}

// Service implements login, tenant switching, and "who am I" against the
// user/role/tenant repositories, plus JWT issuance.
type Service struct {
	users     userFinder
	roles     roleReader
	tenants   tenantResolver
	jwtSecret string
	expiresIn time.Duration
}

func NewService(users userFinder, roles roleReader, tenants tenantResolver, jwtSecret string, expiresIn time.Duration) *Service {
	return &Service{users: users, roles: roles, tenants: tenants, jwtSecret: jwtSecret, expiresIn: expiresIn}
}

// Login verifies credentials with bcrypt, resolves the caller's default
// active tenant membership (fail-closed: no membership means no login —
// see tenant.Service.DefaultMembership), and on success issues a JWT
// embedding that membership's role code and permission codes.
func (s *Service) Login(ctx context.Context, username, password string) (LoginData, error) {
	u, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, user.ErrNotFound) {
		return LoginData{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginData{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return LoginData{}, ErrInvalidCredentials
	}

	if u.Status != user.StatusActive {
		return LoginData{}, ErrAccountNotActive{Status: u.Status}
	}

	membership, err := s.tenants.DefaultMembership(ctx, u.ID)
	if err != nil {
		return LoginData{}, err
	}

	summary, permissions, err := s.buildSummary(ctx, u, membership)
	if err != nil {
		return LoginData{}, err
	}

	token, expiresAt, err := IssueToken(s.jwtSecret, s.expiresIn, u.ID, u.Username, membership.TenantID, summary.Role.Code, permissions)
	if err != nil {
		return LoginData{}, err
	}

	// Best-effort: login has already succeeded (credentials verified,
	// account ACTIVE, tenant membership resolved, token issued) by this
	// point, so a failure updating last_login_at must never turn a
	// successful login into an error response — it's logged and
	// swallowed instead. Deliberately after token issuance, not before:
	// only a login that actually returns a usable token counts as
	// "successful" for this timestamp.
	if err := s.users.UpdateLastLogin(ctx, u.ID); err != nil {
		slog.Error("failed to update last_login_at",
			slog.Int64("user_id", u.ID), slog.String("error", err.Error()))
	}

	return LoginData{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(expiresAt).Seconds()),
		User:        summary,
	}, nil
}

// SwitchTenant re-verifies, server-side, that userID holds an ACTIVE
// membership in tenantID before issuing a *new* token scoped to it — the
// client-supplied tenantID is never trusted as the basis for the new
// token's active_tenant_id claim by itself (tenant-isolation-security.md
// §2's switch-tenant design). The account's own status is re-checked too,
// since it may have changed since the session's original login.
func (s *Service) SwitchTenant(ctx context.Context, userID, tenantID int64) (LoginData, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return LoginData{}, err
	}

	if u.Status != user.StatusActive {
		return LoginData{}, ErrAccountNotActive{Status: u.Status}
	}

	membership, err := s.tenants.ActiveMembership(ctx, userID, tenantID)
	if err != nil {
		return LoginData{}, err
	}

	summary, permissions, err := s.buildSummary(ctx, u, membership)
	if err != nil {
		return LoginData{}, err
	}

	token, expiresAt, err := IssueToken(s.jwtSecret, s.expiresIn, u.ID, u.Username, membership.TenantID, summary.Role.Code, permissions)
	if err != nil {
		return LoginData{}, err
	}

	return LoginData{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(expiresAt).Seconds()),
		User:        summary,
	}, nil
}

// Me loads the current profile for userID — used by GET /auth/me so the
// response always reflects the live database row (name/email/status,
// and tenant membership/role), not just what was embedded in the token at
// login time. tenantID is the caller's active tenant (read from the JWT's
// active_tenant_id claim by the handler) — Me re-verifies that membership
// against the database exactly as SwitchTenant does, rather than trusting
// the claim on its own.
func (s *Service) Me(ctx context.Context, userID, tenantID int64) (UserSummary, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return UserSummary{}, err
	}

	membership, err := s.tenants.ActiveMembership(ctx, userID, tenantID)
	if err != nil {
		return UserSummary{}, err
	}

	summary, _, err := s.buildSummary(ctx, u, membership)
	return summary, err
}

// buildSummary resolves membership's role and permission codes and
// assembles the UserSummary every success path (Login/SwitchTenant/Me)
// returns. Role/permissions come from the tenant membership's role_id, not
// u.RoleID — the RBAC catalog stays global (internal/rbac), but which role
// in that catalog applies is per-tenant-membership, not per-user.
func (s *Service) buildSummary(ctx context.Context, u *user.User, m *tenant.Membership) (UserSummary, []string, error) {
	r, err := s.roles.FindRoleByID(ctx, m.RoleID)
	if err != nil {
		return UserSummary{}, nil, err
	}

	permissions, err := s.roles.RolePermissionCodes(ctx, m.RoleID)
	if err != nil {
		return UserSummary{}, nil, err
	}
	if permissions == nil {
		permissions = []string{}
	}

	summary := UserSummary{
		ID:       u.ID,
		Username: u.Username,
		FullName: u.FullName,
		Email:    u.Email,
		Tenant: TenantSummary{
			ID:   m.TenantID,
			Slug: m.TenantSlug,
			Name: m.TenantName,
		},
		Role: RoleSummary{
			Code: r.Code,
			Name: r.Name,
		},
		Permissions: permissions,
	}

	return summary, permissions, nil
}
