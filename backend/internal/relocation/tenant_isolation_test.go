// Phase 32 — Stock Relocation tenant isolation. Mirrors
// internal/putaway's own Phase 31C tenant_isolation_test.go shape
// (fresh tenant per test, real seeded rows via the real product/
// location/warehouse/inventory repositories, testsupport.ConnectDB +
// BeginRollbackTx) but seeds starting stock directly via
// inventory.Service.AddStockTx instead of driving a full Receiving
// Create->Confirm — relocation has no dependency on internal/receiving
// at all, since it moves stock that is already somewhere in the
// warehouse.
package relocation

import (
	"context"
	"errors"
	"math"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
)

type relocationFixture struct {
	tenantID         int64
	productID        int64
	warehouseID      int64
	storageLocID     int64 // source
	storageLocID2    int64 // alternate destination
	pickingZoneLocID int64 // valid destination (PICKING zone)
	receivingLocID   int64 // invalid destination (RECEIVING zone)
}

func newRelocationFixture(t *testing.T, tx *gorm.DB, label string) relocationFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"rl-iso-"+label, "Relocation Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed warehouse: %v", label, err)
	}

	var storageZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "ZS-"+label, "Storage Zone "+label,
	).Scan(&storageZoneID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed storage zone: %v", label, err)
	}
	storage1 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "A-" + label, Name: "Storage A " + label, Status: location.StatusActive}
	if err := tx.Create(&storage1).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed storage location 1: %v", label, err)
	}
	storage2 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "B-" + label, Name: "Storage B " + label, Status: location.StatusActive}
	if err := tx.Create(&storage2).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed storage location 2: %v", label, err)
	}

	var pickingZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'PICKING', 'ACTIVE')",
		tenantID, warehouseID, "ZP-"+label, "Picking Zone "+label,
	).Scan(&pickingZoneID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed picking zone: %v", label, err)
	}
	pickingLoc := location.Location{TenantID: tenantID, ZoneID: pickingZoneID, Code: "P-" + label, Name: "Picking " + label, Status: location.StatusActive}
	if err := tx.Create(&pickingLoc).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed picking location: %v", label, err)
	}

	var receivingZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "ZR-"+label, "Receiving Zone "+label,
	).Scan(&receivingZoneID).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed receiving zone: %v", label, err)
	}
	receivingLoc := location.Location{TenantID: tenantID, ZoneID: receivingZoneID, Code: "R-" + label, Name: "Receiving " + label, Status: location.StatusActive}
	if err := tx.Create(&receivingLoc).Error; err != nil {
		t.Fatalf("newRelocationFixture(%s): seed receiving location: %v", label, err)
	}

	return relocationFixture{
		tenantID: tenantID, productID: p.ID, warehouseID: warehouseID,
		storageLocID: storage1.ID, storageLocID2: storage2.ID,
		pickingZoneLocID: pickingLoc.ID, receivingLocID: receivingLoc.ID,
	}
}

func newRelocationServiceAndRepo(tx *gorm.DB) (*Service, *Repository, *inventory.Service) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	return NewService(repo, productRepo, warehouseRepo, invSvc), repo, invSvc
}

// seedStock posts qty units of f.productID at f.storageLocID (lotNo),
// directly via AddStock — relocation moves stock that is already
// somewhere in the warehouse, so unlike putaway's tests there is no
// Receiving document to drive first.
func seedStock(t *testing.T, invSvc *inventory.Service, f relocationFixture, lotNo string, qty float64) {
	t.Helper()
	_, err := invSvc.AddStock(context.Background(), inventory.StockChangeRequest{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.storageLocID, LotNo: lotNo,
		Quantity: qty, Type: inventory.TransactionReceive,
	})
	if err != nil {
		t.Fatalf("seedStock: AddStock: %v", err)
	}
}

// createTask is a small helper that runs Service.Create end-to-end for
// one line sourced from f.storageLocID -> f.storageLocID2.
func createTask(t *testing.T, svc *Service, f relocationFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ProductID: f.productID, LotNo: lotNo,
				SourceLocationID: f.storageLocID, DestinationLocationID: f.storageLocID2,
				RequestedQty: qty, UOM: "PCS",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createTask: Create: %v", err)
	}
	return resp.ID
}

