package productcategory

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Mirrors internal/product's own TestProduct_SKUUniqueWithinTenantOnly
// (model_tenant_integration_test.go) — proves UQ_product_categories_tenant_code
// rejects a duplicate code within the same tenant but allows the same
// code under a different tenant.

func TestCategory_CodeUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	first := Category{TenantID: bootstrapID, Code: "PHASE-MDE-CAT", Name: "First", Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first category: %v", err)
	}

	duplicateSameTenant := Category{TenantID: bootstrapID, Code: "PHASE-MDE-CAT", Name: "Duplicate", Status: StatusActive}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected UQ_product_categories_tenant_code to reject a duplicate code within the same tenant")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase-mde-cat-test-tenant", "Phase MDE Category Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameCodeOtherTenant := Category{TenantID: otherTenantID, Code: "PHASE-MDE-CAT", Name: "Other Tenant", Status: StatusActive}
	if err := tx.Create(&sameCodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same code under a different tenant to succeed, got: %v", err)
	}
}

// TestCategory_ParentMustExist proves the FK_product_categories_parent
// constraint rejects a parent_id that doesn't exist at all — the
// database-level backstop behind Service.validateParent's own
// application-level existence check.
func TestCategory_ParentMustExist(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	bogusParent := int64(99999999)
	c := Category{TenantID: bootstrapID, Code: "PHASE-MDE-CAT-ORPHAN", Name: "Orphan", ParentID: &bogusParent, Status: StatusActive}
	if err := tx.Create(&c).Error; err == nil {
		t.Fatal("expected FK_product_categories_parent to reject a nonexistent parent_id")
	}
}
