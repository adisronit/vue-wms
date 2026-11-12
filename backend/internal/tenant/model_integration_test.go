package tenant

import (
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/role"
	"wms-backend/internal/user"
)

// These are integration tests: they exercise real SQL Server
// constraints (UNIQUE, FOREIGN KEY, CHECK) created by
// migrations/0041 and 0042, not GORM behavior in isolation. They skip
// cleanly (via testsupport.ConnectDB) when no database is configured,
// and run inside a transaction that is always rolled back afterward
// (testsupport.BeginRollbackTx), so they never leave data behind.

func TestTenant_UniqueSlug(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	first := Tenant{Slug: "acme", Name: "Acme Co", Status: TenantStatusTrial}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first tenant: %v", err)
	}

	second := Tenant{Slug: "acme", Name: "Acme Co Duplicate", Status: TenantStatusTrial}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected UQ_tenants_slug violation inserting a second tenant with slug %q, got no error", second.Slug)
	}
}

func TestTenant_StatusCheckConstraintRejectsUnknownValue(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	bad := Tenant{Slug: "bogus-status", Name: "Bogus Status Co", Status: "NOT_A_REAL_STATUS"}
	if err := tx.Create(&bad).Error; err == nil {
		t.Fatalf("expected CK_tenants_status violation for status %q, got no error", bad.Status)
	}
}

func TestTenant_StatusCheckConstraintAllowsEveryDocumentedValue(t *testing.T) {
	db := testsupport.ConnectDB(t)

	for _, status := range []string{TenantStatusTrial, TenantStatusActive, TenantStatusSuspended, TenantStatusCancelled} {
		status := status
		t.Run(status, func(t *testing.T) {
			tx := testsupport.BeginRollbackTx(t, db)
			tenant := Tenant{Slug: "status-" + status, Name: "Status Check Co", Status: status}
			if err := tx.Create(&tenant).Error; err != nil {
				t.Fatalf("expected status %q to be accepted by CK_tenants_status, got error: %v", status, err)
			}
		})
	}
}

// seedRoleAndUser creates one role and one user row inside tx, for
// tests that need a valid foreign-key target. Reused across this
// file's tenant_users tests; codes/usernames are fixed strings because
// every caller runs in its own rolled-back transaction, so there is no
// cross-test collision risk.
func seedRoleAndUser(t *testing.T, tx *gorm.DB) (roleID int64, userID int64) {
	t.Helper()

	r := role.Role{Code: "PHASE28_TEST_ROLE", Name: "Phase 28 Test Role", Description: "fixture", IsActive: true}
	if err := tx.Create(&r).Error; err != nil {
		t.Fatalf("seed fixture role: %v", err)
	}

	u := user.User{
		Username:     "phase28_fixture_user",
		PasswordHash: "not-a-real-hash",
		FullName:     "Phase 28 Fixture User",
		Email:        "phase28-fixture@example.invalid",
		RoleID:       r.ID,
		Status:       user.StatusActive,
	}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatalf("seed fixture user: %v", err)
	}

	return r.ID, u.ID
}

func TestTenantUser_DuplicateMembershipRejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	roleID, userID := seedRoleAndUser(t, tx)

	tenantA := Tenant{Slug: "dup-member-co", Name: "Dup Member Co", Status: TenantStatusActive}
	if err := tx.Create(&tenantA).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	first := TenantUser{TenantID: tenantA.ID, UserID: userID, RoleID: roleID, Status: TenantUserStatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first membership: %v", err)
	}

	second := TenantUser{TenantID: tenantA.ID, UserID: userID, RoleID: roleID, Status: TenantUserStatusActive}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected UQ_tenant_users_tenant_user violation inserting a duplicate (tenant_id, user_id) pair, got no error")
	}
}

