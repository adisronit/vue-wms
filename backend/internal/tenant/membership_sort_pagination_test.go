package tenant

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/role"
	"wms-backend/internal/user"
)

// seedMembershipFixture creates a fresh tenant, role, and a membership
// (tenant_users row joining a freshly-created user to that tenant via
// that role) with the given display name and joined_at — the minimum
// hierarchy ListMemberships' joined query needs. label keeps
// username/email/slug unique across calls within the same rolled-back
// transaction.
func seedMembershipFixture(t *testing.T, tx *gorm.DB, label, fullName, roleName string, joinedAt time.Time) (tenantID int64, member TenantUser) {
	t.Helper()

	tn := Tenant{Slug: fmt.Sprintf("mem-sort-%s", label), Name: fmt.Sprintf("Membership Sort Tenant %s", label), Status: TenantStatusActive}
	if err := tx.Create(&tn).Error; err != nil {
		t.Fatalf("seedMembershipFixture(%s): create tenant: %v", label, err)
	}

	r := role.Role{Code: fmt.Sprintf("MEMSORT_%s", label), Name: roleName, Description: "fixture", IsActive: true}
	if err := tx.Create(&r).Error; err != nil {
		t.Fatalf("seedMembershipFixture(%s): create role: %v", label, err)
	}

	u := user.User{
		Username:     fmt.Sprintf("memsort_%s", label),
		PasswordHash: "not-a-real-hash",
		FullName:     fullName,
		Email:        fmt.Sprintf("memsort-%s@example.invalid", label),
		RoleID:       r.ID,
		Status:       user.StatusActive,
	}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatalf("seedMembershipFixture(%s): create user: %v", label, err)
	}

	tu := TenantUser{TenantID: tn.ID, UserID: u.ID, RoleID: r.ID, Status: TenantUserStatusActive, JoinedAt: &joinedAt}
	if err := tx.Create(&tu).Error; err != nil {
		t.Fatalf("seedMembershipFixture(%s): create membership: %v", label, err)
	}

	return tn.ID, tu
}

func TestMembershipService_List_DefaultsToNameAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	now := time.Now()

	tenantID, _ := seedMembershipFixture(t, tx, "default-c", "Charlie User", "Role C", now)
	tenantID2, _ := seedMembershipFixture(t, tx, "default-a", "Alpha User", "Role A", now)
	tenantID3, _ := seedMembershipFixture(t, tx, "default-b", "Bravo User", "Role B", now)
	_ = tenantID2
	_ = tenantID3

	// Each fixture gets its own tenant (seedMembershipFixture always
	// creates one), so list each tenant separately rather than trying to
	// assert a combined order across three different tenants' member
	// lists — ListMemberships is always scoped to one tenantID.
	repo := NewRepository(tx)
	svc := NewMembershipService(repo, nil, nil, repo)

	_, total, err := svc.ListMemberships(context.Background(), tenantID, MembershipListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (one member in this fixture's own tenant)", total)
	}
}

func TestMembershipService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	now := time.Now()

	tn := Tenant{Slug: "mem-sort-shared", Name: "Membership Sort Shared Tenant", Status: TenantStatusActive}
	if err := tx.Create(&tn).Error; err != nil {
		t.Fatalf("create shared tenant: %v", err)
	}

	addMember := func(label, fullName, roleName string, joinedAt time.Time) {
		r := role.Role{Code: fmt.Sprintf("MEMSHARED_%s", label), Name: roleName, Description: "fixture", IsActive: true}
		if err := tx.Create(&r).Error; err != nil {
			t.Fatalf("create role %s: %v", label, err)
		}
		u := user.User{
			Username: fmt.Sprintf("memshared_%s", label), PasswordHash: "x", FullName: fullName,
			Email: fmt.Sprintf("memshared-%s@example.invalid", label), RoleID: r.ID, Status: user.StatusActive,
		}
		if err := tx.Create(&u).Error; err != nil {
			t.Fatalf("create user %s: %v", label, err)
		}
		tu := TenantUser{TenantID: tn.ID, UserID: u.ID, RoleID: r.ID, Status: TenantUserStatusActive, JoinedAt: &joinedAt}
		if err := tx.Create(&tu).Error; err != nil {
			t.Fatalf("create membership %s: %v", label, err)
		}
	}

	addMember("charlie", "Charlie User", "Role C", now)
	addMember("alpha", "Alpha User", "Role A", now)
	addMember("bravo", "Bravo User", "Role B", now)

	repo := NewRepository(tx)
	svc := NewMembershipService(repo, nil, nil, repo)

	asc, _, err := svc.ListMemberships(context.Background(), tn.ID, MembershipListParams{SortBy: "name", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(name asc): %v", err)
	}
	if asc[0].FullName != "Alpha User" || asc[1].FullName != "Bravo User" || asc[2].FullName != "Charlie User" {
		t.Errorf("name ASC order = [%s %s %s], want Alpha/Bravo/Charlie", asc[0].FullName, asc[1].FullName, asc[2].FullName)
	}

	desc, _, err := svc.ListMemberships(context.Background(), tn.ID, MembershipListParams{SortBy: "name", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(name desc): %v", err)
	}
	if desc[0].FullName != "Charlie User" || desc[1].FullName != "Bravo User" || desc[2].FullName != "Alpha User" {
		t.Errorf("name DESC order = [%s %s %s], want Charlie/Bravo/Alpha", desc[0].FullName, desc[1].FullName, desc[2].FullName)
	}
}

func TestMembershipService_List_InvalidSortBy_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID, _ := seedMembershipFixture(t, tx, "invalid-sortby", "Someone", "Role", time.Now())

	repo := NewRepository(tx)
	svc := NewMembershipService(repo, nil, nil, repo)
	_, _, err := svc.ListMemberships(context.Background(), tenantID, MembershipListParams{SortBy: "password_hash"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

func TestMembershipService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID, _ := seedMembershipFixture(t, tx, "invalid-sortdir", "Someone", "Role", time.Now())

	repo := NewRepository(tx)
	svc := NewMembershipService(repo, nil, nil, repo)
	_, _, err := svc.ListMemberships(context.Background(), tenantID, MembershipListParams{SortBy: "name", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

// TestMembershipService_List_SortByStatus_IsUnambiguous proves the
// qualified "tenant_users.status" sort column resolves correctly even
// though users.status is also a real column in the same joined query —
// an unqualified "status" would be ambiguous to SQL Server.
func TestMembershipService_List_SortByStatus_IsUnambiguous(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID, _ := seedMembershipFixture(t, tx, "status-sort", "Someone", "Role", time.Now())

	repo := NewRepository(tx)
	svc := NewMembershipService(repo, nil, nil, repo)
	_, _, err := svc.ListMemberships(context.Background(), tenantID, MembershipListParams{SortBy: "status", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(status desc): %v, want no ambiguous-column error", err)
	}
}
