package pickingreport

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/order"
	"wms-backend/internal/picking"
	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
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
		"pkr-"+label, "Picking Report Test Tenant "+label,
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

func seedBalance(t *testing.T, tx *gorm.DB, f fixture, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, Quantity: quantity}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
}

func newPickingService(tx *gorm.DB) *picking.Service {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	return picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
}

// seedPickedTask creates one picking task with one line (Create then
// Confirm PICKED), via the real service path. Returns the task ID and
// item ID.
func seedPickedTask(t *testing.T, tx *gorm.DB, svc *picking.Service, f fixture, lotNo string, orderedQty, pickedQty float64, createdAt time.Time) (taskID, itemID int64) {
	t.Helper()

	resp, err := svc.Create(context.Background(), f.tenantID, picking.CreateRequest{
		OrderNo: "SO-" + lotNo, WarehouseID: f.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: f.productID, LotNo: lotNo, LocationID: f.locationID, OrderedQty: orderedQty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("seedPickedTask: Create: %v", err)
	}
	taskID = resp.ID
	itemID = resp.Items[0].ID

	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo,
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: pickedQty}},
	}, 1); err != nil {
		t.Fatalf("seedPickedTask: Confirm: %v", err)
	}

	if err := tx.Exec("UPDATE picking_tasks SET created_at = ? WHERE id = ? AND tenant_id = ?", createdAt, taskID, f.tenantID).Error; err != nil {
		t.Fatalf("seedPickedTask: stamp created_at: %v", err)
	}

	return taskID, itemID
}

func seedShipment(t *testing.T, tx *gorm.DB, f fixture, taskID, taskItemID int64, lotNo string, qty float64, status shipping.Status) {
	t.Helper()
	sh := shipping.Shipment{TenantID: f.tenantID, ShipmentNo: "SHP-" + lotNo, PickingTaskID: taskID, Status: status, Packages: 1}
	if err := tx.Create(&sh).Error; err != nil {
		t.Fatalf("seedShipment: seed shipment: %v", err)
	}
	item := shipping.Item{
		TenantID: f.tenantID, ShipmentID: sh.ID, PickingTaskItemID: taskItemID,
		ProductID: f.productID, LotNo: lotNo, LocationID: f.locationID, Qty: qty, UOM: "PCS",
	}
	if err := tx.Create(&item).Error; err != nil {
		t.Fatalf("seedShipment: seed shipment item: %v", err)
	}
}

// --- Tenant isolation ---

func TestRepository_List_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "list-a")
	b := newFixture(t, tx, "list-b")
	svcA, svcB := newPickingService(tx), newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-A", 100)
	seedBalance(t, tx, b, "LOT-B", 100)
	seedPickedTask(t, tx, svcA, a, "LOT-A", 100, 100, now)
	seedPickedTask(t, tx, svcB, b, "LOT-B", 100, 50, now)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's task)", len(rows), total)
	}
	if rows[0].LotNo != "LOT-A" {
		t.Errorf("rows[0].LotNo = %q, want LOT-A", rows[0].LotNo)
	}
}

func TestRepository_Summary_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "sum-a")
	b := newFixture(t, tx, "sum-b")
	svcA, svcB := newPickingService(tx), newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-A", 100)
	seedBalance(t, tx, b, "LOT-B", 999)
	seedPickedTask(t, tx, svcA, a, "LOT-A", 100, 90, now)
	seedPickedTask(t, tx, svcB, b, "LOT-B", 999, 999, now)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalTasks != 1 {
		t.Errorf("TotalTasks = %d, want 1 (tenant B's task must not leak in)", summary.TotalTasks)
	}
	if summary.TotalPickedQty >= 999 {
		t.Errorf("TotalPickedQty = %v, tenant B's huge quantity leaked across tenant boundary", summary.TotalPickedQty)
	}
}

// --- Date boundary ---

func TestRepository_List_DateBoundary_InclusiveFromExclusiveTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "date-a")
	svc := newPickingService(tx)
	seedBalance(t, tx, a, "LOT-BEFORE", 10)
	seedBalance(t, tx, a, "LOT-START", 10)
	seedBalance(t, tx, a, "LOT-MID", 10)
	seedBalance(t, tx, a, "LOT-END", 10)

	windowStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	seedPickedTask(t, tx, svc, a, "LOT-BEFORE", 10, 10, windowStart.Add(-time.Second))
	seedPickedTask(t, tx, svc, a, "LOT-START", 10, 10, windowStart)
	seedPickedTask(t, tx, svc, a, "LOT-MID", 10, 10, windowStart.Add(3*24*time.Hour))
	seedPickedTask(t, tx, svc, a, "LOT-END", 10, 10, windowEnd)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{
		Page: 1, Limit: 20, Filters: Filters{From: &windowStart, To: &windowEnd},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (LOT-START and LOT-MID only)", total)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.LotNo] = true
	}
	if !got["LOT-START"] || !got["LOT-MID"] {
		t.Errorf("rows = %+v, want LOT-START and LOT-MID present", rows)
	}
	if got["LOT-BEFORE"] || got["LOT-END"] {
		t.Errorf("rows = %+v, want LOT-BEFORE and LOT-END excluded (strict boundary)", rows)
	}
}

