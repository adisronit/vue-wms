// Phase 30 Batch 1 — Inventory tenant isolation. Mirrors
// internal/product/tenant_isolation_test.go's own shape (fresh tenant
// per test, real seeded rows, testsupport.ConnectDB + BeginRollbackTx)
// but covers the additional surface unique to this package: the two
// raw-SQL atomic UPDATE statements (AdjustBalance/AdjustReservation),
// cross-tenant product/location ownership verification, and
// concurrent-reservation safety under real tenant scoping.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

// tenantFixture is one fully-seeded tenant: a product and a location,
// both real rows (via the real product/location repositories, not
// stubs) so verifyOwnership exercises the actual Exists() queries this
// batch added to those two packages.
type tenantFixture struct {
	tenantID   int64
	productID  int64
	locationID int64
}

func newTenantFixture(t *testing.T, tx *gorm.DB, label string) tenantFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("inv-iso-%s", label), fmt.Sprintf("Inventory Isolation Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newTenantFixture(%s): seed location: %v", label, err)
	}

	return tenantFixture{tenantID: tenantID, productID: p.ID, locationID: l.ID}
}

func seedBalance(t *testing.T, tx *gorm.DB, f tenantFixture, lotNo string, quantity, reserved float64) Inventory {
	t.Helper()
	inv := Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
	return inv
}

func newServiceAndRepos(tx *gorm.DB) (*Service, *Repository, *product.Repository, *location.Repository) {
	invRepo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	return NewService(invRepo, productRepo, locationRepo), invRepo, productRepo, locationRepo
}

// --- Read-side tenant scoping ---

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "list-a")
	b := newTenantFixture(t, tx, "list-b")
	seedBalance(t, tx, a, "LOT-A", 100, 0)
	seedBalance(t, tx, b, "LOT-B", 200, 0)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's balance)", len(rows), total)
	}
	if rows[0].LotNo != "LOT-A" {
		t.Errorf("List(tenantA) returned lot %q, want LOT-A", rows[0].LotNo)
	}
}

func TestRepository_FindByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "find-a")
	b := newTenantFixture(t, tx, "find-b")
	balanceB := seedBalance(t, tx, b, "LOT-B", 50, 0)

	repo := NewRepository(tx)
	if _, err := repo.FindByID(context.Background(), a.tenantID, balanceB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID(tenantA, tenantB's balance) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), b.tenantID, balanceB.ID); err != nil {
		t.Errorf("FindByID(tenantB, its own balance) = %v, want success", err)
	}
}

func TestRepository_Summary_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "summary-a")
	b := newTenantFixture(t, tx, "summary-b")
	seedBalance(t, tx, a, "LOT-A", 100, 10)
	seedBalance(t, tx, b, "LOT-B", 999, 0)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.AvailableQty != 90 {
		t.Errorf("Summary(tenantA).AvailableQty = %v, want 90 (100-10, must not include tenant B's 999)", summary.AvailableQty)
	}
}

// --- Write-side: the atomic raw-SQL UPDATEs themselves ---

// TestRepository_AdjustBalance_WrongTenant_NoRowsAffected proves the
// highest-risk fix in this batch directly: a caller passing a
// mismatched tenantID — even one that correctly names the real
// product_id/location_id/lot_no of an existing balance owned by a
// *different* tenant — affects zero rows. Before this batch, this
// exact call would have silently mutated tenant A's quantity.
func TestRepository_AdjustBalance_WrongTenant_NoRowsAffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "adjust-a")
	b := newTenantFixture(t, tx, "adjust-b")
	seedBalance(t, tx, a, "LOT-A", 100, 0)

	repo := NewRepository(tx)
	rows, err := repo.AdjustBalance(context.Background(), b.tenantID, a.productID, a.locationID, "LOT-A", -50)
	if err != nil {
		t.Fatalf("AdjustBalance: %v", err)
	}
	if rows != 0 {
		t.Fatalf("AdjustBalance(tenantB, tenantA's product/location) affected %d rows, want 0", rows)
	}

	bal, err := repo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after rejected cross-tenant adjust: %v", err)
	}
	if bal.Quantity != 100 {
		t.Errorf("tenant A's quantity = %v after a rejected cross-tenant AdjustBalance, want unchanged 100", bal.Quantity)
	}
}

