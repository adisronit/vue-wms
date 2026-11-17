package inventoryreport

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

type fixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newFixture(t *testing.T, tx *gorm.DB, label string) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"invr-"+label, "Inventory Report Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newFixture(%s): seed location: %v", label, err)
	}

	return fixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedBalance(t *testing.T, tx *gorm.DB, f fixture, lotNo string, qty, reserved, hold float64, expiry *time.Time) int64 {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo,
		Quantity: qty, ReservedQuantity: reserved, HoldQuantity: hold, ExpiryDate: expiry,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
	return inv.ID
}

func seedTransaction(t *testing.T, tx *gorm.DB, f fixture, lotNo string, txType inventory.TransactionType, qty float64, createdAt time.Time) {
	t.Helper()
	txn := inventory.Transaction{
		TenantID: f.tenantID, TransactionType: txType, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: qty,
	}
	if err := tx.Create(&txn).Error; err != nil {
		t.Fatalf("seedTransaction: %v", err)
	}
	if err := tx.Exec("UPDATE inventory_transactions SET created_at = ? WHERE id = ? AND tenant_id = ?", createdAt, txn.ID, f.tenantID).Error; err != nil {
		t.Fatalf("seedTransaction: stamp created_at: %v", err)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// --- Tenant isolation ---

func TestRepository_List_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "list-a")
	b := newFixture(t, tx, "list-b")
	seedBalance(t, tx, a, "LOT-A", 100, 0, 0, nil)
	seedBalance(t, tx, b, "LOT-B", 999, 0, 0, nil)

	now := time.Now().UTC()
	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20}, now)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].LotNo != "LOT-A" {
		t.Fatalf("List(tenantA) = %+v (total %d), want exactly LOT-A", rows, total)
	}
}

func TestRepository_Summary_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "sum-a")
	b := newFixture(t, tx, "sum-b")
	seedBalance(t, tx, a, "LOT-A", 10, 0, 0, nil)
	seedBalance(t, tx, b, "LOT-B", 999, 0, 0, nil)

	now := time.Now().UTC()
	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{}, now)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	for _, uq := range summary.TotalOnHandByUOM {
		if uq.Qty >= 999 {
			t.Errorf("TotalOnHandByUOM contains %v, tenant B's huge quantity leaked across tenant boundary", uq)
		}
	}
}

// --- Available computation ---

func TestRepository_List_AvailableComputation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "avail-a")
	seedBalance(t, tx, a, "LOT-AVAIL", 100, 20, 15, nil)

	now := time.Now().UTC()
	repo := NewRepository(tx)
	rows, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20}, now)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Available != 65 {
		t.Errorf("Available = %v, want 65 (100 - 20 - 15)", rows[0].Available)
	}
	if rows[0].InventoryStatus != "ON_HOLD" {
		t.Errorf("InventoryStatus = %q, want ON_HOLD (hold_quantity > 0 takes precedence)", rows[0].InventoryStatus)
	}
}

// --- Low Stock lens ---

func TestRepository_List_BelowQty_LowStockLens(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "low-a")
	seedBalance(t, tx, a, "LOT-LOW", 5, 0, 0, nil)   // available 5
	seedBalance(t, tx, a, "LOT-HIGH", 50, 0, 0, nil) // available 50

	now := time.Now().UTC()
	threshold := 10.0
	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20, Filters: Filters{BelowQty: &threshold}}, now)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].LotNo != "LOT-LOW" {
		t.Fatalf("List(below_qty=10) = %+v (total %d), want exactly LOT-LOW", rows, total)
	}

	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{BelowQty: &threshold}, now)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.LowStockCount == nil || *summary.LowStockCount != 1 {
		t.Fatalf("LowStockCount = %v, want 1", summary.LowStockCount)
	}

	summaryNoFilter, err := repo.Summary(context.Background(), a.tenantID, Filters{}, now)
	if err != nil {
		t.Fatalf("Summary (no filter): %v", err)
	}
	if summaryNoFilter.LowStockCount != nil {
		t.Errorf("LowStockCount without a below_qty filter = %v, want nil (N/A — no stored threshold to default to)", summaryNoFilter.LowStockCount)
	}
}

// --- Near Expiry / Expired lens ---

