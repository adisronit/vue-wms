// Phase 31C — Putaway tenant isolation. Mirrors internal/receiving's
// and internal/qualitycheck's own Phase 31A/31B shape (fresh tenant per
// test, real seeded rows via the real product/location/warehouse/
// inventory/receiving repositories, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on task/item lookup and location resolution, cross-tenant/cross-
// warehouse rejection, partial putaway across multiple destinations, no
// double movement, and the critical new safety property — QC-held
// stock can never be moved via standard putaway.
package putaway

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/receiving"
	"wms-backend/internal/warehouse"
)

type putawayFixture struct {
	tenantID         int64
	productID        int64
	warehouseID      int64
	zoneID           int64
	stagingLocID     int64
	storageLocID     int64
	storageLocID2    int64
	pickingZoneLocID int64
}

func newPutawayFixture(t *testing.T, tx *gorm.DB, label string, qcRequired bool) putawayFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"pa-iso-"+label, "Putaway Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: qcRequired,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed warehouse: %v", label, err)
	}

	var receivingZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "ZR-"+label, "Receiving Zone "+label,
	).Scan(&receivingZoneID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed receiving zone: %v", label, err)
	}
	staging := location.Location{TenantID: tenantID, ZoneID: receivingZoneID, Code: "STG-" + label, Name: "Staging " + label, Status: location.StatusActive}
	if err := tx.Create(&staging).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed staging location: %v", label, err)
	}

	var storageZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "ZS-"+label, "Storage Zone "+label,
	).Scan(&storageZoneID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed storage zone: %v", label, err)
	}
	storage1 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "A-" + label, Name: "Storage A " + label, Status: location.StatusActive}
	if err := tx.Create(&storage1).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed storage location 1: %v", label, err)
	}
	storage2 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "B-" + label, Name: "Storage B " + label, Status: location.StatusActive}
	if err := tx.Create(&storage2).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed storage location 2: %v", label, err)
	}

	var pickingZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'PICKING', 'ACTIVE')",
		tenantID, warehouseID, "ZP-"+label, "Picking Zone "+label,
	).Scan(&pickingZoneID).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed picking zone: %v", label, err)
	}
	pickingLoc := location.Location{TenantID: tenantID, ZoneID: pickingZoneID, Code: "P-" + label, Name: "Picking " + label, Status: location.StatusActive}
	if err := tx.Create(&pickingLoc).Error; err != nil {
		t.Fatalf("newPutawayFixture(%s): seed picking location: %v", label, err)
	}

	return putawayFixture{
		tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, zoneID: storageZoneID,
		stagingLocID: staging.ID, storageLocID: storage1.ID, storageLocID2: storage2.ID,
		pickingZoneLocID: pickingLoc.ID,
	}
}

func newPutawayServiceAndRepo(tx *gorm.DB) (*Service, *Repository, *receiving.Service) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	receivingRepo := receiving.NewRepository(tx)
	receivingSvc := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, nil, invSvc)

	return NewService(repo, productRepo, warehouseRepo, receivingSvc, invSvc), repo, receivingSvc
}

// seedReceipt drives a real Receiving Create->Confirm for qty units of
// f.productID at f.stagingLocID, producing a real receipt_items row
// (and, for a QCRequired product, a real held inventory balance).
// Returns the receipt ID and its one item's ID.
func seedReceipt(t *testing.T, receivingSvc *receiving.Service, f putawayFixture, lotNo string, qty float64) (receiptID, itemID int64) {
	t.Helper()
	resp, err := receivingSvc.Create(context.Background(), f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.stagingLocID,
		Items: []receiving.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.stagingLocID, ExpectedQty: qty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("seedReceipt: Create: %v", err)
	}
	itemID = resp.Items[0].ID

	_, err = receivingSvc.Confirm(context.Background(), f.tenantID, resp.ID, receiving.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo,
		Items:          []receiving.ConfirmItemRequest{{ItemID: itemID, Quantity: qty, LotNo: lotNo}},
	}, 1)
	if err != nil {
		t.Fatalf("seedReceipt: Confirm: %v", err)
	}
	return resp.ID, itemID
}

// createTask is a small helper that runs Service.Create end-to-end for
// one line sourced from f.stagingLocID -> f.storageLocID.
func createTask(t *testing.T, svc *Service, f putawayFixture, lotNo string, qty float64, receiptItemID *int64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ReceiptItemID: receiptItemID, ProductID: f.productID, LotNo: lotNo,
				SourceLocationID: f.stagingLocID, DestinationLocationID: f.storageLocID,
				RequestedQty: qty, UOM: "PCS",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createTask: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR ---

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPutawayFixture(t, tx, "get-a", false)
	b := newPutawayFixture(t, tx, "get-b", false)

	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, b, "LOT-B", 100)
	taskID := createTask(t, svc, b, "LOT-B", 100, &itemID)

	if _, err := svc.Get(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPutawayFixture(t, tx, "list-a", false)
	b := newPutawayFixture(t, tx, "list-b", false)

	svc, repo, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemA := seedReceipt(t, recvSvc, a, "LOT-A", 10)
	_, itemB := seedReceipt(t, recvSvc, b, "LOT-B", 10)
	createTask(t, svc, a, "LOT-A", 10, &itemA)
	createTask(t, svc, b, "LOT-B", 10, &itemB)

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's task)", len(rows), total)
	}
}

