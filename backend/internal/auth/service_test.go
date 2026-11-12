package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/role"
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

func newTestService() (*Service, *fakeUsers, *fakeRoles) {
	users := &fakeUsers{byUsername: map[string]*user.User{}, byID: map[int64]*user.User{}}
	roles := &fakeRoles{
		byID:        map[int64]*role.Role{1: {ID: 1, Code: "ADMIN", Name: "Administrator"}},
		permissions: map[int64][]string{1: {"users.view", "users.update"}},
	}
	svc := NewService(users, roles, "test-secret", time.Hour)
	return svc, users, roles
}

func TestLogin_Success(t *testing.T) {
	svc, users, _ := newTestService()
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
	svc, users, _ := newTestService()
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
	svc, users, _ := newTestService()
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
	svc, users, _ := newTestService()

	_, err := svc.Login(context.Background(), "nobody", "whatever")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
	if len(users.lastLoginCalls) != 0 {
		t.Fatalf("lastLoginCalls = %v, want none on a failed login", users.lastLoginCalls)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	svc, users, _ := newTestService()
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
	svc, users, _ := newTestService()
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

func TestMe_ReturnsCurrentProfile(t *testing.T) {
	svc, users, _ := newTestService()
	u := newTestUser(t, 1, "admin", "correct-password", user.StatusActive, 1)
	users.byID[1] = u

	summary, err := svc.Me(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Username != "admin" {
		t.Fatalf("username = %q, want admin", summary.Username)
	}
}

func TestMe_UnknownUser(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.Me(context.Background(), 999)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("error = %v, want user.ErrNotFound", err)
	}
}
