package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/role"
	"wms-backend/internal/user"
)

// fakeMembershipRepo is a membershipManagementRepository + roleLookup
// double (one struct plays both narrow interfaces, mirroring how
// production code passes the same *tenant.Repository value for both
// roles) — lets MembershipService's validation/guard/audit-construction
// logic be tested without a real SQL Server connection.
type fakeMembershipRepo struct {
	memberships map[string]*TenantUser // "tenantID:userID" -> row
	users       map[int64]*user.User   // userID -> user, for display joins
	roles       map[int64]*role.Role   // roleID -> role
	nextMembershipID int64

	createErr error
	roleErr   error
	statusErr error

	lastLog *TenantAuditLog
}

func newFakeMembershipRepo() *fakeMembershipRepo {
	return &fakeMembershipRepo{
		memberships: map[string]*TenantUser{},
		users:       map[int64]*user.User{},
		roles:       map[int64]*role.Role{},
		nextMembershipID: 1,
	}
}

func membershipKey(tenantID, userID int64) string {
	return itoaKey(tenantID) + ":" + itoaKey(userID)
}

func itoaKey(v int64) string {
	// Minimal int64->string without pulling in strconv just for a test
	// helper key — fine for the small positive IDs these tests use.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (f *fakeMembershipRepo) seedRole(id, tenantID int64, code, name string) *role.Role {
	r := &role.Role{ID: id, TenantID: tenantID, Code: code, Name: name, IsActive: true}
	f.roles[id] = r
	return r
}

func (f *fakeMembershipRepo) seedUser(id int64, username, email string, status user.Status) *user.User {
	u := &user.User{ID: id, Username: username, Email: email, FullName: username, Status: status}
	f.users[id] = u
	return u
}

func (f *fakeMembershipRepo) seedMembership(tenantID, userID, roleID int64, status string) *TenantUser {
	m := &TenantUser{ID: f.nextMembershipID, TenantID: tenantID, UserID: userID, RoleID: roleID, Status: status}
	f.nextMembershipID++
	f.memberships[membershipKey(tenantID, userID)] = m
	return m
}

func (f *fakeMembershipRepo) FindRoleByID(_ context.Context, id int64) (*role.Role, error) {
	r, ok := f.roles[id]
	if !ok {
		return nil, role.ErrNotFound
	}
	return r, nil
}

func (f *fakeMembershipRepo) FindTenantRoleByCode(_ context.Context, tenantID int64, code string) (*role.Role, error) {
	for _, r := range f.roles {
		if r.TenantID == tenantID && r.Code == code {
			return r, nil
		}
	}
	return nil, role.ErrNotFound
}

func (f *fakeMembershipRepo) ListMemberships(_ context.Context, tenantID int64, _ MembershipListFilter) ([]MembershipRow, int64, error) {
	var rows []MembershipRow
	for _, m := range f.memberships {
		if m.TenantID != tenantID {
			continue
		}
		rows = append(rows, f.toRow(m))
	}
	return rows, int64(len(rows)), nil
}

func (f *fakeMembershipRepo) FindMembershipRow(_ context.Context, tenantID, userID int64) (*MembershipRow, error) {
	m, ok := f.memberships[membershipKey(tenantID, userID)]
	if !ok {
		return nil, ErrMembershipRowNotFound
	}
	row := f.toRow(m)
	return &row, nil
}