func TestRepository_ResolveLocation_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPutawayFixture(t, tx, "loc-a", false)
	b := newPutawayFixture(t, tx, "loc-b", false)

	repo := NewRepository(tx)
	if _, err := repo.resolveLocation(context.Background(), a.tenantID, b.storageLocID); !errors.Is(err, ErrLocationNotFound) {
		t.Errorf("resolveLocation(tenantA, tenantB's location) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPutawayFixture(t, tx, "confirm-cross-a", false)
	b := newPutawayFixture(t, tx, "confirm-cross-b", false)

	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, b, "LOT-B", 10)
	taskID := createTask(t, svc, b, "LOT-B", 10, &itemID)

	task, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, err = svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 10}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

// --- Location validation ---

func TestService_CreateItem_RejectsNonStorageDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "nonstorage", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-NS", 10)

	_, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ReceiptItemID: &itemID, ProductID: f.productID, LotNo: "LOT-NS",
				SourceLocationID: f.stagingLocID, DestinationLocationID: f.pickingZoneLocID,
				RequestedQty: 10, UOM: "PCS",
			},
		},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(destination in PICKING zone) = %v, want ErrValidation (must be STORAGE)", err)
	}
}

// TestService_CreateItem_RejectsCrossTenantDestination proves
// resolveLocation's own tenant scoping is what actually enforces the
// cross-warehouse/cross-tenant boundary here: tenant B's location is
// invisible to tenant A's lookup entirely (ErrLocationNotFound), never
// reaching the separate ErrCrossWarehouse check at all — the IDOR case
// is closed one layer earlier than the warehouse-mismatch case.
func TestService_CreateItem_RejectsCrossTenantDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPutawayFixture(t, tx, "cw-a", false)
	b := newPutawayFixture(t, tx, "cw-b", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, a, "LOT-A", 10)

	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{
			{
				ReceiptItemID: &itemID, ProductID: a.productID, LotNo: "LOT-A",
				SourceLocationID: a.stagingLocID, DestinationLocationID: b.storageLocID,
				RequestedQty: 10, UOM: "PCS",
			},
		},
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(destination_location_id owned by another tenant) = %v, want ErrLocationNotFound", err)
	}
}

// --- The critical new safety property: QC-held stock can never be
// moved via standard putaway. ---

func TestService_Confirm_RejectsMovingHeldStock(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "held", true) // QCRequired product
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-HELD", 100)

	// Confirms Receiving posted the whole 100 into Hold, leaving nothing
	// available — the exact pre-condition this test needs.
	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-HELD")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.HoldQuantity != 100 || bal.Available() != 0 {
		t.Fatalf("precondition failed: balance = %+v, want HoldQuantity=100 Available=0", bal)
	}

	taskID := createTask(t, svc, f, "LOT-HELD", 100, &itemID)
	task, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, err = svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-held-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 100}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(moving fully-held stock) = %v, want ErrValidation (insufficient available stock)", err)
	}

	// Nothing may have moved: both the source and destination balances
	// must be exactly what they were before the rejected attempt.
	balAfter, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-HELD")
	if err != nil {
		t.Fatalf("FindBalance after rejected Confirm: %v", err)
	}
	if balAfter.Quantity != 100 || balAfter.HoldQuantity != 100 {
		t.Errorf("source balance after rejected Confirm = %+v, want unchanged Quantity=100 HoldQuantity=100", balAfter)
	}
	if _, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-HELD"); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("destination balance after rejected Confirm = %v, want inventory.ErrNotFound (nothing moved)", err)
	}
}

// TestService_Confirm_PartialHold_OnlyMovesAvailablePortion proves the
// boundary case: with 100 received and 70 held (30 available), moving
// exactly 30 succeeds, and moving even 1 more than that fails.
func TestService_Confirm_PartialHold_OnlyMovesAvailablePortion(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "partial-held", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-PH", 100)

	// Manually hold 70 of the 100 (simulating a QC inspection already in
	// progress on this lot) via the real guarded AdjustHold path.
	invRepo := inventory.NewRepository(tx)
	if _, err := invRepo.AdjustHold(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-PH", 70); err != nil {
		t.Fatalf("AdjustHold: %v", err)
	}

	taskID := createTask(t, svc, f, "LOT-PH", 30, &itemID)
	task, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	resp, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-ph-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 30}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(moving exactly the available 30): %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("task status = %v, want COMPLETED", resp.Status)
	}

	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-PH")
	if err != nil {
		t.Fatalf("FindBalance(destination): %v", err)
	}
	if dstBal.Quantity != 30 {
		t.Errorf("destination Quantity = %v, want 30", dstBal.Quantity)
	}
	srcBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-PH")
	if err != nil {
		t.Fatalf("FindBalance(source): %v", err)
	}
	if srcBal.Quantity != 70 || srcBal.HoldQuantity != 70 {
		t.Errorf("source balance after moving the available portion = %+v, want Quantity=70 HoldQuantity=70 (all held, none available)", srcBal)
	}
}

