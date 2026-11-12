package warehouse

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Phase 29A integration tests for Warehouse.TenantID — exercise the
// real tenant_id column, DEFAULT constraint, FK, and tenant-scoped
// UNIQUE constraint added by migrations/0043 and 0047/0048, through the
// GORM model itself (not just raw SQL — see
// cmd/migrate/tenant_isolation_integration_test.go for the migration-
// level equivalents). Skip cleanly via testsupport.ConnectDB when no
// database is configured; every test rolls back via
// testsupport.BeginRollbackTx so nothing is left behind.

func TestWarehouse_TenantIDDefaultsToBootstrapTenantWhenOmitted(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	// Deliberately does not set TenantID — the exact shape of every
	// pre-existing, unmodified repository.Create() call in this
	// codebase (Phase 29A's scope is migrations + model structs +
	// migration tests only, see internal/role.Role's own TenantID doc).
	wh := Warehouse{Code: "PHASE29A-WH-DEFAULT", Name: "Default Tenant WH", Type: "DC", Status: StatusActive}
	if err := tx.Create(&wh).Error; err != nil {
		t.Fatalf("create warehouse without setting TenantID: %v", err)
	}

	var reloaded Warehouse
	if err := tx.First(&reloaded, "id = ?", wh.ID).Error; err != nil {
		t.Fatalf("reload warehouse: %v", err)
	}

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	if reloaded.TenantID != bootstrapID {
		t.Fatalf("expected the DEFAULT constraint to backfill TenantID = %d (bootstrap tenant), got %d", bootstrapID, reloaded.TenantID)
	}
}

func TestWarehouse_CodeUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	first := Warehouse{TenantID: bootstrapID, Code: "PHASE29A-DUP", Name: "First", Type: "DC", Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first warehouse: %v", err)
	}

	duplicateSameTenant := Warehouse{TenantID: bootstrapID, Code: "PHASE29A-DUP", Name: "Duplicate", Type: "DC", Status: StatusActive}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected UQ_warehouses_tenant_code to reject a duplicate code within the same tenant")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase29a-warehouse-test-tenant", "Phase29A Warehouse Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameCodeOtherTenant := Warehouse{TenantID: otherTenantID, Code: "PHASE29A-DUP", Name: "Other Tenant", Type: "DC", Status: StatusActive}
	if err := tx.Create(&sameCodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same code under a different tenant to succeed, got: %v", err)
	}
}

func TestWarehouse_TenantIDRejectsUnknownTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	wh := Warehouse{TenantID: 9_000_000_000, Code: "PHASE29A-GHOST", Name: "Ghost", Type: "DC", Status: StatusActive}
	if err := tx.Create(&wh).Error; err == nil {
		t.Fatal("expected FK_warehouses_tenant to reject a tenant_id with no matching tenants row")
	}
}
