package customer

import (
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Mirrors internal/product's own TestProduct_SKUUniqueWithinTenantOnly —
// proves UQ_customers_tenant_code rejects a duplicate code within the
// same tenant but allows the same code under a different tenant.

func TestCustomer_CodeUniqueWithinTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var bootstrapID int64
	if err := tx.Raw("SELECT id FROM tenants WHERE slug = 'default'").Scan(&bootstrapID).Error; err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}

	first := Customer{TenantID: bootstrapID, Code: "PHASE-MDE-CUST", CompanyName: "First Co.", Status: StatusActive}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first customer: %v", err)
	}

	duplicateSameTenant := Customer{TenantID: bootstrapID, Code: "PHASE-MDE-CUST", CompanyName: "Duplicate Co.", Status: StatusActive}
	if err := tx.Create(&duplicateSameTenant).Error; err == nil {
		t.Fatal("expected UQ_customers_tenant_code to reject a duplicate code within the same tenant")
	}

	var otherTenantID int64
	if err := tx.Raw("INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"phase-mde-cust-test-tenant", "Phase MDE Customer Test Tenant").Scan(&otherTenantID).Error; err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	sameCodeOtherTenant := Customer{TenantID: otherTenantID, Code: "PHASE-MDE-CUST", CompanyName: "Other Tenant Co.", Status: StatusActive}
	if err := tx.Create(&sameCodeOtherTenant).Error; err != nil {
		t.Fatalf("expected the same code under a different tenant to succeed, got: %v", err)
	}
}
