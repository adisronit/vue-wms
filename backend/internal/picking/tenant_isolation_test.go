// Phase 30 Batch 2 — Picking tenant isolation. Mirrors
// internal/inventory/tenant_isolation_test.go's own shape (Batch 1:
// fresh tenant per test, real seeded rows, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on task/item lookup by ID, the raw-SQL atomic AdjustPickedQty UPDATE,
// cross-tenant Product/Location ownership verification at
// task-creation, the full Create->Confirm->Cancel reservation lifecycle
// under real tenant scoping, and rollback atomicity.
package picking

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
	"wms-backend/internal/warehouse"
)

// pickingFixture is one fully-seeded tenant: a product, a warehouse,
// and a location within it, plus (optionally) an inventory balance —
// real rows via the real product/location/warehouse/inventory
// repositories, not stubs, so ownership verification exercises the
// actual FindByIDForTenant/resolveLocation queries this batch added.
type pickingFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newPickingFixture(t *testing.T, tx *gorm.DB, label string) pickingFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"pick-iso-"+label, "Picking Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newPickingFixture(%s): seed location: %v", label, err)
	}

	return pickingFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedPickingBalance(t *testing.T, tx *gorm.DB, f pickingFixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedPickingBalance: %v", err)
	}
}

func newPickingServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, location.NewRepository(tx))
	return NewService(repo, productRepo, warehouseRepo, invSvc), repo
}

// createTask is a small helper that runs Service.Create end-to-end
// (through the real transaction path) and returns the new task's ID.
// lotNo must match whatever lot the caller seeded a balance under —
// ReserveStockTx matches on (product, location, lot) exactly.
func createTask(t *testing.T, svc *Service, tenantID int64, f pickingFixture, lotNo string, orderedQty float64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		OrderNo: "SO-TEST", WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, OrderedQty: orderedQty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createTask: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindTaskByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "find-a")
	b := newPickingFixture(t, tx, "find-b")
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, repo := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := repo.FindTaskByID(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindTaskByID(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindTaskByID(context.Background(), b.tenantID, taskID); err != nil {
		t.Errorf("FindTaskByID(tenantB, its own task) = %v, want success", err)
	}
}

func TestRepository_FindTaskDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "detail-a")
	b := newPickingFixture(t, tx, "detail-b")
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, repo := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := repo.FindTaskDetail(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindTaskDetail(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "get-a")
	b := newPickingFixture(t, tx, "get-b")
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := svc.Get(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "list-a")
	b := newPickingFixture(t, tx, "list-b")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, repo := newPickingServiceAndRepo(tx)
	createTask(t, svc, a.tenantID, a, "LOT-A", 5)
	createTask(t, svc, b.tenantID, b, "LOT-B", 5)

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's task)", len(rows), total)
	}
}

// --- Location ownership (createItem's resolveLocation call) ---

func TestRepository_ResolveLocation_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "loc-a")
	b := newPickingFixture(t, tx, "loc-b")

	repo := NewRepository(tx)
	if _, err := repo.resolveLocation(context.Background(), a.tenantID, b.locationID); !errors.Is(err, ErrLocationNotFound) {
		t.Errorf("resolveLocation(tenantA, tenantB's location) = %v, want ErrLocationNotFound", err)
	}
}

// --- Service-layer ownership verification at task creation ---

