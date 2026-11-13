package productfamily

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Mirrors internal/productcategory's own TestCategory_CodeUniqueWithinTenantOnly.
func TestFamily_CodeUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	first := Family{TenantID: bootstrapID, Code: "EWTA-FAMILY", Name: "First", Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first family: %v", err)
	}

	duplicateSameTenant := Family{TenantID: bootstrapID, Code: "EWTA-FAMILY", Name: "Duplicate", Status: StatusActive}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected UQ_product_families_tenant_code to reject a duplicate code within the same tenant")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"ewta-family-test-tenant", "EWTA Family Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameCodeOtherTenant := Family{TenantID: otherTenantID, Code: "EWTA-FAMILY", Name: "Other Tenant", Status: StatusActive}
	if err := tx.Create(&sameCodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same code under a different tenant to succeed, got: %v", err)
	}
}
