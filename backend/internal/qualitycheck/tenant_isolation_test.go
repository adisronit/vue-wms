// Phase 31B — Quality Control tenant isolation. Mirrors internal/
// receiving/tenant_isolation_test.go's own shape (fresh tenant per
// test, real seeded rows via the real product/location/warehouse/
// inventory/receiving repositories, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on inspection/item lookup, cross-tenant ownership at Create, the
// mandatory PASS/HOLD/REJECT partial-disposition scenario, reinspection
// of a HELD remainder, idempotency, rollback atomicity, and concurrent
// release/reject safety.
package qualitycheck

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/receiving"
	"wms-backend/internal/warehouse"
)

type qcFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newQCFixture(t *testing.T, tx *gorm.DB, label string) qcFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"qc-iso-"+label, "QC Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newQCFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newQCFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: true,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newQCFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newQCFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newQCFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newQCFixture(%s): seed location: %v", label, err)
	}

	return qcFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

// newQCServiceAndRepo returns a qualitycheck Service/Repository bound
// to tx, plus the receiving Service needed to seed a real receipt with
// real held stock (QC's own fixtures never write directly to the
// inventory/receipt_item_splits tables — only through the real
// Receiving Confirm flow, so held_qty_snapshot and the live hold
// balance this package reads are both genuine, not fabricated).
func newQCServiceAndRepo(tx *gorm.DB) (*Service, *Repository, *receiving.Service) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	receivingRepo := receiving.NewRepository(tx)
	receivingSvc := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, nil, invSvc)

	return NewService(repo, warehouseRepo, invSvc), repo, receivingSvc
}

// seedHeldReceipt drives a real Receiving Create->Confirm for qty units
// of f.productID (QCRequired=true) at f.locationID, producing a real
// receipt_item_splits row and a real inventory balance with
// HoldQuantity == qty, Quantity == qty. Returns the new receipt's ID.
func seedHeldReceipt(t *testing.T, receivingSvc *receiving.Service, f qcFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := receivingSvc.Create(context.Background(), f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.locationID,
		Items: []receiving.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, ExpectedQty: qty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("seedHeldReceipt: Create: %v", err)
	}
	itemID := resp.Items[0].ID

	_, err = receivingSvc.Confirm(context.Background(), f.tenantID, resp.ID, receiving.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo,
		Items:          []receiving.ConfirmItemRequest{{ItemID: itemID, Quantity: qty, LotNo: lotNo}},
	}, 1)
	if err != nil {
		t.Fatalf("seedHeldReceipt: Confirm: %v", err)
	}
	return resp.ID
}

// --- IDOR ---

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCFixture(t, tx, "get-a")
	b := newQCFixture(t, tx, "get-b")

	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, b, "LOT-B", 100)

	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: b.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Get(context.Background(), a.tenantID, resp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's inspection) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCFixture(t, tx, "list-a")
	b := newQCFixture(t, tx, "list-b")

	svc, repo, recvSvc := newQCServiceAndRepo(tx)
	rA := seedHeldReceipt(t, recvSvc, a, "LOT-A", 10)
	rB := seedHeldReceipt(t, recvSvc, b, "LOT-B", 10)
	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{ReceiptID: rA, WarehouseID: a.warehouseID}, 1); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := svc.Create(context.Background(), b.tenantID, CreateRequest{ReceiptID: rB, WarehouseID: b.warehouseID}, 1); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	rows, total, err := repo.ListInspections(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListInspections: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("ListInspections(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's inspection)", len(rows), total)
	}
}

func TestService_Create_RejectsCrossTenantReceipt(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCFixture(t, tx, "cr-a")
	b := newQCFixture(t, tx, "cr-b")

	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, b, "LOT-B", 10)

	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: a.warehouseID}, 1)
	if !errors.Is(err, ErrReceiptNotFound) {
		t.Fatalf("Create(tenantA, tenantB's receipt) = %v, want ErrReceiptNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCFixture(t, tx, "cw-a")
	b := newQCFixture(t, tx, "cw-b")

	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, a, "LOT-A", 10)

	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: b.warehouseID}, 1)
	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("Create(tenantA, tenantB's warehouse_id) = %v, want ErrWarehouseNotFound", err)
	}
}

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCFixture(t, tx, "confirm-cross-a")
	b := newQCFixture(t, tx, "confirm-cross-b")

	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: b.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
		Dispositions:   []ConfirmDispositionRequest{{ItemID: resp.Items[0].ID, Disposition: DispositionPass, Quantity: 10}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's inspection) = %v, want ErrNotFound", err)
	}

	// Confirming as the real owner must still work after the rejected
	// cross-tenant attempt — proves it left no residue.
	got, err := svc.Get(context.Background(), b.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("inspection status after rejected cross-tenant Confirm = %v, want still PENDING", got.Status)
	}
}