func TestRepository_AdjustReservation_WrongTenant_NoRowsAffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "adjustres-a")
	b := newTenantFixture(t, tx, "adjustres-b")
	seedBalance(t, tx, a, "LOT-A", 100, 20)

	repo := NewRepository(tx)
	rows, err := repo.AdjustReservation(context.Background(), b.tenantID, a.productID, a.locationID, "LOT-A", 10)
	if err != nil {
		t.Fatalf("AdjustReservation: %v", err)
	}
	if rows != 0 {
		t.Fatalf("AdjustReservation(tenantB, tenantA's product/location) affected %d rows, want 0", rows)
	}

	bal, err := repo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after rejected cross-tenant reserve: %v", err)
	}
	if bal.ReservedQuantity != 20 {
		t.Errorf("tenant A's reserved_quantity = %v after a rejected cross-tenant AdjustReservation, want unchanged 20", bal.ReservedQuantity)
	}
}

// --- Service-layer ownership verification ---

func TestService_AddStockTx_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "own-product-a")
	b := newTenantFixture(t, tx, "own-product-b")

	svc, repo, _, _ := newServiceAndRepos(tx)
	err := repo.Transaction(context.Background(), func(txRepo *Repository) error {
		_, err := svc.AddStockTx(context.Background(), txRepo, StockChangeRequest{
			TenantID: a.tenantID, ProductID: b.productID, LocationID: a.locationID,
			LotNo: "LOT-X", Quantity: 10, Type: TransactionAdjust,
		})
		return err
	})
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("AddStockTx(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_AddStockTx_RejectsCrossTenantLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "own-location-a")
	b := newTenantFixture(t, tx, "own-location-b")

	svc, repo, _, _ := newServiceAndRepos(tx)
	err := repo.Transaction(context.Background(), func(txRepo *Repository) error {
		_, err := svc.AddStockTx(context.Background(), txRepo, StockChangeRequest{
			TenantID: a.tenantID, ProductID: a.productID, LocationID: b.locationID,
			LotNo: "LOT-X", Quantity: 10, Type: TransactionAdjust,
		})
		return err
	})
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("AddStockTx(tenantA, tenantB's location_id) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_AddStockTx_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "own-ok")

	svc, repo, _, _ := newServiceAndRepos(tx)
	var result *Inventory
	err := repo.Transaction(context.Background(), func(txRepo *Repository) error {
		inv, err := svc.AddStockTx(context.Background(), txRepo, StockChangeRequest{
			TenantID: a.tenantID, ProductID: a.productID, LocationID: a.locationID,
			LotNo: "LOT-OK", Quantity: 25, Type: TransactionAdjust,
		})
		result = inv
		return err
	})
	if err != nil {
		t.Fatalf("AddStockTx(own tenant's own product/location): %v", err)
	}
	if result.Quantity != 25 {
		t.Errorf("Quantity = %v, want 25", result.Quantity)
	}
}

func TestService_ReserveStockTx_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "reserve-cross-a")
	b := newTenantFixture(t, tx, "reserve-cross-b")
	seedBalance(t, tx, a, "LOT-A", 100, 0)

	svc, repo, _, _ := newServiceAndRepos(tx)
	err := repo.Transaction(context.Background(), func(txRepo *Repository) error {
		_, err := svc.ReserveStockTx(context.Background(), txRepo, ReserveRequest{
			TenantID: a.tenantID, ProductID: b.productID, LocationID: a.locationID, LotNo: "LOT-A", Quantity: 10,
		})
		return err
	})
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("ReserveStockTx(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

// --- Rollback / atomicity ---

// TestService_MoveStock_InsufficientSource_LeavesDestinationUnchanged
// proves MoveStock's two legs (remove from source, add to destination)
// still commit or roll back together now that both legs carry
// tenant_id — an insufficient-stock failure on the remove leg must
// never let the add leg execute.
func TestService_MoveStock_InsufficientSource_LeavesDestinationUnchanged(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTenantFixture(t, tx, "move-rollback")

	var toLocationID int64
	var zoneID int64
	if err := tx.Raw("SELECT zone_id FROM locations WHERE id = ?", a.locationID).Scan(&zoneID).Error; err != nil {
		t.Fatalf("lookup source zone_id: %v", err)
	}
	dest := location.Location{TenantID: a.tenantID, ZoneID: zoneID, Code: "LOC-move-dest", Name: "Dest", Status: location.StatusActive}
	if err := tx.Create(&dest).Error; err != nil {
		t.Fatalf("seed destination location: %v", err)
	}
	toLocationID = dest.ID

	seedBalance(t, tx, a, "LOT-MOVE", 5, 0) // only 5 on hand

	svc := NewService(NewRepository(tx), product.NewRepository(tx), location.NewRepository(tx))
	err := svc.MoveStock(context.Background(), MoveStockRequest{
		TenantID: a.tenantID, ProductID: a.productID, FromLocationID: a.locationID, ToLocationID: toLocationID,
		LotNo: "LOT-MOVE", Quantity: 100, // more than the 5 available
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("MoveStock(insufficient source) = %v, want ErrInsufficientStock", err)
	}

	if _, err := NewRepository(tx).FindBalance(context.Background(), a.tenantID, a.productID, toLocationID, "LOT-MOVE"); !errors.Is(err, ErrNotFound) {
		t.Errorf("destination balance exists after a failed MoveStock — the two legs did not roll back together")
	}
}

// --- Concurrency: the Phase 12 guarantee, now tenant-scoped ---

// TestService_ReserveStockTx_ConcurrentReservations_NeverExceedOnHand
// re-proves Phase 12's own concurrency guarantee (concurrent
// reservations against the same tuple can never together exceed
// on-hand quantity) still holds with tenant_id added to
// AdjustReservation's WHERE clause — the fix must not have
// reintroduced a read-then-write race. Deliberately does NOT use
// testsupport.BeginRollbackTx: a single *sql.Tx is not safe for
// concurrent use by multiple goroutines (database/sql's own
// documented restriction), so true concurrent-connection coverage
// needs the real pooled *gorm.DB and explicit, FK-ordered cleanup
// instead of rollback.
func TestService_ReserveStockTx_ConcurrentReservations_NeverExceedOnHand(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('inv-concurrent', 'Inventory Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		// FK-ordered cleanup, children first — this test commits for
		// real (unlike every other test in this file), so it must clean
		// up for real too rather than relying on a rollback.
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-CONC', 'Category Conc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-CONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-CONC', 'Warehouse Conc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-CONC', 'Zone Conc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-CONC", Name: "Location Conc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	inv := Inventory{TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-CONC", Quantity: 10, ReservedQuantity: 0}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("seed balance: %v", err)
	}

	const attempts = 20
	var succeeded int32
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo := NewRepository(db)
			rows, err := repo.AdjustReservation(context.Background(), tenantID, productID, locationID, "LOT-CONC", 1)
			if err != nil {
				t.Errorf("AdjustReservation: %v", err)
				return
			}
			if rows == 1 {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if succeeded != 10 {
		t.Errorf("succeeded reservations = %d, want exactly 10 (on-hand quantity) — concurrent reservations must never together exceed it", succeeded)
	}

	bal, err := NewRepository(db).FindBalance(context.Background(), tenantID, productID, locationID, "LOT-CONC")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.ReservedQuantity != 10 {
		t.Errorf("final reserved_quantity = %v, want 10", bal.ReservedQuantity)
	}
}
