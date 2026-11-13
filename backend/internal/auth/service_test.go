package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/role"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

type fakeUsers struct {
	byUsername         map[string]*user.User
	byID               map[int64]*user.User
	updateLastLoginErr error
	lastLoginCalls     []int64
}

func (f *fakeUsers) FindByUsername(_ context.Context, username string) (*user.User, error) {
	u, ok := f.byUsername[username]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) FindByID(_ context.Context, id int64) (*user.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) UpdateLastLogin(_ context.Context, id int64) error {
	f.lastLoginCalls = append(f.lastLoginCalls, id)
	return f.updateLastLoginErr
}

type fakeRoles struct {
	byID        map[int64]*role.Role
	permissions map[int64][]string
}

func (f *fakeRoles) FindRoleByID(_ context.Context, id int64) (*role.Role, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, role.ErrNotFound
	}
	return r, nil
}

func (f *fakeRoles) RolePermissionCodes(_ context.Context, roleID int64) ([]string, error) {
	return f.permissions[roleID], nil
}

// fakeTenants is a minimal tenantResolver double: memberships maps a
// userID to the ACTIVE, operational-tenant memberships that userID holds,
// ordered the same way tenant.Repository.ActiveMemberships would (tenant
// id ascending) so DefaultMembership's "pick the first one" behavior is
// exercised the same way it would be against a real database.
type fakeTenants struct {
	memberships map[int64][]tenant.Membership
	err         error
}

func (f *fakeTenants) DefaultMembership(_ context.Context, userID int64) (*tenant.Membership, error) {
	if f.err != nil {
		return nil, f.err
	}
	ms := f.memberships[userID]
	if len(ms) == 0 {
		return nil, tenant.ErrMembershipNotFound
	}
	m := ms[0]
	return &m, nil
}

func (f *fakeTenants) ActiveMembership(_ context.Context, userID, tenantID int64) (*tenant.Membership, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, m := range f.memberships[userID] {
		if m.TenantID == tenantID {
			found := m
			return &found, nil
		}
	}
	return nil, tenant.ErrMembershipNotFound
}

func newTestUser(t *testing.T, id int64, username, password string, status user.Status, roleID int64) *user.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	return &user.User{
		ID: id, Username: username, PasswordHash: string(hash),
		FullName: "Test User", Email: username + "@example.com",
		RoleID: roleID, Status: status,
	}
}

// newTestService wires a Service against fakes with one default fixture:
// user id 1 holds an ACTIVE membership in tenant 100 ("Acme Co") with role
// 1 (ADMIN) — the single-tenant shape every pre-existing Login/Me test
// below exercises. Tests that need a different membership shape (none,
// multiple, a different tenant) overwrite tenants.memberships directly.
func newTestService() (*Service, *fakeUsers, *fakeRoles, *fakeTenants) {
	users := &fakeUsers{byUsername: map[string]*user.User{}, byID: map[int64]*user.User{}}
	roles := &fakeRoles{
		byID:        map[int64]*role.Role{1: {ID: 1, Code: "ADMIN", Name: "Administrator"}},
		permissions: map[int64][]string{1: {"users.view", "users.update"}},
	}
	tenants := &fakeTenants{memberships: map[int64][]tenant.Membership{
		1: {{TenantID: 100, TenantSlug: "acme", TenantName: "Acme Co", RoleID: 1}},
	}}
	svc := NewService(users, roles, tenants, "test-secret", time.Hour)
	return svc, users, roles, tenants
}

func TestLogin_Success(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byUsername["admin"] = u
	users.byID[1] = u

	data, err := svc.Login(context.Background(), "admin", "correct-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.AccessToken == "" {
		t.Fatal("expected a non-empty access token")
	}
	if data.User.Role.Code != "ADMIN" {
		t.Fatalf("role code = %q, want ADMIN", data.User.Role.Code)
	}
	if data.User.Tenant.ID != 100 || data.User.Tenant.Slug != "acme" {
		t.Fatalf("tenant = %+v, want tenant 100 (acme)", data.User.Tenant)
	}
	if len(data.User.Permissions) != 2 {
		t.Fatalf("permissions = %v, want 2 entries", data.User.Permissions)
	}
	if len(users.lastLoginCalls) != 1 || users.lastLoginCalls[0] != u.ID {
		t.Fatalf("lastLoginCalls = %v, want exactly one call for user %d", users.lastLoginCalls, u.ID)
	}
}

// TestLogin_UpdateLastLoginFailureDoesNotFailLogin covers CLAUDE.md's own
// "do not make updating last_login_at a reason for a successful login to
// fail" requirement: a broken UpdateLastLogin must not turn an otherwise
// successful login into an error response.
func TestLogin_UpdateLastLoginFailureDoesNotFailLogin(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byUsername["admin"] = u
	users.updateLastLoginErr = errors.New("connection reset")

	data, err := svc.Login(context.Background(), "admin", "correct-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.AccessToken == "" {
		t.Fatal("expected a non-empty access token despite the last-login update failure")
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byUsername["admin"] = u

	_, err := svc.Login(context.Background(), "admin", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none on a failed login", users.lastLoginCalls)
	}
}

func TestLogin_UnknownUsername(t *testing.T) {
	svc, users, _, _ := newTestService()

	_, err := svc.Login(context.Background(), "nobody", "whatever")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none on a failed login", users.lastLoginCalls)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusInactive, 1)
	users.byUsername["admin"] = u

	_, err := svc.Login(context.Background(), "admin", "correct-password")
	var notActive ErrAccountNotActive
	if !errors.As(err, &notActive) {
		t.Fatalf("error = %v, want ErrAccountNotActive", err)
	}
	if notActive.Status != user.StatusInactive {
		t.Fatalf("status = %q, want INACTIVE", notActive.Status)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none on a failed login", users.lastLoginCalls)
	}
}

func TestLogin_LockedUser(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusLocked, 1)
	users.byUsername["admin"] = u

	_, err := svc.Login(context.Background(), "admin", "correct-password")
	var notActive ErrAccountNotActive
	if !errors.As(err, &notActive) {
		t.Fatalf("error = %v, want ErrAccountNotActive", err)
	}
	if notActive.Status != user.StatusLocked {
		t.Fatalf("status = %q, want LOCKED", notActive.Status)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none on a failed login", users.lastLoginCalls)
	}
}