func TestRepository_List_ExpiryLenses(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "exp-a")
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-EXPIRED", 10, 0, 0, timePtr(now.AddDate(0, 0, -5)))
	seedBalance(t, tx, a, "LOT-SOON", 10, 0, 0, timePtr(now.AddDate(0, 0, 3)))
	seedBalance(t, tx, a, "LOT-FAR", 10, 0, 0, timePtr(now.AddDate(0, 0, 90)))
	seedBalance(t, tx, a, "LOT-NOEXPIRY", 10, 0, 0, nil)

	repo := NewRepository(tx)

	expiredOnly := Filters{ExpiredOnly: true}
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20, Filters: expiredOnly}, now)
	if err != nil {
		t.Fatalf("List(expired_only): %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].LotNo != "LOT-EXPIRED" {
		t.Fatalf("List(expired_only) = %+v (total %d), want exactly LOT-EXPIRED", rows, total)
	}

	within7 := 7
	nearExpiry := Filters{ExpiringWithinDays: &within7}
	rows, total, err = repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20, Filters: nearExpiry}, now)
	if err != nil {
		t.Fatalf("List(expiring_within_days=7): %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.LotNo] = true
	}
	if total != 2 || !got["LOT-EXPIRED"] || !got["LOT-SOON"] {
		t.Fatalf("List(expiring_within_days=7) = %+v (total %d), want LOT-EXPIRED and LOT-SOON (already-expired counts as within the window)", rows, total)
	}
	if got["LOT-FAR"] || got["LOT-NOEXPIRY"] {
		t.Errorf("List(expiring_within_days=7) incorrectly included a far-future or no-expiry lot: %+v", rows)
	}
}

// --- Slow/Non-Moving lens ---

func TestRepository_List_InactiveDays_NonMovingLens(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "stale-a")
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-STALE", 10, 0, 0, nil)
	seedTransaction(t, tx, a, "LOT-STALE", inventory.TransactionReceive, 10, now.AddDate(0, 0, -100))

	seedBalance(t, tx, a, "LOT-RECENT", 10, 0, 0, nil)
	seedTransaction(t, tx, a, "LOT-RECENT", inventory.TransactionReceive, 10, now.AddDate(0, 0, -1))

	seedBalance(t, tx, a, "LOT-NEVERMOVED", 10, 0, 0, nil) // no transaction at all

	repo := NewRepository(tx)
	days := 30
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20, Filters: Filters{InactiveDays: &days}}, now)
	if err != nil {
		t.Fatalf("List(inactive_days=30): %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.LotNo] = true
	}
	if total != 2 || !got["LOT-STALE"] || !got["LOT-NEVERMOVED"] {
		t.Fatalf("List(inactive_days=30) = %+v (total %d), want LOT-STALE and LOT-NEVERMOVED (null last_movement also counts as inactive)", rows, total)
	}
	if got["LOT-RECENT"] {
		t.Errorf("List(inactive_days=30) incorrectly included a recently-moved lot: %+v", rows)
	}
}

// --- Stock Movement: signed quantity, separate grain ---

func TestRepository_ListMovements_SignedQuantityAndTenantIsolation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "mv-a")
	b := newFixture(t, tx, "mv-b")
	now := time.Now().UTC()

	seedTransaction(t, tx, a, "LOT-RCV", inventory.TransactionReceive, 50, now)
	seedTransaction(t, tx, a, "LOT-PICK", inventory.TransactionPick, -20, now)
	seedTransaction(t, tx, b, "LOT-OTHER", inventory.TransactionReceive, 999, now)

	repo := NewRepository(tx)
	rows, total, err := repo.ListMovements(context.Background(), a.tenantID, MovementListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListMovements: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (tenant B's row must not leak in)", total)
	}
	byType := map[string]float64{}
	for _, r := range rows {
		byType[r.TransactionType] = r.Quantity
	}
	if byType["RECEIVE"] != 50 {
		t.Errorf("RECEIVE quantity = %v, want +50", byType["RECEIVE"])
	}
	if byType["PICK"] != -20 {
		t.Errorf("PICK quantity = %v, want -20 (signed debit)", byType["PICK"])
	}
}

// --- Historical As-of Balance ---

func TestRepository_ListAsOf_ReconstructsBalanceAtCutoff(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "asof-a")
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -5)

	// Before cutoff: +100 receive, -30 pick -> balance at cutoff should be 70.
	seedTransaction(t, tx, a, "LOT-ASOF", inventory.TransactionReceive, 100, now.AddDate(0, 0, -10))
	seedTransaction(t, tx, a, "LOT-ASOF", inventory.TransactionPick, -30, now.AddDate(0, 0, -8))
	// After cutoff: must NOT be included in the as-of reconstruction.
	seedTransaction(t, tx, a, "LOT-ASOF", inventory.TransactionPick, -50, now.AddDate(0, 0, -1))

	repo := NewRepository(tx)
	rows, total, err := repo.ListAsOf(context.Background(), a.tenantID, AsOfListParams{
		AsOfFilters: AsOfFilters{AsOf: cutoff}, Page: 1, Limit: 20,
	})
	if err != nil {
		t.Fatalf("ListAsOf: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("ListAsOf = %+v (total %d), want exactly 1 row", rows, total)
	}
	if rows[0].AsOfQuantity != 70 {
		t.Errorf("AsOfQuantity = %v, want 70 (100 - 30, excluding the -50 after cutoff)", rows[0].AsOfQuantity)
	}
}