// readyTask drives Create then Ready, returning the task ID and its
// single line's item ID.
func readyTask(t *testing.T, svc *Service, f relocationFixture, lotNo string, qty float64) (taskID, itemID int64) {
	t.Helper()
	taskID = createTask(t, svc, f, lotNo, qty)
	resp, err := svc.Ready(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("readyTask: Ready: %v", err)
	}
	return taskID, resp.Items[0].ID
}

// --- IDOR ---

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newRelocationFixture(t, tx, "get-a")
	b := newRelocationFixture(t, tx, "get-b")

	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, b, "LOT-B", 100)
	taskID := createTask(t, svc, b, "LOT-B", 100)

	if _, err := svc.Get(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newRelocationFixture(t, tx, "list-a")
	b := newRelocationFixture(t, tx, "list-b")

	svc, repo, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, a, "LOT-A", 10)
	seedStock(t, invSvc, b, "LOT-B", 10)
	createTask(t, svc, a, "LOT-A", 10)
	createTask(t, svc, b, "LOT-B", 10)

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

	a := newRelocationFixture(t, tx, "loc-a")
	b := newRelocationFixture(t, tx, "loc-b")

	repo := NewRepository(tx)
	if _, err := repo.resolveLocation(context.Background(), a.tenantID, b.storageLocID); !errors.Is(err, ErrLocationNotFound) {
		t.Errorf("resolveLocation(tenantA, tenantB's location) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newRelocationFixture(t, tx, "confirm-cross-a")
	b := newRelocationFixture(t, tx, "confirm-cross-b")

	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, b, "LOT-B", 10)
	taskID, itemID := readyTask(t, svc, b, "LOT-B", 10)

	_, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 10}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

// --- Location validation ---

func TestService_CreateItem_RejectsReceivingZoneDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "nonstorage")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-NS", 10)

	_, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ProductID: f.productID, LotNo: "LOT-NS",
				SourceLocationID: f.storageLocID, DestinationLocationID: f.receivingLocID,
				RequestedQty: 10, UOM: "PCS",
			},
		},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(destination in RECEIVING zone) = %v, want ErrValidation (must be STORAGE or PICKING)", err)
	}
}

func TestService_CreateItem_AllowsPickingZoneDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "picking-ok")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-PK", 10)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{
				ProductID: f.productID, LotNo: "LOT-PK",
				SourceLocationID: f.storageLocID, DestinationLocationID: f.pickingZoneLocID,
				RequestedQty: 10, UOM: "PCS",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Create(destination in PICKING zone, replenishing a pick face) = %v, want success", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(resp.Items))
	}
}

// TestService_CreateItem_RejectsCrossTenantDestination proves
// resolveLocation's own tenant scoping is what actually enforces the
// cross-warehouse/cross-tenant boundary here: tenant B's location is
// invisible to tenant A's lookup entirely (ErrLocationNotFound).
func TestService_CreateItem_RejectsCrossTenantDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newRelocationFixture(t, tx, "cw-a")
	b := newRelocationFixture(t, tx, "cw-b")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, a, "LOT-A", 10)

	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{
			{
				ProductID: a.productID, LotNo: "LOT-A",
				SourceLocationID: a.storageLocID, DestinationLocationID: b.storageLocID2,
				RequestedQty: 10, UOM: "PCS",
			},
		},
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(destination_location_id owned by another tenant) = %v, want ErrLocationNotFound", err)
	}
}

// --- Status machine ---

func TestService_Ready_RequiresAtLeastOneItem(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "empty-ready")
	svc, _, _ := newRelocationServiceAndRepo(tx)

	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create (no items): %v", err)
	}

	if _, err := svc.Ready(context.Background(), f.tenantID, resp.ID); !errors.Is(err, ErrEmptyTask) {
		t.Errorf("Ready(task with zero items) = %v, want ErrEmptyTask", err)
	}
}

func TestService_Confirm_BeforeReady_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "before-ready")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-CBR", 10)
	taskID := createTask(t, svc, f, "LOT-CBR", 10)

	task, err := svc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, err = svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cbr-1",
		Items:          []ConfirmItemRequest{{ItemID: task.Items[0].ID, Quantity: 10}},
	}, 1)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Confirm(still DRAFT task) = %v, want ErrInvalidTransition", err)
	}
}

func TestService_Ready_Twice_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "ready-twice")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-RT", 10)
	taskID, _ := readyTask(t, svc, f, "LOT-RT", 10)

	if _, err := svc.Ready(context.Background(), f.tenantID, taskID); !errors.Is(err, ErrNotReadyable) {
		t.Errorf("Ready(already READY task) = %v, want ErrNotReadyable", err)
	}
}

