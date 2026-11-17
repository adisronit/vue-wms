package inventory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

// AddStock itself now retries on SQL Server choosing this session as a
// deadlock victim (error 1205) — see its doc comment in service.go.
// This test used to carry its own test-local retry wrapper to absorb
// that; it now calls svc.AddStock directly, which is the point: this
// test proves the production retry (not a test-only one) is what makes
// every concurrent caller converge to success.

// TestService_AddStock_ConcurrentFirstInsert_NeverFails is the real
// regression test for the addStockTx check-then-insert race: N
// goroutines calling AddStock concurrently against the exact same
// brand-new (tenant_id, product_id, location_id, lot_no) tuple used to
// let 1-of-N callers get a raw, unwrapped UNIQUE-constraint-violation
// error from CreateBalance (reproduced live before this fix at a
// 7-20% rate across several runs in this environment; the task's own
// audit measured ~3.3% elsewhere — same race, different hardware).
// After the fix, every one of those calls must succeed, and the final
// balance must be the exact sum of what every concurrent caller added
// — no lost updates, no double-counting. Mirrors
// TestService_ReserveStockTx_ConcurrentReservations_NeverExceedOnHand's
// own shape: real pooled *gorm.DB, not a shared *sql.Tx (database/sql
// does not allow concurrent use of one *sql.Tx by multiple goroutines),
// explicit FK-ordered cleanup instead of rollback.
func TestService_AddStock_ConcurrentFirstInsert_NeverFails(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('inv-conc-insert', 'Inventory Concurrent Insert Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-CI', 'Category CI')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-CI", Name: "Concurrent Insert Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-CI', 'Warehouse CI', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-CI', 'Zone CI', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-CI", Name: "Location CI", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	svc := NewService(NewRepository(db), product.NewRepository(db), location.NewRepository(db))

	const iterations = 10
	const concurrency = 10

	for iter := 0; iter < iterations; iter++ {
		lotNo := fmt.Sprintf("LOT-CI-%d", iter)

		var wg sync.WaitGroup
		var mu sync.Mutex
		var errs []error
		successCount := 0

		for g := 0; g < concurrency; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := svc.AddStock(context.Background(), StockChangeRequest{
					TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: lotNo,
					Quantity: 1, Type: TransactionAdjust,
				})
				mu.Lock()
				if err != nil {
					errs = append(errs, err)
				} else {
					successCount++
				}
				mu.Unlock()
			}()
		}
		wg.Wait()

		for _, err := range errs {
			t.Errorf("iter=%d: AddStock returned an error, want none: %v", iter, err)
		}

		// Quantity conservation: the committed balance must equal exactly
		// successCount (not the fixed `concurrency`) — this is the
		// invariant that actually matters: every caller AddStock reports
		// as successful must have landed exactly once, and no caller it
		// reports as failed may have landed at all. A mismatch here,
		// independent of whether successCount == concurrency, is a real
		// lost update or double-count; successCount < concurrency alone
		// (with every failure captured in errs above) just means some
		// callers genuinely failed, which the errs assertion above
		// already catches.
		var got Inventory
		if err := db.Where("tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?", tenantID, productID, locationID, lotNo).First(&got).Error; err != nil {
			t.Fatalf("iter=%d: read back balance: %v", iter, err)
		}
		if got.Quantity != float64(successCount) {
			t.Errorf("iter=%d: balance quantity = %v, want %v (successCount) — real lost update or double-count", iter, got.Quantity, successCount)
		}

		var ledgerCount int64
		if err := db.Model(&Transaction{}).Where("tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?", tenantID, productID, locationID, lotNo).Count(&ledgerCount).Error; err != nil {
			t.Fatalf("iter=%d: count ledger rows: %v", iter, err)
		}
		if ledgerCount != int64(successCount) {
			t.Errorf("iter=%d: ledger rows = %v, want %v (successCount) — balance/ledger committed out of step", iter, ledgerCount, successCount)
		}

		db.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ? AND location_id = ?", tenantID, locationID)
		db.Exec("DELETE FROM inventory WHERE tenant_id = ? AND lot_no = ?", tenantID, lotNo)
	}
}

// TestAddStockTx_PermanentFKViolation_NotRetried proves the retry added
// to addStockTx is scoped to the unique-violation case only: a genuine,
// permanent error (a nonexistent product_id, which CreateBalance's own
// FK constraint — fk_inventory_product — rejects) must return
// immediately with the usual translated ErrValidation, not be retried
// and not be silently swallowed or turned into some other failure.
// Calls addStockTx directly (this test file is in package inventory)
// rather than through Service.AddStock/AddStockTx, because
// Service.verifyOwnership would reject the bogus product_id before
// ever reaching addStockTx's retry logic — this test needs to exercise
// the retry gate itself, not the ownership check sitting in front of it.
func TestAddStockTx_PermanentFKViolation_NotRetried(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('inv-fk-notretry', 'Inventory FK Not-Retried Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-FKNR', 'Warehouse FKNR', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-FKNR', 'Zone FKNR', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-FKNR", Name: "Location FKNR", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	const bogusProductID = int64(999999999)

	err := db.Transaction(func(tx *gorm.DB) error {
		txRepo := NewRepository(tx)
		_, err := addStockTx(context.Background(), txRepo, StockChangeRequest{
			TenantID: tenantID, ProductID: bogusProductID, LocationID: locationID, LotNo: "LOT-FKNR",
			Quantity: 1, Type: TransactionAdjust,
		})
		return err
	})

	if err == nil {
		t.Fatal("addStockTx with a nonexistent product_id: want an error, got nil")
	}
	ve, ok := err.(ErrValidation)
	if !ok {
		t.Fatalf("addStockTx with a nonexistent product_id: want ErrValidation (translated FK violation), got %T: %v", err, err)
	}
	if ve.Message != "product does not exist" {
		t.Errorf("addStockTx with a nonexistent product_id: message = %q, want %q", ve.Message, "product does not exist")
	}

	// The retry gate (sqlerr.IsUniqueViolation(err) && attempt == 0) can
	// structurally never fire for an FK violation — error 547 is never
	// also error 2627/2601 — so there is no second AdjustBalance/
	// CreateBalance round trip to observe directly. What IS observable:
	// no balance row was left behind by any attempt (a retried/partially
	// applied path could have left one), confirming the transaction
	// rolled back cleanly on the first and only attempt.
	var count int64
	if err := db.Model(&Inventory{}).Where("tenant_id = ? AND lot_no = ?", tenantID, "LOT-FKNR").Count(&count).Error; err != nil {
		t.Fatalf("count balance rows: %v", err)
	}
	if count != 0 {
		t.Errorf("found %d balance row(s) for LOT-FKNR after a permanent FK-violation failure, want 0", count)
	}
}