func (f *fakeMembershipRepo) toRow(m *TenantUser) MembershipRow {
	u := f.users[m.UserID]
	r := f.roles[m.RoleID]
	row := MembershipRow{
		ID: m.ID, TenantID: m.TenantID, UserID: m.UserID, RoleID: m.RoleID,
		Status: m.Status, InvitedBy: m.InvitedBy, InvitedAt: m.InvitedAt, JoinedAt: m.JoinedAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if u != nil {
		row.Username, row.FullName, row.Email = u.Username, u.FullName, u.Email
	}
	if r != nil {
		row.RoleCode, row.RoleName = r.Code, r.Name
	}
	return row
}

func (f *fakeMembershipRepo) FindMembership(_ context.Context, tenantID, userID int64) (*TenantUser, error) {
	m, ok := f.memberships[membershipKey(tenantID, userID)]
	if !ok {
		return nil, ErrMembershipRowNotFound
	}
	copy := *m
	return &copy, nil
}

func (f *fakeMembershipRepo) CreateMembershipWithNewUser(_ context.Context, tenantID int64, newUser *user.User, membership *TenantUser, buildLog func(int64) *TenantAuditLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	for _, u := range f.users {
		if u.Username == newUser.Username || u.Email == newUser.Email {
			return user.ErrDuplicate
		}
	}
	newUser.ID = int64(len(f.users) + 1000)
	f.users[newUser.ID] = newUser

	membership.TenantID = tenantID
	membership.UserID = newUser.ID
	key := membershipKey(tenantID, newUser.ID)
	if _, exists := f.memberships[key]; exists {
		return ErrMembershipDuplicate
	}
	membership.ID = f.nextMembershipID
	f.nextMembershipID++
	f.memberships[key] = membership

	f.lastLog = buildLog(membership.ID)
	return nil
}

func (f *fakeMembershipRepo) CreateMembershipWithExistingUser(_ context.Context, membership *TenantUser, buildLog func(int64) *TenantAuditLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	key := membershipKey(membership.TenantID, membership.UserID)
	if _, exists := f.memberships[key]; exists {
		return ErrMembershipDuplicate
	}
	membership.ID = f.nextMembershipID
	f.nextMembershipID++
	f.memberships[key] = membership
	f.lastLog = buildLog(membership.ID)
	return nil
}

func (f *fakeMembershipRepo) UpdateMembershipRole(_ context.Context, tenantID, userID, roleID int64, log *TenantAuditLog) error {
	if f.roleErr != nil {
		return f.roleErr
	}
	m, ok := f.memberships[membershipKey(tenantID, userID)]
	if !ok {
		return ErrMembershipRowNotFound
	}
	m.RoleID = roleID
	f.lastLog = log
	return nil
}

func (f *fakeMembershipRepo) UpdateMembershipStatus(_ context.Context, tenantID, userID int64, status string, joinedAt *time.Time, log *TenantAuditLog) error {
	if f.statusErr != nil {
		return f.statusErr
	}
	m, ok := f.memberships[membershipKey(tenantID, userID)]
	if !ok {
		return ErrMembershipRowNotFound
	}
	m.Status = status
	m.JoinedAt = joinedAt
	f.lastLog = log
	return nil
}

func (f *fakeMembershipRepo) CountActiveAdmins(_ context.Context, tenantID int64) (int64, error) {
	var count int64
	for _, m := range f.memberships {
		if m.TenantID != tenantID || m.Status != TenantUserStatusActive {
			continue
		}
		if r, ok := f.roles[m.RoleID]; ok && r.Code == tenantAdminRoleCode {
			count++
		}
	}
	return count, nil
}

// fakeUserLookup is a userLookup double for the "link existing user" flow.
type fakeUserLookup struct {
	byIdentifier map[string]*user.User
}

func (f *fakeUserLookup) FindByUsernameOrEmail(_ context.Context, identifier string) (*user.User, error) {
	u, ok := f.byIdentifier[identifier]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func newMembershipServiceUnderTest(repo *fakeMembershipRepo, users *fakeUserLookup, tenants *fakeManagementRepo) *MembershipService {
	return NewMembershipService(repo, users, repo, tenants)
}

func TestCreateMembership_NewMode_Success(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "RECEIVING", "Receiving Staff")
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	resp, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeNew, RoleCode: "RECEIVING",
		Username: "newstaff", Password: "SuperSecret1", FullName: "New Staff", Email: "new@example.com",
	}, 99, AuditActorPlatformAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != TenantUserStatusInvited {
		t.Errorf("status = %q, want INVITED", resp.Status)
	}
	if resp.Role.Code != "RECEIVING" {
		t.Errorf("role code = %q, want RECEIVING", resp.Role.Code)
	}
	if repo.lastLog == nil || repo.lastLog.Action != "tenant_user.create" {
		t.Errorf("expected tenant_user.create audit log, got %+v", repo.lastLog)
	}
}