// --- Held/reserved stock protection ---

func TestService_Confirm_RejectsMovingHeldStock(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "held")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)

	// Post 100 units, all into Hold — none available.
	invRepo := inventory.NewRepository(tx)
	_, err := invSvc.AddStock(context.Background(), inventory.StockChangeRequest{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.storageLocID, LotNo: "LOT-HELD",
		Quantity: 100, HoldQuantity: 100, Type: inventory.TransactionReceive,
	})
	if err != nil {
		t.Fatalf("AddStock(fully held): %v", err)
	}

	taskID, itemID := readyTask(t, svc, f, "LOT-HELD", 100)

	_, err = svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-held-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 100}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(moving fully-held stock) = %v, want ErrValidation (insufficient available stock)", err)
	}

	balAfter, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-HELD")
	if err != nil {
		t.Fatalf("FindBalance after rejected Confirm: %v", err)
	}
	if balAfter.Quantity != 100 || balAfter.HoldQuantity != 100 {
		t.Errorf("source balance after rejected Confirm = %+v, want unchanged Quantity=100 HoldQuantity=100", balAfter)
	}
	if _, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-HELD"); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("destination balance after rejected Confirm = %v, want inventory.ErrNotFound (nothing moved)", err)
	}
}

// TestService_Confirm_PartialHold_OnlyMovesAvailablePortion proves the
// boundary case: with 100 on hand and 70 held (30 available), moving
// exactly 30 succeeds, matching putaway's own equivalent test.
func TestService_Confirm_PartialHold_OnlyMovesAvailablePortion(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "partial-held")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-PH", 100)

	invRepo := inventory.NewRepository(tx)
	if _, err := invRepo.AdjustHold(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-PH", 70); err != nil {
		t.Fatalf("AdjustHold: %v", err)
	}

	taskID, itemID := readyTask(t, svc, f, "LOT-PH", 30)

	resp, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-ph-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 30}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(moving exactly the available 30): %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("task status = %v, want COMPLETED", resp.Status)
	}

	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-PH")
	if err != nil {
		t.Fatalf("FindBalance(destination): %v", err)
	}
	if dstBal.Quantity != 30 {
		t.Errorf("destination Quantity = %v, want 30", dstBal.Quantity)
	}
}

// --- Partial relocation across multiple destinations ---

func TestService_Confirm_PartialAcrossMultipleDestinations(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "multi-dest")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-MD", 100)

	taskID, itemID := readyTask(t, svc, f, "LOT-MD", 100)

	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-md-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 60}},
	}, 1); err != nil {
		t.Fatalf("Confirm(60 to location A): %v", err)
	}

	final, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-md-2",
		Items: []ConfirmItemRequest{
			{ItemID: itemID, Quantity: 35, DestinationLocationID: f.pickingZoneLocID},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(35 to picking location): %v", err)
	}
	if final.Items[0].MovedQty != 95 {
		t.Errorf("MovedQty = %v, want 95 (60+35)", final.Items[0].MovedQty)
	}
	if final.Status != StatusInProgress {
		t.Errorf("task status = %v, want IN_PROGRESS (5 of 100 still outstanding)", final.Status)
	}

	invRepo := inventory.NewRepository(tx)
	balA, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-MD")
	if err != nil || balA.Quantity != 60 {
		t.Errorf("location A (storageLocID2) balance = %+v, err=%v, want Quantity=60", balA, err)
	}
	balB, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.pickingZoneLocID, "LOT-MD")
	if err != nil || balB.Quantity != 35 {
		t.Errorf("location B (pickingZoneLocID) balance = %+v, err=%v, want Quantity=35", balB, err)
	}
	srcBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-MD")
	if err != nil || srcBal.Quantity != 5 {
		t.Errorf("source balance = %+v, err=%v, want Quantity=5 (100-60-35)", srcBal, err)
	}
}

// --- Idempotency / no double movement ---