func TestTenantUser_OneUserMultipleTenants(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	roleID, userID := seedRoleAndUser(t, tx)

	tenantA := Tenant{Slug: "multi-tenant-a", Name: "Multi Tenant A", Status: TenantStatusActive}
	tenantB := Tenant{Slug: "multi-tenant-b", Name: "Multi Tenant B", Status: TenantStatusActive}
	if err := tx.Create(&tenantA).Error; err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	if err := tx.Create(&tenantB).Error; err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	membershipA := TenantUser{TenantID: tenantA.ID, UserID: userID, RoleID: roleID, Status: TenantUserStatusActive}
	membershipB := TenantUser{TenantID: tenantB.ID, UserID: userID, RoleID: roleID, Status: TenantUserStatusActive}
	if err := tx.Create(&membershipA).Error; err != nil {
		t.Fatalf("create membership in tenant A: %v", err)
	}
	if err := tx.Create(&membershipB).Error; err != nil {
		t.Fatalf("expected one user to hold independent memberships in a second tenant, got error: %v", err)
	}

	var count int64
	if err := tx.Model(&TenantUser{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 tenant_users rows for one user across two tenants, got %d", count)
	}
}

func TestTenantUser_ReferentialIntegrity(t *testing.T) {
	db := testsupport.ConnectDB(t)

	t.Run("unknown tenant_id rejected", func(t *testing.T) {
		tx := testsupport.BeginRollbackTx(t, db)
		roleID, userID := seedRoleAndUser(t, tx)

		row := TenantUser{TenantID: 999999999, UserID: userID, RoleID: roleID, Status: TenantUserStatusActive}
		if err := tx.Create(&row).Error; err == nil {
			t.Fatalf("expected FK_tenant_users_tenant violation for a nonexistent tenant_id, got no error")
		}
	})

	t.Run("unknown user_id rejected", func(t *testing.T) {
		tx := testsupport.BeginRollbackTx(t, db)
		roleID, _ := seedRoleAndUser(t, tx)
		tenant := Tenant{Slug: "ref-integrity-user", Name: "Ref Integrity Co", Status: TenantStatusActive}
		if err := tx.Create(&tenant).Error; err != nil {
			t.Fatalf("create tenant: %v", err)
		}

		row := TenantUser{TenantID: tenant.ID, UserID: 999999999, RoleID: roleID, Status: TenantUserStatusActive}
		if err := tx.Create(&row).Error; err == nil {
			t.Fatalf("expected FK_tenant_users_user violation for a nonexistent user_id, got no error")
		}
	})

	t.Run("unknown role_id rejected", func(t *testing.T) {
		tx := testsupport.BeginRollbackTx(t, db)
		_, userID := seedRoleAndUser(t, tx)
		tenant := Tenant{Slug: "ref-integrity-role", Name: "Ref Integrity Role Co", Status: TenantStatusActive}
		if err := tx.Create(&tenant).Error; err != nil {
			t.Fatalf("create tenant: %v", err)
		}

		row := TenantUser{TenantID: tenant.ID, UserID: userID, RoleID: 999999999, Status: TenantUserStatusActive}
		if err := tx.Create(&row).Error; err == nil {
			t.Fatalf("expected FK_tenant_users_role violation for a nonexistent role_id, got no error")
		}
	})
}

func TestTenantUser_StatusDefaultsToInvited(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	roleID, userID := seedRoleAndUser(t, tx)
	tenant := Tenant{Slug: "default-status-co", Name: "Default Status Co", Status: TenantStatusActive}
	if err := tx.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Insert via raw SQL, deliberately omitting `status`, to verify the
	// column's own DF_tenant_users_status default — not GORM's
	// zero-value behavior, which would insert an empty string instead
	// of exercising the database default.
	if err := tx.Exec(
		"INSERT INTO tenant_users (tenant_id, user_id, role_id) VALUES (?, ?, ?)",
		tenant.ID, userID, roleID,
	).Error; err != nil {
		t.Fatalf("insert without status: %v", err)
	}

	var got TenantUser
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenant.ID, userID).First(&got).Error; err != nil {
		t.Fatalf("read back row: %v", err)
	}
	if got.Status != TenantUserStatusInvited {
		t.Fatalf("expected DF_tenant_users_status to default to %q, got %q", TenantUserStatusInvited, got.Status)
	}
	if got.JoinedAt != nil {
		t.Fatalf("expected joined_at to stay NULL until status becomes ACTIVE, got %v", *got.JoinedAt)
	}
}

func TestTenantSettings_OneRowPerTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenant := Tenant{Slug: "settings-co", Name: "Settings Co", Status: TenantStatusActive}
	if err := tx.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	first := TenantSetting{TenantID: tenant.ID, Timezone: "UTC", Locale: "en-US", Currency: "USD"}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first tenant_settings row: %v", err)
	}

	second := TenantSetting{TenantID: tenant.ID, Timezone: "America/New_York", Locale: "en-US", Currency: "USD"}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected PK_tenant_settings violation inserting a second settings row for the same tenant, got no error")
	}
}

func TestTenantAuditLog_AllowsNullTenantForPlatformLevelActions(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	_, userID := seedRoleAndUser(t, tx)

	entry := TenantAuditLog{
		TenantID:    nil,
		ActorUserID: userID,
		ActorType:   AuditActorPlatformAdmin,
		Action:      "tenant.create",
	}
	if err := tx.Create(&entry).Error; err != nil {
		t.Fatalf("expected a NULL tenant_id to be allowed for a platform-level action, got error: %v", err)
	}
}

func TestTenantAuditLog_ActorTypeCheckConstraintRejectsUnknownValue(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	_, userID := seedRoleAndUser(t, tx)

	entry := TenantAuditLog{ActorUserID: userID, ActorType: "NOT_A_REAL_ACTOR_TYPE", Action: "whatever"}
	if err := tx.Create(&entry).Error; err == nil {
		t.Fatalf("expected CK_tenant_audit_logs_actor_type violation, got no error")
	}
}