// --- Short pick ---

func TestRepository_List_ShortOnly_Filter(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "short-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-FULL", 50)
	seedBalance(t, tx, a, "LOT-SHORT", 50)

	seedPickedTask(t, tx, svc, a, "LOT-FULL", 50, 50, now)

	resp, err := svc.Create(context.Background(), a.tenantID, picking.CreateRequest{
		OrderNo: "SO-SHORT", WarehouseID: a.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: a.productID, LotNo: "LOT-SHORT", LocationID: a.locationID, OrderedQty: 50, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), a.tenantID, resp.ID, picking.ConfirmRequest{
		IdempotencyKey: "short-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: resp.Items[0].ID, Action: "SHORT", ShortReason: "OUT_OF_STOCK"}},
	}, 1); err != nil {
		t.Fatalf("Confirm SHORT: %v", err)
	}

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20, Filters: Filters{ShortOnly: true}})
	if err != nil {
		t.Fatalf("List(short_only): %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].LotNo != "LOT-SHORT" {
		t.Fatalf("List(short_only) = %+v (total %d), want exactly LOT-SHORT", rows, total)
	}
	if rows[0].ShortQty == nil || *rows[0].ShortQty != 50 {
		t.Errorf("ShortQty = %v, want 50", rows[0].ShortQty)
	}

	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.ShortPickRate == nil || *summary.ShortPickRate != 50 {
		t.Fatalf("ShortPickRate = %v, want 50 (1 of 2 lines short)", summary.ShortPickRate)
	}
}

// --- Picked vs Dispatched ---

func TestRepository_List_DispatchedQty_OnlyCountsActuallyDispatchedShipments(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "disp-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-DISP", 100)

	taskID, itemID := seedPickedTask(t, tx, svc, a, "LOT-DISP", 100, 100, now)

	// A READY shipment must NOT count as dispatched...
	seedShipment(t, tx, a, taskID, itemID, "LOT-DISP", 100, shipping.StatusReady)

	repo := NewRepository(tx)
	beforeDispatch, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (before dispatch): %v", err)
	}
	if len(beforeDispatch) != 1 || beforeDispatch[0].DispatchedQty != nil {
		t.Fatalf("DispatchedQty while shipment is still READY = %v, want nil", beforeDispatch[0].DispatchedQty)
	}

	// ...flip it to DISPATCHED, now it must count.
	if err := tx.Exec("UPDATE shipments SET status = 'DISPATCHED' WHERE picking_task_id = ? AND tenant_id = ?", taskID, a.tenantID).Error; err != nil {
		t.Fatalf("flip shipment to DISPATCHED: %v", err)
	}

	afterDispatch, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (after dispatch): %v", err)
	}
	if len(afterDispatch) != 1 || afterDispatch[0].DispatchedQty == nil || *afterDispatch[0].DispatchedQty != 100 {
		t.Fatalf("DispatchedQty after DISPATCHED = %v, want 100", afterDispatch[0])
	}
	if afterDispatch[0].PickedQty != 100 {
		t.Errorf("PickedQty = %v, want 100 (unchanged by dispatch — picked and dispatched are tracked separately)", afterDispatch[0].PickedQty)
	}
}

// --- Double-counting: consolidated batch pick shared across orders ---