func TestService_Confirm_DuplicateIdempotencyKey_DoesNotDoubleMove(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "idem")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-I", 50)
	taskID, itemID := readyTask(t, svc, f, "LOT-I", 50)

	req := ConfirmRequest{
		IdempotencyKey: "key-idem-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 20}},
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, req, 1); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, req, 1); err != nil {
		t.Fatalf("duplicate Confirm: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-I")
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

	f := newRelocationFixture(t, tx, "rollback")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-RB", 10)
	taskID, itemID := readyTask(t, svc, f, "LOT-RB", 10)

	// Redirects the only line to a RECEIVING-zone destination at confirm
	// time — rejected after the idempotency key is claimed, but before
	// any inventory mutation; the whole transaction (including the
	// confirmation row) must roll back.
	_, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items: []ConfirmItemRequest{
			{ItemID: itemID, Quantity: 10, DestinationLocationID: f.receivingLocID},
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
	if updated.Status != StatusReady {
		t.Errorf("task status after rolled-back Confirm = %v, want still READY", updated.Status)
	}

	// The idempotency key must also have rolled back — retrying the
	// exact same key with a valid destination must succeed, not report
	// a duplicate.
	resp, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 10}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(retry with the same key after rollback): %v", err)
	}
	if resp.Items[0].MovedQty != 10 {
		t.Errorf("MovedQty after the successful retry = %v, want 10", resp.Items[0].MovedQty)
	}
}

// --- Cancel ---

// TestService_Cancel_WhileInProgress_KeepsAlreadyMovedStock proves the
// one deliberate behavioral difference from putaway.Service.Cancel: an
// IN_PROGRESS relocation task (some lines already partially moved) may
// still be cancelled — the already-moved stock stays exactly where
// Confirm put it; only the header transitions to CANCELLED, abandoning
// the unconfirmed remainder. See Service.Cancel's own doc for why.
func TestService_Cancel_WhileInProgress_KeepsAlreadyMovedStock(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "cancel-inprogress")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-CIP", 100)
	taskID, itemID := readyTask(t, svc, f, "LOT-CIP", 100)

	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cip-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 40}},
	}, 1); err != nil {
		t.Fatalf("Confirm(partial 40): %v", err)
	}

	resp, err := svc.Cancel(context.Background(), f.tenantID, taskID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(IN_PROGRESS task with 40 already moved) = %v, want success", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("Status = %v, want CANCELLED", resp.Status)
	}
	if resp.Items[0].MovedQty != 40 {
		t.Errorf("MovedQty after Cancel = %v, want unchanged 40 (already-moved stock is never rolled back)", resp.Items[0].MovedQty)
	}

	invRepo := inventory.NewRepository(tx)
	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-CIP")
	if err != nil || dstBal.Quantity != 40 {
		t.Errorf("destination balance = %+v, err=%v, want Quantity=40 (unaffected by Cancel)", dstBal, err)
	}

	// Cancelling an already-CANCELLED task must be rejected, not a
	// silent no-op.
	if _, err := svc.Cancel(context.Background(), f.tenantID, taskID, CancelRequest{}, 1); !errors.Is(err, ErrNotCancellable) {
		t.Errorf("Cancel(already CANCELLED task) = %v, want ErrNotCancellable", err)
	}
}

func TestService_Cancel_AlreadyCompleted_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newRelocationFixture(t, tx, "cancel-completed")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-CC", 10)
	taskID, itemID := readyTask(t, svc, f, "LOT-CC", 10)

	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cc-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 10}},
	}, 1); err != nil {
		t.Fatalf("Confirm(full 10): %v", err)
	}

	if _, err := svc.Cancel(context.Background(), f.tenantID, taskID, CancelRequest{}, 1); !errors.Is(err, ErrNotCancellable) {
		t.Errorf("Cancel(already COMPLETED task) = %v, want ErrNotCancellable", err)
	}
}

// --- Concurrency ---

