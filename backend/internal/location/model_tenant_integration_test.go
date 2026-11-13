package location

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Location Master Management integration tests — covers the
// UQ_locations_tenant_zone_code constraint (tenant_id, zone_id, code)
// already in place since migrations/0047/0048 (Phase 29A), which the
// application layer only started actually respecting with this phase's
// tenant-scoped Repository.

func TestLocation_CodeUniqueWithinTenantAndZoneOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	var zoneID int64
	if err := tx.Raw("SELECT TOP 1 id FROM zones WHERE tenant_id = ?", bootstrapID).Scan(&zoneID).Error; err != nil || zoneID == 0 {
		t.Skip("skipping: no zone exists for the bootstrap tenant in this test database")
	}

	first := Location{TenantID: bootstrapID, ZoneID: zoneID, Code: "PHASE-LMM-LOC", Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first location: %v", err)
	}

	duplicateSameTenantZone := Location{TenantID: bootstrapID, ZoneID: zoneID, Code: "PHASE-LMM-LOC", Status: StatusActive}
	if err := tx.Create(&duplicateSameTenantZone).Error; err == nil {
		t.Fatal("expected UQ_locations_tenant_zone_code to reject a duplicate code within the same tenant+zone")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase-lmm-loc-test-tenant", "Phase LMM Location Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	var otherWarehouseID int64
	if err := tx.Raw("INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		otherTenantID, "PHASE-LMM-WH", "Phase LMM Warehouse").Scan(&otherWarehouseID).Error; err != nil {
		t.Fatalf("insert second tenant's warehouse: %v", err)
	}
	var otherZoneID int64
	if err := tx.Raw("INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		otherTenantID, otherWarehouseID, "PHASE-LMM-Z", "Phase LMM Zone").Scan(&otherZoneID).Error; err != nil {
		t.Fatalf("insert second tenant's zone: %v", err)
	}

	sameCodeOtherTenant := Location{TenantID: otherTenantID, ZoneID: otherZoneID, Code: "PHASE-LMM-LOC", Status: StatusActive}
	if err := tx.Create(&sameCodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same code under a different tenant+zone to succeed, got: %v", err)
	}
}

// TestLocation_TypeCheckConstraint proves CK_locations_type
// (migrations/0050) allows NULL/'' (optional) and the 7 defined values,
// and rejects anything else — the database-level backstop behind
// Service's own Type.IsValid() check.
func TestLocation_TypeCheckConstraint(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}
	var zoneID int64
	if err := tx.Raw("SELECT TOP 1 id FROM zones WHERE tenant_id = ?", bootstrapID).Scan(&zoneID).Error; err != nil || zoneID == 0 {
		t.Skip("skipping: no zone exists for the bootstrap tenant in this test database")
	}

	validTyped := Location{TenantID: bootstrapID, ZoneID: zoneID, Code: "PHASE-LMM-TYPE-OK", Type: TypeStorage, Status: StatusActive}
	if err := tx.Create(&validTyped).Error; err != nil {
		t.Fatalf("expected a valid type to be accepted: %v", err)
	}

	noType := Location{TenantID: bootstrapID, ZoneID: zoneID, Code: "PHASE-LMM-TYPE-NULL", Status: StatusActive}
	if err := tx.Create(&noType).Error; err != nil {
		t.Fatalf("expected an unset (empty-string) type to be accepted: %v", err)
	}

	invalidTyped := Location{TenantID: bootstrapID, ZoneID: zoneID, Code: "PHASE-LMM-TYPE-BAD", Type: "NOT_REAL", Status: StatusActive}
	if err := tx.Create(&invalidTyped).Error; err == nil {
		t.Fatal("expected CK_locations_type to reject an invalid type value")
	}
}
