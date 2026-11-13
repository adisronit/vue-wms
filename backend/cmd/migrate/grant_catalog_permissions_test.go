package main

import (
	"context"
	"testing"

	"wms-backend/internal/platform/logging"
	"wms-backend/internal/rbac"
)

// Deployment Readiness fix regression test — proves
// grantCatalogPermissionsToAllTenants closes the exact gap
// TestBackfillTenantRoleCatalogs_OnlyFillsTenantsWithZeroRoles
// deliberately does NOT cover: a tenant that already has a role (so
// backfillTenantRoleCatalogs' zero-role check skips it) but is still
// missing some or all of that role's rbac.RoleDefaultGrants —
// the real-world shape of "a tenant was created before a new
// permission module (e.g. Master Data Expansion's product_category/
// customer/supplier) existed in the catalog."
//
// Proves three things in one pass: a tenant with an existing,
// zero-grant ADMIN role gets topped up to the full catalog; a tenant
// whose only role has no rbac.RoleDefaultGrants entry at all (a
// hand-rolled custom role) is left completely untouched; and running
// the sweep twice never double-grants (idempotent).
func TestGrantCatalogPermissionsToAllTenants_TopsUpExistingRolesAcrossTenants(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)
	logger := logging.New("test", "migrate-test")

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	permissionIDs := map[string]int64{}
	rows, err := tx.QueryContext(ctx, "SELECT code, id FROM permissions")
	if err != nil {
		t.Fatalf("load permission catalog: %v", err)
	}
	for rows.Next() {
		var code string
		var id int64
		if err := rows.Scan(&code, &id); err != nil {
			rows.Close()
			t.Fatalf("scan permission row: %v", err)
		}
		permissionIDs[code] = id
	}
	rows.Close()
	if len(permissionIDs) == 0 {
		t.Skip("skipping: permissions table is empty in this test database (run `wms-migrate seed` first)")
	}

	// Tenant A: already has its own ADMIN role (simulating a tenant
	// created before Master Data Expansion's permissions existed), but
	// zero rows in role_permissions for it — exactly what
	// backfillTenantRoleCatalogs' existingRoleCount > 0 check skips.
	var tenantAID int64
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('grant-sweep-test-a', 'Grant Sweep Test A', 'ACTIVE')").
		Scan(&tenantAID); err != nil {
		t.Fatalf("insert tenant A: %v", err)
	}
	var adminRoleID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (@p1, 'ADMIN', 'Administrator', 'pre-existing role, zero grants', 1)`,
		tenantAID).Scan(&adminRoleID); err != nil {
		t.Fatalf("insert tenant A's ADMIN role: %v", err)
	}

	// Tenant B: a hand-rolled custom role with no rbac.RoleDefaultGrants
	// entry at all — must stay at zero grants after the sweep.
	var tenantBID int64
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('grant-sweep-test-b', 'Grant Sweep Test B', 'ACTIVE')").
		Scan(&tenantBID); err != nil {
		t.Fatalf("insert tenant B: %v", err)
	}
	var customRoleID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (@p1, 'GRANT_SWEEP_CUSTOM', 'Custom', 'hand-rolled, not from rbac.RoleCatalog', 1)`,
		tenantBID).Scan(&customRoleID); err != nil {
		t.Fatalf("insert tenant B's custom role: %v", err)
	}

	run := func() {
		if err := grantCatalogPermissionsToAllTenants(ctx, tx, logger, permissionIDs); err != nil {
			t.Fatalf("grantCatalogPermissionsToAllTenants: %v", err)
		}
	}
	run()
	run() // idempotency: a second pass must not duplicate or error

	var adminGrantCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM role_permissions WHERE role_id = @p1", adminRoleID).Scan(&adminGrantCount); err != nil {
		t.Fatalf("count tenant A's ADMIN grants: %v", err)
	}
	if adminGrantCount != len(rbac.PermissionCatalog()) {
		t.Errorf("expected tenant A's pre-existing ADMIN role to be topped up to every catalog permission (%d), got %d",
			len(rbac.PermissionCatalog()), adminGrantCount)
	}

	var customGrantCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM role_permissions WHERE role_id = @p1", customRoleID).Scan(&customGrantCount); err != nil {
		t.Fatalf("count tenant B's custom role grants: %v", err)
	}
	if customGrantCount != 0 {
		t.Errorf("expected tenant B's custom role (no rbac.RoleDefaultGrants entry) to stay untouched at 0 grants, got %d", customGrantCount)
	}
}