func TestCreateMembership_NewMode_RejectsShortPassword(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "RECEIVING", "Receiving Staff")
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeNew, RoleCode: "RECEIVING",
		Username: "newstaff", Password: "short", FullName: "New Staff", Email: "new@example.com",
	}, 99, AuditActorPlatformAdmin)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
}

func TestCreateMembership_UnknownRoleCode_Rejected(t *testing.T) {
	repo := newFakeMembershipRepo()
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeNew, RoleCode: "NOPE",
		Username: "newstaff", Password: "SuperSecret1", FullName: "New Staff", Email: "new@example.com",
	}, 99, AuditActorPlatformAdmin)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
}

func TestCreateMembership_ExistingMode_Success(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "RECEIVING", "Receiving Staff")
	existing := repo.seedUser(42, "existing", "existing@example.com", user.StatusActive)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	users := &fakeUserLookup{byIdentifier: map[string]*user.User{"existing@example.com": existing}}
	svc := newMembershipServiceUnderTest(repo, users, tenants)

	resp, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeExisting, RoleCode: "RECEIVING", Identifier: "existing@example.com",
	}, 99, AuditActorPlatformAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UserID != 42 {
		t.Errorf("user_id = %d, want 42", resp.UserID)
	}
	if repo.lastLog == nil || repo.lastLog.Action != "tenant_user.link" {
		t.Errorf("expected tenant_user.link audit log, got %+v", repo.lastLog)
	}
}

func TestCreateMembership_ExistingMode_UserNotFound(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "RECEIVING", "Receiving Staff")
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{byIdentifier: map[string]*user.User{}}, tenants)

	_, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeExisting, RoleCode: "RECEIVING", Identifier: "nobody@example.com",
	}, 99, AuditActorPlatformAdmin)
	if !errors.Is(err, ErrLinkUserNotFound) {
		t.Fatalf("got %v, want ErrLinkUserNotFound", err)
	}
}

func TestCreateMembership_DuplicateMembership_Rejected(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "RECEIVING", "Receiving Staff")
	existing := repo.seedUser(42, "existing", "existing@example.com", user.StatusActive)
	repo.seedMembership(1, 42, 1, TenantUserStatusActive)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	users := &fakeUserLookup{byIdentifier: map[string]*user.User{"existing@example.com": existing}}
	svc := newMembershipServiceUnderTest(repo, users, tenants)

	_, err := svc.CreateMembership(context.Background(), 1, CreateMembershipRequest{
		Mode: MembershipModeExisting, RoleCode: "RECEIVING", Identifier: "existing@example.com",
	}, 99, AuditActorPlatformAdmin)
	if !errors.Is(err, ErrMembershipDuplicate) {
		t.Fatalf("got %v, want ErrMembershipDuplicate", err)
	}
}

func TestChangeRole_LastActiveAdmin_Blocked(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "ADMIN", "Administrator")
	repo.seedRole(2, 1, "RECEIVING", "Receiving Staff")
	repo.seedUser(1, "admin", "admin@example.com", user.StatusActive)
	repo.seedMembership(1, 1, 1, TenantUserStatusActive)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, err := svc.ChangeRole(context.Background(), 1, 1, ChangeMembershipRoleRequest{RoleCode: "RECEIVING"}, 1, AuditActorTenantAdmin)
	if !errors.Is(err, ErrLastActiveAdmin) {
		t.Fatalf("got %v, want ErrLastActiveAdmin", err)
	}
}

func TestChangeRole_SecondActiveAdmin_Allowed(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "ADMIN", "Administrator")
	repo.seedRole(2, 1, "RECEIVING", "Receiving Staff")
	repo.seedUser(1, "admin1", "admin1@example.com", user.StatusActive)
	repo.seedUser(2, "admin2", "admin2@example.com", user.StatusActive)
	repo.seedMembership(1, 1, 1, TenantUserStatusActive)
	repo.seedMembership(1, 2, 1, TenantUserStatusActive) // a second ACTIVE ADMIN
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	resp, err := svc.ChangeRole(context.Background(), 1, 1, ChangeMembershipRoleRequest{RoleCode: "RECEIVING"}, 2, AuditActorTenantAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Role.Code != "RECEIVING" {
		t.Errorf("role code = %q, want RECEIVING", resp.Role.Code)
	}
}