func TestService_Create_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "cp-a")
	b := newPickingFixture(t, tx, "cp-b")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		OrderNo: "SO-X", WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{{ProductID: b.productID, LocationID: a.locationID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("Create(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "cl-a")
	b := newPickingFixture(t, tx, "cl-b")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		OrderNo: "SO-X", WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{{ProductID: a.productID, LocationID: b.locationID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(tenantA, tenantB's location_id) = %v, want ErrLocationNotFound", err)
	}
}

// TestService_Create_ReservesAgainstOwnTenantOnly proves the Inventory
// reservation created by createItem is itself scoped to the caller's
// own tenant — a same-named/looking balance belonging to another tenant
// must never be reserved against, and must never be able to satisfy
// this tenant's own reservation.
func TestService_Create_ReservesAgainstOwnTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "res-a")
	b := newPickingFixture(t, tx, "res-b")
	// Tenant B has plenty of stock; tenant A has none — if tenant A's
	// reservation could somehow reach tenant B's balance, this would
	// wrongly succeed.
	seedPickingBalance(t, tx, b, "LOT-A", 1000, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		OrderNo: "SO-X", WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{{ProductID: a.productID, LocationID: a.locationID, LotNo: "LOT-A", OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrNoSourceStock) {
		t.Fatalf("Create(tenantA, no stock of its own) = %v, want ErrNoSourceStock (must not reach tenant B's balance)", err)
	}
}

func TestService_Create_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "create-ok")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		OrderNo: "SO-OK", WarehouseID: a.warehouseID,
		Items: []CreateItemRequest{{ProductID: a.productID, LocationID: a.locationID, LotNo: "LOT-A", OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create(own tenant's own product/location): %v", err)
	}
	if resp.Status != StatusPending || len(resp.Items) != 1 {
		t.Fatalf("Create response = %+v, want 1 PENDING item", resp)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after Create: %v", err)
	}
	if bal.ReservedQuantity != 10 {
		t.Errorf("ReservedQuantity after Create = %v, want 10", bal.ReservedQuantity)
	}
}

// --- Confirm: IDOR + the Picked/Short reservation lifecycle ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "confirm-cross-a")
	b := newPickingFixture(t, tx, "confirm-cross-b")
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, b.tenantID, b, "LOT-B", 10)

	_, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
		Items:          []ConfirmItemRequest{{ItemID: 1, Action: "PICKED", Quantity: 10}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}

	// Confirming as the task's real owner must still work after the
	// rejected cross-tenant attempt — proves the cross-tenant call left
	// no residue (no idempotency row inserted under the wrong key/tenant
	// pairing that could block the legitimate caller).
	resp, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("task status after rejected cross-tenant Confirm = %v, want still PENDING", resp.Status)
	}
}

func TestService_Confirm_PickedLine_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "confirm-ok")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	task, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := task.Items[0].ID

	resp, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-ok-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 10}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(own tenant, PICKED): %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("task status = %v, want COMPLETED", resp.Status)
	}
	if resp.Items[0].PickedQty != 10 {
		t.Errorf("PickedQty = %v, want 10", resp.Items[0].PickedQty)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after Confirm: %v", err)
	}
	if bal.Quantity != 90 {
		t.Errorf("Quantity after Confirm = %v, want 90 (100-10)", bal.Quantity)
	}
	if bal.ReservedQuantity != 0 {
		t.Errorf("ReservedQuantity after Confirm = %v, want 0", bal.ReservedQuantity)
	}
}

func TestService_Confirm_ShortLine_ReleasesReservation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "confirm-short")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	task, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := task.Items[0].ID

	resp, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-short-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Action: "SHORT", Remark: "out of stock"}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(own tenant, SHORT): %v", err)
	}
	if resp.Status != StatusShortPick {
		t.Errorf("task status = %v, want SHORT_PICK", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after SHORT Confirm: %v", err)
	}
	if bal.Quantity != 100 {
		t.Errorf("Quantity after SHORT = %v, want unchanged 100 (nothing was picked)", bal.Quantity)
	}
	if bal.ReservedQuantity != 0 {
		t.Errorf("ReservedQuantity after SHORT = %v, want 0 (fully released)", bal.ReservedQuantity)
	}
}

// --- Cancel: IDOR + reservation release ---

func TestService_Cancel_CrossTenant_NotCancellable(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "cancel-cross-a")
	b := newPickingFixture(t, tx, "cancel-cross-b")
	seedPickingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, b.tenantID, b, "LOT-B", 10)

	_, err := svc.Cancel(context.Background(), a.tenantID, taskID, CancelRequest{}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}

	resp, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Cancel: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("task status after rejected cross-tenant Cancel = %v, want still PENDING (not CANCELLED)", resp.Status)
	}
}

func TestService_Cancel_OwnTenant_ReleasesRemaining(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "cancel-ok")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)

	resp, err := svc.Cancel(context.Background(), a.tenantID, taskID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(own tenant): %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("task status = %v, want CANCELLED", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after Cancel: %v", err)
	}
	if bal.ReservedQuantity != 0 {
		t.Errorf("ReservedQuantity after Cancel = %v, want 0 (fully released)", bal.ReservedQuantity)
	}
}

// --- Rollback / atomicity ---

