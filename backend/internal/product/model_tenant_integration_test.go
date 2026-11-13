package product

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Phase 29A integration tests for Product.TenantID — covers the one
// constraint pattern internal/warehouse's own tenant integration test
// doesn't: a tenant-scoped FILTERED unique index (barcode, which allows
// multiple NULLs) rather than a plain composite UNIQUE constraint. See
// migrations/0048's header comment for why barcode needed a filtered
// index both before and after this phase.

func TestProduct_SKUUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	first := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU", Name: "First", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first product: %v", err)
	}

	duplicateSameTenant := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU", Name: "Duplicate", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected UQ_products_tenant_sku to reject a duplicate SKU within the same tenant")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase29a-product-test-tenant", "Phase29A Product Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameSKUOtherTenant := Product{TenantID: otherTenantID, SKU: "PHASE29A-SKU", Name: "Other Tenant", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive}
	if err := tx.Create(&sameSKUOtherTenant).Error; err != nil {
		t.Fatalf("expected the same SKU under a different tenant to succeed, got: %v", err)
	}
}

func TestProduct_BarcodeFilteredUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	barcode := "PHASE29A-BARCODE"

	first := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU-A", Name: "First", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive, Barcode: &barcode}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first product with barcode: %v", err)
	}

	duplicateSameTenant := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU-B", Name: "Duplicate Barcode", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive, Barcode: &barcode}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected the tenant-scoped filtered UQ_products_barcode index to reject a duplicate barcode within the same tenant")
	}

	// Multiple NULL barcodes within the SAME tenant must still be
	// allowed — the entire reason this is a filtered index, not a plain
	// UNIQUE constraint (see migrations/0008's original comment, and
	// migrations/0048's tenant-scoped carry-forward of it).
	noBarcodeA := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU-C", Name: "No Barcode A", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive}
	noBarcodeB := Product{TenantID: bootstrapID, SKU: "PHASE29A-SKU-D", Name: "No Barcode B", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive}
	if err := tx.Create(&noBarcodeA).Error; err != nil {
		t.Fatalf("create first NULL-barcode product: %v", err)
	}
	if err := tx.Create(&noBarcodeB).Error; err != nil {
		t.Fatalf("expected a second NULL barcode within the same tenant to be allowed, got: %v", err)
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase29a-barcode-test-tenant", "Phase29A Barcode Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameBarcodeOtherTenant := Product{TenantID: otherTenantID, SKU: "PHASE29A-SKU-E", Name: "Other Tenant", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: StatusActive, Barcode: &barcode}
	if err := tx.Create(&sameBarcodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same barcode under a different tenant to succeed, got: %v", err)
	}
}