// TestService_Confirm_ConcurrentConfirmations_NeverOversellsSource runs
// 6 concurrent Confirm calls of 20 units each (120 requested) against a
// source balance of only 100 available, each against its own real
// transaction on the shared disposable database (unlike every test
// above, which shares one BeginRollbackTx transaction — a meaningful
// concurrency test needs genuinely independent connections/transactions
// racing against committed data).
//
// The invariant that must hold regardless of how many calls actually
// succeed is: destination balance == successes * 20, source balance ==
// 100 - that, and it is never possible for more than 100 total to have
// moved — enforced by the atomic guarded UPDATE in
// inventory.Repository.AdjustAvailableBalance (no application-level
// locking anywhere in this package).
//
// A Confirm transaction here touches FOUR shared rows (the task header,
// the single task_item line, and both the source and destination
// inventory balances), all contended by every one of the 6 goroutines
// at once — real SQL Server deadlocks under that much contention are
// expected, not a bug: a deadlocked transaction is fully rolled back
// (no partial effect, confirmed by the balance assertions below), so it
// never violates correctness, only availability — exactly the same
// "occasionally a deadlock victim under heavy contention" property this
// codebase already documents for
// TestService_ReserveStockTx_ConcurrentReservations_NeverExceedOnHand.
// Neither this package nor internal/inventory implements deadlock
// retry anywhere; a caller that hits one is expected to retry the HTTP
// request with the same idempotency_key, the same as any other
// transient DB error.
func TestService_Confirm_ConcurrentConfirmations_NeverOversellsSource(t *testing.T) {
	db := testsupport.ConnectDB(t)

	tx := testsupport.BeginRollbackTx(t, db)
	f := newRelocationFixture(t, tx, "concurrent")
	svc, _, invSvc := newRelocationServiceAndRepo(tx)
	seedStock(t, invSvc, f, "LOT-CC", 100)
	taskID, itemID := readyTask(t, svc, f, "LOT-CC", 100)
	// Commit this setup for real — goroutines below each open their own
	// transaction against the shared connection pool (db), which cannot
	// see tx's uncommitted rows otherwise.
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit fixture setup: %v", err)
	}
	t.Cleanup(func() {
		cleanup := db.Begin()
		cleanup.Exec("DELETE FROM relocation_confirmations WHERE relocation_task_id = ?", taskID)
		cleanup.Exec("DELETE FROM relocation_task_items WHERE relocation_task_id = ?", taskID)
		cleanup.Exec("DELETE FROM relocation_tasks WHERE id = ?", taskID)
		cleanup.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM inventory WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM locations WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM zones WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM warehouses WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM products WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM product_categories WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM tenants WHERE id = ?", f.tenantID)
		_ = cleanup.Commit().Error
	})

	// A fresh Service bound directly to db (the connection pool), not
	// tx — each Confirm call below opens its own independent transaction
	// via Repository.Transaction.
	liveSvc, _, _ := newRelocationServiceAndRepo(db)

	const goroutines = 6
	const perCall = 20.0
	results := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			_, err := liveSvc.Confirm(context.Background(), f.tenantID, taskID, ConfirmRequest{
				IdempotencyKey: "key-concurrent-" + string(rune('A'+i)),
				Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: perCall}},
			}, 1)
			results <- err
		}()
	}

	// Deliberately NOT counting successes from each goroutine's returned
	// error: Confirm's mutating transaction commits, THEN (outside that
	// transaction) calls Service.Get to build its response — if that
	// trailing Get hits its own transient error under this much
	// contention, Confirm returns an error to this test even though the
	// stock move it reports on already committed durably. (A real HTTP
	// caller recovers from this by retrying with the same
	// idempotency_key — InsertConfirmation's row already exists, so the
	// retry replays the already-correct state via the ErrDuplicateIdempotencyKey
	// path instead of moving stock twice. This test does not retry, so
	// it must not trust the error return to count "how many actually
	// moved".) Log what each call reported for visibility, then verify
	// the real safety invariant directly from committed DB state.
	for i := 0; i < goroutines; i++ {
		if err := <-results; err != nil {
			t.Logf("Confirm goroutine result: %v (informational only — see this test's own doc)", err)
		}
	}

	invRepo := inventory.NewRepository(db)
	dstBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-CC")
	if err != nil {
		t.Fatalf("FindBalance(destination): %v", err)
	}
	srcBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-CC")
	if err != nil {
		t.Fatalf("FindBalance(source): %v", err)
	}

	moved := dstBal.Quantity
	if moved < perCall {
		t.Errorf("destination Quantity = %v, want at least %v (test is vacuous if nothing moved)", moved, perCall)
	}
	if moved > 100 {
		t.Errorf("destination Quantity = %v, must never exceed the 100 available — oversold!", moved)
	}
	if math.Mod(moved, perCall) != 0 {
		t.Errorf("destination Quantity = %v, want an exact multiple of %v (no double-counting or partial-write corruption)", moved, perCall)
	}
	if srcBal.Quantity < 0 {
		t.Errorf("source Quantity = %v, must never go negative", srcBal.Quantity)
	}
	if srcBal.Quantity+moved != 100 {
		t.Errorf("source(%v) + destination(%v) = %v, want 100 (total quantity conserved, nothing lost or duplicated)", srcBal.Quantity, moved, srcBal.Quantity+moved)
	}
}