// TestSeedRoles_ScopesLookupToGivenTenant proves the bug fix directly:
// seedRoles must return the role ids belonging to the TENANT PASSED IN,
// never an arbitrary other tenant's same-named role — reproduced by
// seeding a second tenant's "ADMIN" role first (the exact precondition
// that made the old, tenant-unscoped `WHERE code = @p1` lookup
// ambiguous), then confirming seedRoles(tenantID) for a THIRD, fresh
// tenant returns ids that actually belong to that third tenant.
func TestSeedRoles_ScopesLookupToGivenTenant(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)
	logger := logging.New("test", "migrate-test")

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	// A decoy tenant with its own ADMIN role, created first — before
	// the fix, this is exactly the row an unscoped lookup could return
	// instead of the target tenant's own.
	var decoyTenantID int64
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('seedroles-test-decoy', 'SeedRoles Test Decoy', 'ACTIVE')").
		Scan(&decoyTenantID); err != nil {
		t.Fatalf("insert decoy tenant: %v", err)
	}
	var decoyAdminRoleID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (@p1, 'ADMIN', 'Administrator', 'decoy', 1)`,
		decoyTenantID).Scan(&decoyAdminRoleID); err != nil {
		t.Fatalf("insert decoy ADMIN role: %v", err)
	}

	var targetTenantID int64
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('seedroles-test-target', 'SeedRoles Test Target', 'ACTIVE')").
		Scan(&targetTenantID); err != nil {
		t.Fatalf("insert target tenant: %v", err)
	}

	roleIDs, err := seedRoles(ctx, tx, logger, targetTenantID)
	if err != nil {
		t.Fatalf("seedRoles: %v", err)
	}

	targetAdminRoleID, ok := roleIDs["ADMIN"]
	if !ok {
		t.Fatal("seedRoles did not return an ADMIN role id")
	}
	if targetAdminRoleID == decoyAdminRoleID {
		t.Fatal("seedRoles(targetTenantID) returned the DECOY tenant's ADMIN role id — tenant-scoping regression")
	}

	var actualTenantID int64
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id FROM roles WHERE id = @p1", targetAdminRoleID).Scan(&actualTenantID); err != nil {
		t.Fatalf("lookup returned role's tenant_id: %v", err)
	}
	if actualTenantID != targetTenantID {
		t.Errorf("seedRoles(targetTenantID=%d) returned a role actually owned by tenant %d", targetTenantID, actualTenantID)
	}

	// Idempotency: calling it again for the same tenant must return the
	// SAME role ids, not create duplicates.
	roleIDsAgain, err := seedRoles(ctx, tx, logger, targetTenantID)
	if err != nil {
		t.Fatalf("seedRoles (second call): %v", err)
	}
	if roleIDsAgain["ADMIN"] != targetAdminRoleID {
		t.Errorf("seedRoles called twice for the same tenant returned different ADMIN role ids: %d then %d", targetAdminRoleID, roleIDsAgain["ADMIN"])
	}
	var roleCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE tenant_id = @p1 AND code = 'ADMIN'", targetTenantID).Scan(&roleCount); err != nil {
		t.Fatalf("count target tenant's ADMIN roles: %v", err)
	}
	if roleCount != 1 {
		t.Errorf("expected exactly 1 ADMIN role for the target tenant after two seedRoles calls, got %d", roleCount)
	}
}