// --- Mandatory scenario: receive 100 -> PASS 95 / HOLD 3 / REJECT 2 ---

func TestService_Confirm_MandatoryPartialDispositionScenario(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCFixture(t, tx, "mandatory")
	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-M", 100)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.RemainingHeldQty != 100 {
		t.Fatalf("RemainingHeldQty at creation = %v, want 100", item.RemainingHeldQty)
	}

	confirmed, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-mandatory-1",
		Dispositions: []ConfirmDispositionRequest{
			{ItemID: item.ID, Disposition: DispositionPass, Quantity: 95},
			{ItemID: item.ID, Disposition: DispositionHold, Quantity: 3},
			{ItemID: item.ID, Disposition: DispositionReject, Quantity: 2},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(PASS 95 / HOLD 3 / REJECT 2): %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.locationID, "LOT-M")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.Quantity != 98 {
		t.Errorf("Quantity = %v, want 98 (100 received - 2 rejected)", bal.Quantity)
	}
	if bal.HoldQuantity != 3 {
		t.Errorf("HoldQuantity = %v, want 3 (100 held - 95 passed - 2 rejected)", bal.HoldQuantity)
	}
	if bal.ReservedQuantity != 0 {
		t.Errorf("ReservedQuantity = %v, want 0", bal.ReservedQuantity)
	}
	if bal.Available() != 95 {
		t.Errorf("Available() = %v, want 95", bal.Available())
	}

	// The inspection item must reflect 3 still remaining (HOLD doesn't
	// release anything) and the header must not be COMPLETED yet.
	updatedItem := confirmed.Items[0]
	if updatedItem.RemainingHeldQty != 3 {
		t.Errorf("RemainingHeldQty after mandatory Confirm = %v, want 3", updatedItem.RemainingHeldQty)
	}
	if updatedItem.Status != ItemStatusInProgress {
		t.Errorf("item status = %v, want IN_PROGRESS (3 units still held)", updatedItem.Status)
	}
	if confirmed.Status != StatusInProgress {
		t.Errorf("inspection status = %v, want IN_PROGRESS", confirmed.Status)
	}
	if len(updatedItem.Dispositions) != 3 {
		t.Fatalf("len(Dispositions) = %d, want 3 (PASS, HOLD, REJECT each recorded)", len(updatedItem.Dispositions))
	}

	// Reinspection: a later Confirm call can dispose of the remaining 3
	// held units — no special state transition needed, just another
	// Confirm against the same item.
	final, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-mandatory-2",
		Dispositions:   []ConfirmDispositionRequest{{ItemID: item.ID, Disposition: DispositionPass, Quantity: 3}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(reinspection PASS 3): %v", err)
	}
	if final.Items[0].RemainingHeldQty != 0 {
		t.Errorf("RemainingHeldQty after reinspection = %v, want 0", final.Items[0].RemainingHeldQty)
	}
	if final.Items[0].Status != ItemStatusCompleted {
		t.Errorf("item status after reinspection = %v, want COMPLETED", final.Items[0].Status)
	}
	if final.Status != StatusCompleted {
		t.Errorf("inspection status after reinspection = %v, want COMPLETED", final.Status)
	}

	bal2, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.locationID, "LOT-M")
	if err != nil {
		t.Fatalf("FindBalance after reinspection: %v", err)
	}
	if bal2.Quantity != 98 || bal2.HoldQuantity != 0 || bal2.Available() != 98 {
		t.Errorf("final balance = quantity=%v hold=%v available=%v, want quantity=98 hold=0 available=98",
			bal2.Quantity, bal2.HoldQuantity, bal2.Available())
	}
}

// --- Idempotency ---

func TestService_Confirm_DuplicateIdempotencyKey_DoesNotDoubleRelease(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCFixture(t, tx, "idem")
	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-I", 50)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := resp.Items[0].ID

	req := ConfirmRequest{
		IdempotencyKey: "key-idem-1",
		Dispositions:   []ConfirmDispositionRequest{{ItemID: itemID, Disposition: DispositionPass, Quantity: 20}},
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, req, 1); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, req, 1); err != nil {
		t.Fatalf("duplicate Confirm: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.locationID, "LOT-I")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.HoldQuantity != 30 {
		t.Errorf("HoldQuantity after duplicate Confirm = %v, want 30 (50-20, must not double-release)", bal.HoldQuantity)
	}
}