func TestChangeStatus_LastActiveAdmin_Blocked(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "ADMIN", "Administrator")
	repo.seedUser(1, "admin", "admin@example.com", user.StatusActive)
	repo.seedMembership(1, 1, 1, TenantUserStatusActive)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, err := svc.ChangeStatus(context.Background(), 1, 1, ChangeMembershipStatusRequest{Status: TenantUserStatusDisabled}, 1, AuditActorTenantAdmin)
	if !errors.Is(err, ErrLastActiveAdmin) {
		t.Fatalf("got %v, want ErrLastActiveAdmin", err)
	}
}

func TestChangeStatus_InvalidStatus_Rejected(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(2, 1, "RECEIVING", "Receiving Staff")
	repo.seedUser(5, "staff", "staff@example.com", user.StatusActive)
	repo.seedMembership(1, 5, 2, TenantUserStatusActive)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, err := svc.ChangeStatus(context.Background(), 1, 5, ChangeMembershipStatusRequest{Status: "INVITED"}, 1, AuditActorTenantAdmin)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
}

func TestChangeStatus_Reactivate_StampsJoinedAt(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(2, 1, "RECEIVING", "Receiving Staff")
	repo.seedUser(5, "staff", "staff@example.com", user.StatusActive)
	repo.seedMembership(1, 5, 2, TenantUserStatusDisabled)
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	resp, err := svc.ChangeStatus(context.Background(), 1, 5, ChangeMembershipStatusRequest{Status: TenantUserStatusActive}, 1, AuditActorTenantAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.JoinedAt == nil {
		t.Error("expected joined_at to be stamped on first activation")
	}
}

func TestEnsureActingTenantAdmin(t *testing.T) {
	repo := newFakeMembershipRepo()
	repo.seedRole(1, 1, "ADMIN", "Administrator")
	repo.seedRole(2, 1, "RECEIVING", "Receiving Staff")
	repo.seedUser(1, "admin", "admin@example.com", user.StatusActive)
	repo.seedUser(2, "staff", "staff@example.com", user.StatusActive)
	repo.seedMembership(1, 1, 1, TenantUserStatusActive)   // ADMIN, ACTIVE
	repo.seedMembership(1, 2, 2, TenantUserStatusActive)   // RECEIVING, ACTIVE
	tenants := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme"})
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	if err := svc.EnsureActingTenantAdmin(context.Background(), 1, 1); err != nil {
		t.Errorf("ADMIN actor: got %v, want nil", err)
	}
	if err := svc.EnsureActingTenantAdmin(context.Background(), 1, 2); !errors.Is(err, ErrNotTenantAdmin) {
		t.Errorf("non-ADMIN actor: got %v, want ErrNotTenantAdmin", err)
	}
	if err := svc.EnsureActingTenantAdmin(context.Background(), 1, 999); !errors.Is(err, ErrNotTenantAdmin) {
		t.Errorf("non-member actor: got %v, want ErrNotTenantAdmin", err)
	}
}

func TestEnsureNotSelf(t *testing.T) {
	if err := EnsureNotSelf(5, 5); !errors.Is(err, ErrCannotModifySelf) {
		t.Errorf("same id: got %v, want ErrCannotModifySelf", err)
	}
	if err := EnsureNotSelf(5, 6); err != nil {
		t.Errorf("different id: got %v, want nil", err)
	}
}

func TestListMemberships_UnknownTenant_NotFound(t *testing.T) {
	repo := newFakeMembershipRepo()
	tenants := newFakeManagementRepo()
	svc := newMembershipServiceUnderTest(repo, &fakeUserLookup{}, tenants)

	_, _, err := svc.ListMemberships(context.Background(), 999, MembershipListParams{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
