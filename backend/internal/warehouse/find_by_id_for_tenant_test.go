package warehouse

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// Phase 31A — FindByIDForTenant is the first tenant-scoped warehouse
// lookup anywhere in this codebase (see that method's own doc for why
// Receiving is its first caller); this proves the IDOR guarantee
// directly rather than only through internal/receiving's own tests.
func TestRepository_FindByIDForTenant_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	var tenantA, tenantB int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('wh-fbft-a', 'WH FindByIDForTenant A', 'ACTIVE')",
	).Scan(&tenantA).Error; err != nil {
		t.Fatalf("insert tenant A: %v", err)
	}
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('wh-fbft-b', 'WH FindByIDForTenant B', 'ACTIVE')",
	).Scan(&tenantB).Error; err != nil {
		t.Fatalf("insert tenant B: %v", err)
	}

	wh := Warehouse{TenantID: tenantB, Code: "WH-FBFT-B", Name: "Tenant B Warehouse", Type: "DC", Status: StatusActive}
	if err := tx.Create(&wh).Error; err != nil {
		t.Fatalf("seed tenant B warehouse: %v", err)
	}

	repo := NewRepository(tx)
	if _, err := repo.FindByIDForTenant(context.Background(), tenantA, wh.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByIDForTenant(tenantA, tenantB's warehouse) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByIDForTenant(context.Background(), tenantB, wh.ID); err != nil {
		t.Errorf("FindByIDForTenant(tenantB, its own warehouse) = %v, want success", err)
	}
}