// TestLogin_NoActiveTenantMembership_Fails is Phase 29B's core fail-closed
// requirement: credentials being correct and the account being ACTIVE is
// not enough — without an active tenant membership, login must fail
// rather than issue a token with no real tenant context.
func TestLogin_NoActiveTenantMembership_Fails(t *testing.T) {
	svc, users, _, tenants := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byUsername["admin"] = u
	tenants.memberships = map[int64][]tenant.Membership{}

	_, err := svc.Login(context.Background(), "admin", "correct-password")
	if !errors.Is(err, tenant.ErrMembershipNotFound) {
		t.Fatalf("error = %v, want tenant.ErrMembershipNotFound", err)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none when login fails before issuing a token", users.lastLoginCalls)
	}
}

// TestLogin_MultipleMemberships_PicksLowestTenantID documents
// DefaultMembership's deterministic tie-break: with no "default tenant"
// flag in the schema (migrations/0042) and no tenant-picker UI in the
// existing frontend, login auto-selects the lowest tenant_id.
func TestLogin_MultipleMemberships_PicksLowestTenantID(t *testing.T) {
	svc, users, _, tenants := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byUsername["admin"] = u
	// Ordered ascending by tenant_id, exactly as tenant.Repository's own
	// "ORDER BY tenant_users.tenant_id ASC" would return them — fakeTenants
	// mirrors the repository's contract rather than re-sorting, so this
	// fixture's order is itself part of what the test verifies.
	tenants.memberships[1] = []tenant.Membership{
		{TenantID: 100, TenantSlug: "acme", TenantName: "Acme Co", RoleID: 1},
		{TenantID: 200, TenantSlug: "later", TenantName: "Later Co", RoleID: 1},
	}

	data, err := svc.Login(context.Background(), "admin", "correct-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.User.Tenant.ID != 100 {
		t.Fatalf("tenant id = %d, want 100 (the lowest of the caller's memberships)", data.User.Tenant.ID)
	}
}

func TestMe_ReturnsCurrentProfile(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byID[1] = u

	summary, err := svc.Me(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Username != "admin" {
		t.Fatalf("username = %q, want admin", summary.Username)
	}
	if summary.Tenant.ID != 100 {
		t.Fatalf("tenant id = %d, want 100", summary.Tenant.ID)
	}
}

func TestMe_UnknownUser(t *testing.T) {
	svc, _, _, _ := newTestService()

	_, err := svc.Me(context.Background(), 999, 100)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("error = %v, want user.ErrNotFound", err)
	}
}

// TestMe_NoActiveMembership_Fails covers the re-verification Me performs
// on every call (tenant-isolation-security.md's "re-check against the
// database" principle applied to the one route that already hits the
// database on every call by design) — a membership revoked after login
// must make /auth/me fail too, not keep serving the token-time snapshot.
func TestMe_NoActiveMembership_Fails(t *testing.T) {
	svc, users, _, tenants := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byID[1] = u
	tenants.memberships = map[int64][]tenant.Membership{}

	_, err := svc.Me(context.Background(), 1, 100)
	if !errors.Is(err, tenant.ErrMembershipNotFound) {
		t.Fatalf("error = %v, want tenant.ErrMembershipNotFound", err)
	}
}

func TestSwitchTenant_Success(t *testing.T) {
	svc, users, _, tenants := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byID[1] = u
	tenants.memberships[1] = append(tenants.memberships[1],
		tenant.Membership{TenantID: 200, TenantSlug: "other", TenantName: "Other Co", RoleID: 1})

	data, err := svc.SwitchTenant(context.Background(), 1, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.AccessToken == "" {
		t.Fatal("expected a non-empty access token")
	}
	if data.User.Tenant.ID != 200 || data.User.Tenant.Slug != "other" {
		t.Fatalf("tenant = %+v, want tenant 200 (other)", data.User.Tenant)
	}
}

func TestSwitchTenant_DeniedForMissingMembership(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byID[1] = u

	_, err := svc.SwitchTenant(context.Background(), 1, 999)
	if !errors.Is(err, tenant.ErrMembershipNotFound) {
		t.Fatalf("error = %v, want tenant.ErrMembershipNotFound", err)
	}
}

func TestSwitchTenant_DeniedForInactiveAccount(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusLocked, 1)
	users.byID[1] = u

	_, err := svc.SwitchTenant(context.Background(), 1, 100)
	var notActive ErrAccountNotActive
	if !errors.As(err, &notActive) {
		t.Fatalf("error = %v, want ErrAccountNotActive", err)
	}
	if notActive.Status != user.StatusLocked {
		t.Fatalf("status = %q, want LOCKED", notActive.Status)
	}
}

func TestSwitchTenant_UnknownUser(t *testing.T) {
	svc, _, _, _ := newTestService()

	_, err := svc.SwitchTenant(context.Background(), 999, 100)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("error = %v, want user.ErrNotFound", err)
	}
}