func TestRepository_ListBatchAllocations_DoesNotDoubleCountSharedPick(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "batch-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-BATCH", 100)

	// One real physical pick of 100, shared across two orders (50 each).
	taskID, itemID := seedPickedTask(t, tx, svc, a, "LOT-BATCH", 100, 100, now)

	batch := pickingbatch.Batch{
		TenantID: a.tenantID, BatchNo: "BATCH-1", WarehouseID: a.warehouseID, Status: pickingbatch.StatusConfirmed,
		PickingTaskID: &taskID, SortationStatus: pickingbatch.SortationNotStarted,
	}
	if err := tx.Create(&batch).Error; err != nil {
		t.Fatalf("seed picking_batches: %v", err)
	}

	order1 := order.Order{TenantID: a.tenantID, OrderNo: "SO-BATCH-1", OrderType: "SALES_ORDER", Priority: "NORMAL", WarehouseID: a.warehouseID, Status: order.StatusPicking, PickingMode: order.PickingModeByProduct, PickingBatchID: &batch.ID}
	order2 := order.Order{TenantID: a.tenantID, OrderNo: "SO-BATCH-2", OrderType: "SALES_ORDER", Priority: "NORMAL", WarehouseID: a.warehouseID, Status: order.StatusPicking, PickingMode: order.PickingModeByProduct, PickingBatchID: &batch.ID}
	if err := tx.Create(&order1).Error; err != nil {
		t.Fatalf("seed order1: %v", err)
	}
	if err := tx.Create(&order2).Error; err != nil {
		t.Fatalf("seed order2: %v", err)
	}

	for _, bo := range []pickingbatch.BatchOrder{
		{TenantID: a.tenantID, BatchID: batch.ID, OrderID: order1.ID},
		{TenantID: a.tenantID, BatchID: batch.ID, OrderID: order2.ID},
	} {
		if err := tx.Create(&bo).Error; err != nil {
			t.Fatalf("seed picking_batch_orders: %v", err)
		}
	}

	orderItem1 := order.Item{TenantID: a.tenantID, OrderID: order1.ID, ProductID: a.productID, OrderedQty: 50, UOM: "PCS"}
	orderItem2 := order.Item{TenantID: a.tenantID, OrderID: order2.ID, ProductID: a.productID, OrderedQty: 50, UOM: "PCS"}
	if err := tx.Create(&orderItem1).Error; err != nil {
		t.Fatalf("seed order_items (order1): %v", err)
	}
	if err := tx.Create(&orderItem2).Error; err != nil {
		t.Fatalf("seed order_items (order2): %v", err)
	}

	for _, alloc := range []pickingbatch.Allocation{
		{TenantID: a.tenantID, BatchID: batch.ID, OrderID: order1.ID, OrderItemID: orderItem1.ID, ProductID: a.productID, LotNo: "LOT-BATCH", LocationID: a.locationID, PickingTaskItemID: &itemID, AllocatedQty: 50, UOM: "PCS", SortedQty: 0},
		{TenantID: a.tenantID, BatchID: batch.ID, OrderID: order2.ID, OrderItemID: orderItem2.ID, ProductID: a.productID, LotNo: "LOT-BATCH", LocationID: a.locationID, PickingTaskItemID: &itemID, AllocatedQty: 50, UOM: "PCS", SortedQty: 0},
	} {
		if err := tx.Create(&alloc).Error; err != nil {
			t.Fatalf("seed picking_batch_allocations: %v", err)
		}
	}

	repo := NewRepository(tx)

	// The main report, at task_item grain, must show the ONE physical
	// pick exactly once — never 200 (50+50 double-counted).
	mainRows, mainTotal, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if mainTotal != 1 || len(mainRows) != 1 || mainRows[0].PickedQty != 100 {
		t.Fatalf("main List = %+v (total %d), want exactly 1 row with PickedQty=100 (the shared physical pick, not doubled)", mainRows, mainTotal)
	}
	if mainRows[0].PickingMode != "BATCH" {
		t.Errorf("PickingMode = %q, want BATCH (derived from the picking_batches link)", mainRows[0].PickingMode)
	}

	// The batch-allocation report, at allocation grain, must show BOTH
	// orders' own 50-unit shares, summing back to exactly the shared
	// 100 — proving per-order attribution without inventing quantity.
	allocRows, allocTotal, err := repo.ListBatchAllocations(context.Background(), a.tenantID, BatchAllocationListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListBatchAllocations: %v", err)
	}
	if allocTotal != 2 || len(allocRows) != 2 {
		t.Fatalf("ListBatchAllocations = %+v (total %d), want exactly 2 rows (one per order)", allocRows, allocTotal)
	}
	var allocatedSum float64
	for _, r := range allocRows {
		allocatedSum += r.AllocatedQty
		if r.SharedTaskPickedQty == nil || *r.SharedTaskPickedQty != 100 {
			t.Errorf("SharedTaskPickedQty = %v, want 100 for every allocation row referencing this shared line", r.SharedTaskPickedQty)
		}
	}
	if allocatedSum != 100 {
		t.Fatalf("sum(AllocatedQty) across both orders = %v, want 100 (matches the shared pick exactly, no invented quantity)", allocatedSum)
	}
}