// --- Partial putaway across multiple destinations ---

func TestService_Confirm_PartialAcrossMultipleDestinations(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "multi-dest", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-MD", 100)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ReceiptItemID: &itemID, ProductID: f.productID, LotNo: "LOT-MD",
				SourceLocationID: f.stagingLocID, DestinationLocationID: f.storageLocID,
				RequestedQty: 100, UOM: "PCS",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lineItemID := resp.Items[0].ID

	// First confirm: 60 to storageLocID (the line's own default
	// destination).
	if _, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-md-1",
		Items:          []ConfirmItemRequest{{ItemID: lineItemID, Action: "CONFIRMED", Quantity: 60}},
	}, 1); err != nil {
		t.Fatalf("Confirm(60 to location A): %v", err)
	}

	// Second confirm: remaining 35 (of the mandatory-scenario's own
	// 60+35=95 split) redirected to storageLocID2 via
	// destination_location_id override on the same line.
	final, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-md-2",
		Items: []ConfirmItemRequest{
			{ItemID: lineItemID, Action: "CONFIRMED", Quantity: 35, DestinationLocationID: f.storageLocID2},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(35 to location B): %v", err)
	}
	if final.Items[0].MovedQty != 95 {
		t.Errorf("MovedQty = %v, want 95 (60+35)", final.Items[0].MovedQty)
	}

	invRepo := inventory.NewRepository(tx)
	balA, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-MD")
	if err != nil || balA.Quantity != 60 {
		t.Errorf("location A balance = %+v, err=%v, want Quantity=60", balA, err)
	}
	balB, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-MD")
	if err != nil || balB.Quantity != 35 {
		t.Errorf("location B balance = %+v, err=%v, want Quantity=35", balB, err)
	}
	srcBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-MD")
	if err != nil || srcBal.Quantity != 5 {
		t.Errorf("source balance = %+v, err=%v, want Quantity=5 (100-60-35)", srcBal, err)
	}
}

// --- Idempotency / no double movement ---

func TestService_Confirm_DuplicateIdempotencyKey_DoesNotDoubleMove(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "idem", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-I", 50)
	taskID := createTask(t, svc, f, "LOT-I", 50, &itemID)
	task, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	req := ConfirmRequest{
		IdempotencyKey: "key-idem-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 20}},
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, req, 1); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, req, 1); err != nil {
		t.Fatalf("duplicate Confirm: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-I")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if dstBal.Quantity != 20 {
		t.Errorf("destination Quantity after duplicate Confirm = %v, want 20 (must not double-move)", dstBal.Quantity)
	}
}

// --- Rollback / atomicity ---

func TestService_Confirm_InvalidDestination_RollsBackEverything(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newPutawayFixture(t, tx, "rollback", false)
	svc, _, recvSvc := newPutawayServiceAndRepo(tx)
	_, itemID := seedReceipt(t, recvSvc, f, "LOT-RB", 10)
	taskID := createTask(t, svc, f, "LOT-RB", 10, &itemID)
	task, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Redirects the only line to a non-STORAGE destination at confirm
	// time — rejected after the idempotency key is claimed, but before
	// any inventory mutation; the whole transaction (including the
	// confirmation row) must roll back.
	_, err = svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items: []ConfirmItemRequest{
			{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 10, DestinationLocationID: f.pickingZoneLocID},
		},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(invalid destination zone) = %v, want ErrValidation", err)
	}

	updated, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rolled-back Confirm: %v", err)
	}
	if updated.Items[0].MovedQty != 0 {
		t.Errorf("MovedQty after rolled-back Confirm = %v, want 0", updated.Items[0].MovedQty)
	}
	if updated.Status != StatusPending {
		t.Errorf("task status after rolled-back Confirm = %v, want still PENDING", updated.Status)
	}

	// The idempotency key must also have rolled back — retrying the
	// exact same key with a valid destination must succeed, not report
	// a duplicate.
	resp, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Action: "CONFIRMED", Quantity: 10}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(retry with the same key after rollback): %v", err)
	}
	if resp.Items[0].MovedQty != 10 {
		t.Errorf("MovedQty after the successful retry = %v, want 10", resp.Items[0].MovedQty)
	}
}
