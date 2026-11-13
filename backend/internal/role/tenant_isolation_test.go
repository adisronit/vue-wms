package role

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/rbac"
)

// seedTenantWithCatalog inserts a brand-new tenant row plus its own copy
// of rbac.RoleCatalog (the same 7-role set tenant.Repository.CreateWithAudit
// seeds in production — see this package's own model.go doc) directly on
// tx, so these tests exercise Repository/Service against real tenant_id
// values and real UQ_roles_tenant_code constraints without needing the
// legacy test/isolation HTTP harness's login flow (whose fixtures predate
// Phase 29E's tenant_users membership requirement and cannot authenticate
// today — a separate, pre-existing gap, not something this file fixes).
func seedTenantWithCatalog(t *testing.T, tx *gorm.DB, label string) int64 {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("role-isolation-%s", label), fmt.Sprintf("Role Isolation Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seedTenantWithCatalog(%s): insert tenant: %v", label, err)
	}

	for _, def := range rbac.RoleCatalog {
		role := Role{
			TenantID:    tenantID,
			Code:        def.Code,
			Name:        def.Name,
			Description: def.Description,
			IsActive:    true,
		}
		if err := tx.Create(&role).Error; err != nil {
			t.Fatalf("seedTenantWithCatalog(%s): create role %s: %v", label, def.Code, err)
		}
	}

	return tenantID
}

// TestListRoles_EachTenantSeesOnlyItsOwnSevenCatalogRoles is the task's
// own acceptance check: two tenants, each seeded with the real 7-role
// rbac.RoleCatalog, must each see exactly their own 7 rows from
// Service.ListRoles — never 14 (the old ListRoles(ctx) with no tenant_id
// filter at all), and never another tenant's rows.
func TestListRoles_EachTenantSeesOnlyItsOwnSevenCatalogRoles(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA := seedTenantWithCatalog(t, tx, "a")
	tenantB := seedTenantWithCatalog(t, tx, "b")

	repo := NewRepository(tx)
	svc := NewService(repo, nil)

	rolesA, err := svc.ListRoles(context.Background(), tenantA)
	if err != nil {
		t.Fatalf("ListRoles(tenantA): %v", err)
	}
	if len(rolesA) != len(rbac.RoleCatalog) {
		t.Errorf("tenant A: got %d roles, want exactly %d (the catalog size) — got %+v", len(rolesA), len(rbac.RoleCatalog), rolesA)
	}
	for _, r := range rolesA {
		if r.TenantID != tenantA {
			t.Errorf("tenant A's ListRoles leaked a row owned by tenant %d: %+v", r.TenantID, r)
		}
	}

	rolesB, err := svc.ListRoles(context.Background(), tenantB)
	if err != nil {
		t.Fatalf("ListRoles(tenantB): %v", err)
	}
	if len(rolesB) != len(rbac.RoleCatalog) {
		t.Errorf("tenant B: got %d roles, want exactly %d", len(rolesB), len(rbac.RoleCatalog))
	}
}

// TestGetRole_CrossTenant_ReturnsNotFound proves the IDOR
// TestTenantA_CannotReadOrMutateTenantB_CustomRole (test/isolation)
// described is closed at the Service layer: tenant A's own tenantID
// can no longer resolve tenant B's role by ID.
func TestGetRole_CrossTenant_ReturnsNotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA := seedTenantWithCatalog(t, tx, "cross-get-a")
	tenantB := seedTenantWithCatalog(t, tx, "cross-get-b")

	repo := NewRepository(tx)
	svc := NewService(repo, nil)

	rolesB, err := svc.ListRoles(context.Background(), tenantB)
	if err != nil || len(rolesB) == 0 {
		t.Fatalf("seeding tenant B's roles: err=%v len=%d", err, len(rolesB))
	}
	targetID := rolesB[0].ID

	if _, err := svc.GetRole(context.Background(), tenantA, targetID); err != ErrNotFound {
		t.Errorf("GetRole(tenantA, tenantB's role %d) = %v, want ErrNotFound", targetID, err)
	}

	// Sanity: tenant B itself can still read its own role.
	if _, err := svc.GetRole(context.Background(), tenantB, targetID); err != nil {
		t.Errorf("GetRole(tenantB, its own role %d) = %v, want success", targetID, err)
	}
}

// TestSetRolePermissions_CrossTenant_RejectedWithoutMutating proves the
// write side of the same IDOR is closed too: tenant A cannot overwrite
// tenant B's role's permission grants by guessing its ID.
func TestSetRolePermissions_CrossTenant_RejectedWithoutMutating(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA := seedTenantWithCatalog(t, tx, "cross-set-a")
	tenantB := seedTenantWithCatalog(t, tx, "cross-set-b")

	repo := NewRepository(tx)
	svc := NewService(repo, nil)

	rolesB, err := svc.ListRoles(context.Background(), tenantB)
	if err != nil || len(rolesB) == 0 {
		t.Fatalf("seeding tenant B's roles: err=%v len=%d", err, len(rolesB))
	}
	targetID := rolesB[0].ID

	before, err := repo.RolePermissionCodes(context.Background(), targetID)
	if err != nil {
		t.Fatalf("RolePermissionCodes before: %v", err)
	}

	if _, err := svc.SetRolePermissions(context.Background(), tenantA, targetID, []string{"warehouse.view"}); err != ErrNotFound {
		t.Errorf("SetRolePermissions(tenantA, tenantB's role %d) = %v, want ErrNotFound", targetID, err)
	}

	after, err := repo.RolePermissionCodes(context.Background(), targetID)
	if err != nil {
		t.Fatalf("RolePermissionCodes after: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("CROSS-TENANT WRITE CONFIRMED: tenant B's role %d permissions changed from %v to %v after tenant A's rejected call", targetID, before, after)
	}
}