// --- Rollback / atomicity ---

func TestService_Confirm_SecondEntryFailure_RollsBackFirstEntryToo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCFixture(t, tx, "rollback")
	svc, _, recvSvc := newQCServiceAndRepo(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-R", 10)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := resp.Items[0].ID

	// First entry (PASS 5) would succeed on its own; the second entry
	// tries to release/reject more than remains (only 5 left of the 10
	// held after the first entry), forcing ErrHoldUnderflow and rolling
	// back the whole transaction, including the first entry's already-
	// applied ReleaseHoldTx.
	_, err = svc.Confirm(context.Background(), f.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Dispositions: []ConfirmDispositionRequest{
			{ItemID: itemID, Disposition: DispositionPass, Quantity: 5},
			{ItemID: itemID, Disposition: DispositionReject, Quantity: 999},
		},
	}, 1)
	if !errors.Is(err, inventory.ErrHoldUnderflow) {
		t.Fatalf("Confirm(second entry over-releases) = %v, want inventory.ErrHoldUnderflow", err)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.locationID, "LOT-R")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.HoldQuantity != 10 {
		t.Errorf("HoldQuantity after rolled-back Confirm = %v, want still 10 (first entry's PASS must have rolled back too)", bal.HoldQuantity)
	}
}

// --- Concurrency: AdjustHold's guarded UPDATE prevents over-release ---

// TestInventory_ConcurrentReleaseHold_NeverOverReleases proves the same
// concurrency guarantee Phase 30 Batch 1 established for AdjustBalance/
// AdjustReservation, now for AdjustHold: N concurrent PASS releases
// against the same held balance can never together release more than
// was actually held, regardless of how many of them "think" they
// should succeed. Deliberately does NOT use testsupport.BeginRollbackTx
// — a single *sql.Tx is not safe for concurrent use — so this uses the
// real pooled *gorm.DB with explicit, FK-ordered cleanup instead of
// rollback.
func TestInventory_ConcurrentReleaseHold_NeverOverReleases(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('qc-concurrent', 'QC Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM qc_dispositions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM qc_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM qc_inspection_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM qc_inspections WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipt_item_splits WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipt_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipt_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipts WHERE tenant_id = ?", tenantID)
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
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-QCONC', 'Category QConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-QCONC", Name: "Concurrent QC Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: true}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-QCONC', 'Warehouse QConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-QCONC', 'Zone QConc', 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-QCONC", Name: "Location QConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	// Seed a held balance of exactly 20 directly (bypassing Receiving —
	// this test only needs a real inventory row, not a real receipt).
	inv := inventory.Inventory{TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-QCONC", Quantity: 20, HoldQuantity: 20}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	invSvc := inventory.NewService(inventory.NewRepository(db), product.NewRepository(db), location.NewRepository(db))

	const attempts = 20
	var wg sync.WaitGroup
	var successCount int64
	var mu sync.Mutex
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.Transaction(func(tx *gorm.DB) error {
				txRepo := inventory.NewRepository(tx)
				_, err := invSvc.ReleaseHoldTx(context.Background(), txRepo, inventory.ReserveRequest{
					TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-QCONC", Quantity: 1,
				})
				return err
			})
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			} else if !errors.Is(err, inventory.ErrHoldUnderflow) {
				t.Errorf("ReleaseHoldTx: unexpected error %v", err)
			}
		}()
	}
	wg.Wait()

	if successCount != 20 {
		t.Errorf("successCount = %d, want exactly 20 (releasing 1 unit 20 times against a 20-unit hold)", successCount)
	}

	invRepo := inventory.NewRepository(db)
	bal, err := invRepo.FindBalance(context.Background(), tenantID, productID, locationID, "LOT-QCONC")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.HoldQuantity != 0 {
		t.Errorf("final HoldQuantity = %v, want exactly 0 (no over-release, no lost update)", bal.HoldQuantity)
	}
	if bal.Quantity != 20 {
		t.Errorf("final Quantity = %v, want unchanged 20 (PASS never touches Quantity)", bal.Quantity)
	}
}