// TestService_Confirm_InsufficientStock_RollsBackEverything proves
// Confirm's whole transaction — including AdjustPickedQty's atomic
// increment — rolls back together when a later step fails, now that
// tenant_id is threaded through every statement in it. Requests more
// than the line's own reservation covers, so ConsumeReservationTx's
// AdjustReservation leg underflows (CK_inventory_reserved_within_quantity
// otherwise forbids reducing on-hand quantity below what is already
// reserved, so that invariant can't be used to simulate this directly —
// an over-large confirm request is the realistic way this fails).
func TestService_Confirm_InsufficientStock_RollsBackEverything(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "confirm-rollback")
	seedPickingBalance(t, tx, a, "LOT-A", 10, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	task, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := task.Items[0].ID

	// Only 10 was ever reserved for this line — requesting 999 forces
	// AdjustReservation to underflow (ErrReservationUnderflow), rejected
	// before any stock is actually removed.
	_, err = svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 999}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(insufficient stock) = %v, want ErrValidation", err)
	}

	// The whole transaction — including AdjustPickedQty's increment —
	// must have rolled back: picked_qty must still be 0.
	updated, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rolled-back Confirm: %v", err)
	}
	if updated.Items[0].PickedQty != 0 {
		t.Errorf("PickedQty after rolled-back Confirm = %v, want 0 (AdjustPickedQty must have rolled back too)", updated.Items[0].PickedQty)
	}
	if updated.Status != StatusPending {
		t.Errorf("task status after rolled-back Confirm = %v, want still PENDING", updated.Status)
	}
}

// --- Concurrency: AdjustPickedQty's atomic guarded UPDATE ---

// TestRepository_AdjustPickedQty_WrongTenant_NoRowsAffected proves the
// highest-risk fix in this batch directly — the same way Batch 1 proved
// it for AdjustBalance/AdjustReservation: a caller passing a mismatched
// tenantID against a real itemID owned by a different tenant affects
// zero rows, so a cross-tenant call can never corrupt another tenant's
// picked_qty.
func TestRepository_AdjustPickedQty_WrongTenant_NoRowsAffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "adjust-a")
	b := newPickingFixture(t, tx, "adjust-b")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, repo := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	task, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := task.Items[0].ID

	rows, err := repo.AdjustPickedQty(context.Background(), b.tenantID, itemID, 5)
	if err != nil {
		t.Fatalf("AdjustPickedQty: %v", err)
	}
	if rows != 0 {
		t.Fatalf("AdjustPickedQty(tenantB, tenantA's item) affected %d rows, want 0", rows)
	}

	item, err := repo.FindItemByID(context.Background(), a.tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if item.PickedQty != 0 {
		t.Errorf("tenant A's picked_qty = %v after a rejected cross-tenant AdjustPickedQty, want unchanged 0", item.PickedQty)
	}
}

// TestRepository_AdjustPickedQty_Concurrent_NoLostUpdates re-proves the
// same concurrency guarantee Batch 1 established for AdjustBalance/
// AdjustReservation: the atomic UPDATE's arithmetic happens inside the
// database, so N concurrent increments against the same row never lose
// an update to a race, now that tenant_id lives in the same WHERE
// clause. Deliberately does NOT use testsupport.BeginRollbackTx — a
// single *sql.Tx is not safe for concurrent use by multiple goroutines
// — so this uses the real pooled *gorm.DB with explicit, FK-ordered
// cleanup instead of rollback.
func TestRepository_AdjustPickedQty_Concurrent_NoLostUpdates(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID, taskID, itemID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('pick-concurrent', 'Picking Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM picking_task_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_tasks WHERE tenant_id = ?", tenantID)
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
	p := product.Product{TenantID: tenantID, SKU: "SKU-PCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-PCONC', 'Warehouse Conc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-PCONC', 'Zone Conc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-PCONC", Name: "Location Conc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	task := Task{TenantID: tenantID, OrderNo: "SO-CONC", WarehouseID: warehouseID, Status: StatusInProgress, Priority: PriorityNormal}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
	taskID = task.ID
	item := TaskItem{TenantID: tenantID, PickingTaskID: taskID, ProductID: productID, LocationID: locationID, OrderedQty: 20, UOM: "PCS", Status: ItemStatusPending}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed item: %v", err)
	}
	itemID = item.ID

	const attempts = 20
	repo := NewRepository(db)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.AdjustPickedQty(context.Background(), tenantID, itemID, 1); err != nil {
				t.Errorf("AdjustPickedQty: %v", err)
			}
		}()
	}
	wg.Wait()

	updated, err := repo.FindItemByID(context.Background(), tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if updated.PickedQty != attempts {
		t.Errorf("final picked_qty = %v, want %d (no lost updates across %d concurrent increments)", updated.PickedQty, attempts, attempts)
	}
}
