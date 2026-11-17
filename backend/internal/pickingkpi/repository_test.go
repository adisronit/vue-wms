package pickingkpi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
)

type fixture struct {
	tenantID     int64
	productID    int64
	warehouseID  int64
	locationID   int64
	sku          string
	locationCode string
}

func newFixture(t *testing.T, tx *gorm.DB, label string) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"pkpi-"+label, "Picking KPI Test Tenant "+label,
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

	sku := "SKU-" + label
	p := product.Product{
		TenantID: tenantID, SKU: sku, Name: "Product " + label,
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

	locationCode := "LOC-" + label
	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: locationCode, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newFixture(%s): seed location: %v", label, err)
	}

	return fixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID, sku: sku, locationCode: locationCode}
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

// seedCompletedTask creates and fully picks one task (one line), via
// the real Create/Confirm path, then stamps created_at/completed_at
// to the given values so cycle-time math is deterministic.
func seedCompletedTask(t *testing.T, tx *gorm.DB, svc *picking.Service, f fixture, lotNo string, qty float64, createdAt, completedAt time.Time, dueDate *time.Time) (taskID, itemID int64) {
	t.Helper()

	resp, err := svc.Create(context.Background(), f.tenantID, picking.CreateRequest{
		OrderNo: "SO-" + lotNo, WarehouseID: f.warehouseID, DueDate: dueDate,
		Items: []picking.CreateItemRequest{{ProductID: f.productID, LotNo: lotNo, LocationID: f.locationID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("seedCompletedTask: Create: %v", err)
	}
	taskID = resp.ID
	itemID = resp.Items[0].ID

	if _, err := svc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo,
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: qty}},
	}, 1); err != nil {
		t.Fatalf("seedCompletedTask: Confirm: %v", err)
	}

	if err := tx.Exec("UPDATE picking_tasks SET created_at = ?, completed_at = ? WHERE id = ? AND tenant_id = ?", createdAt, completedAt, taskID, f.tenantID).Error; err != nil {
		t.Fatalf("seedCompletedTask: stamp timestamps: %v", err)
	}

	return taskID, itemID
}

// --- Tenant isolation ---

func TestRepository_Summary_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "sum-a")
	b := newFixture(t, tx, "sum-b")
	svcA, svcB := newPickingService(tx), newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-A", 10)
	seedBalance(t, tx, b, "LOT-B", 999)
	seedCompletedTask(t, tx, svcA, a, "LOT-A", 10, now.Add(-time.Hour), now, nil)
	seedCompletedTask(t, tx, svcB, b, "LOT-B", 999, now.Add(-time.Hour), now, nil)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalTasks != 1 {
		t.Errorf("TotalTasks = %d, want 1 (tenant B's task must not leak in)", summary.TotalTasks)
	}
	if summary.PickedUnits >= 999 {
		t.Errorf("PickedUnits = %v, tenant B's huge quantity leaked across tenant boundary", summary.PickedUnits)
	}
}

// --- Completion rate + cycle time ---

func TestRepository_Summary_CompletionRateAndCycleTime(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "cycle-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-DONE", 10)

	// One completed task with an exact 2-hour cycle time.
	seedCompletedTask(t, tx, svc, a, "LOT-DONE", 10, now.Add(-2*time.Hour), now, nil)

	// One still-open task (never confirmed) — counts toward total_tasks
	// but not completed_tasks, and contributes no cycle-time sample.
	seedBalance(t, tx, a, "LOT-OPEN", 10)
	if _, err := svc.Create(context.Background(), a.tenantID, picking.CreateRequest{
		OrderNo: "SO-OPEN", WarehouseID: a.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: a.productID, LotNo: "LOT-OPEN", LocationID: a.locationID, OrderedQty: 10, UOM: "PCS"}},
	}, 1); err != nil {
		t.Fatalf("Create(open task): %v", err)
	}

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalTasks != 2 || summary.CompletedTasks != 1 {
		t.Fatalf("TotalTasks/CompletedTasks = %d/%d, want 2/1", summary.TotalTasks, summary.CompletedTasks)
	}
	if summary.CompletionRatePct == nil || *summary.CompletionRatePct != 50 {
		t.Fatalf("CompletionRatePct = %v, want 50", summary.CompletionRatePct)
	}
	if summary.AvgCycleTimeHours == nil || *summary.AvgCycleTimeHours != 2 {
		t.Fatalf("AvgCycleTimeHours = %v, want 2 (only the completed task counts)", summary.AvgCycleTimeHours)
	}
}

// --- Accuracy: scanned subset only ---

func TestRepository_Summary_Accuracy_NilWithoutScans_ComputedWithScans(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "acc-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-NOSCANS", 10)
	seedCompletedTask(t, tx, svc, a, "LOT-NOSCANS", 10, now, now, nil)

	repo := NewRepository(tx)
	noScans, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary (no scans): %v", err)
	}
	if noScans.Accuracy.RatePct != nil {
		t.Errorf("Accuracy.RatePct with zero scan log rows = %v, want nil (N/A)", noScans.Accuracy.RatePct)
	}

	seedBalance(t, tx, a, "LOT-SCANNED", 10)
	resp, err := svc.Create(context.Background(), a.tenantID, picking.CreateRequest{
		OrderNo: "SO-SCANNED", WarehouseID: a.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: a.productID, LotNo: "LOT-SCANNED", LocationID: a.locationID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := resp.Items[0].ID

	if _, err := svc.ScanConfirm(context.Background(), a.tenantID, resp.ID, itemID, picking.ScanRequest{
		LocationCode: a.locationCode, SKU: a.sku, LotNo: "LOT-SCANNED",
	}, 1); err != nil {
		t.Fatalf("ScanConfirm (match): %v", err)
	}
	if _, err := svc.ScanConfirm(context.Background(), a.tenantID, resp.ID, itemID, picking.ScanRequest{
		LocationCode: "WRONG-LOCATION", SKU: a.sku, LotNo: "LOT-SCANNED",
	}, 1); err != nil {
		t.Fatalf("ScanConfirm (mismatch): %v", err)
	}

	withScans, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary (with scans): %v", err)
	}
	if withScans.Accuracy.Denominator != 2 {
		t.Fatalf("Accuracy.Denominator = %d, want 2 (1 match + 1 mismatch)", withScans.Accuracy.Denominator)
	}
	if withScans.Accuracy.RatePct == nil || *withScans.Accuracy.RatePct != 50 {
		t.Fatalf("Accuracy.RatePct = %v, want 50 (1 of 2 scans matched)", withScans.Accuracy.RatePct)
	}
}

// --- On-time: tasks without a due date are excluded from the rate ---

func TestRepository_Summary_OnTimeRate_ExcludesTasksWithoutDueDate(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "ontime-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-NODUE", 10)
	seedCompletedTask(t, tx, svc, a, "LOT-NODUE", 10, now.Add(-time.Hour), now, nil)

	// due_date is a DATE column (no time-of-day) — the due dates must
	// differ by whole calendar days for this test to distinguish
	// on-time from late, not just by hours within the same day.
	seedBalance(t, tx, a, "LOT-ONTIME", 10)
	dueTomorrow := now.AddDate(0, 0, 1)
	seedCompletedTask(t, tx, svc, a, "LOT-ONTIME", 10, now.Add(-time.Hour), now, &dueTomorrow)

	seedBalance(t, tx, a, "LOT-LATE", 10)
	dueYesterday := now.AddDate(0, 0, -1)
	seedCompletedTask(t, tx, svc, a, "LOT-LATE", 10, now.Add(-time.Hour), now, &dueYesterday)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TasksWithoutDueDate != 1 {
		t.Errorf("TasksWithoutDueDate = %d, want 1", summary.TasksWithoutDueDate)
	}
	if summary.OnTime.Denominator != 2 {
		t.Fatalf("OnTime.Denominator = %d, want 2 (only tasks WITH a due date)", summary.OnTime.Denominator)
	}
	if summary.OnTime.RatePct == nil || *summary.OnTime.RatePct != 50 {
		got := "nil"
		if summary.OnTime.RatePct != nil {
			got = fmt.Sprintf("%v", *summary.OnTime.RatePct)
		}
		t.Fatalf("OnTime.RatePct = %s (numerator=%d denominator=%d), want 50 (1 of 2 due-dated tasks finished on time)", got, summary.OnTime.Numerator, summary.OnTime.Denominator)
	}
}

// --- Short / Re-pick rate ---

func TestRepository_Summary_ShortAndRepickRate(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "short-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-FULL", 10)
	seedCompletedTask(t, tx, svc, a, "LOT-FULL", 10, now, now, nil)

	seedBalance(t, tx, a, "LOT-SHORT", 10)
	resp, err := svc.Create(context.Background(), a.tenantID, picking.CreateRequest{
		OrderNo: "SO-SHORT", WarehouseID: a.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: a.productID, LotNo: "LOT-SHORT", LocationID: a.locationID, OrderedQty: 10, UOM: "PCS"}},
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
	if _, err := svc.Repick(context.Background(), a.tenantID, resp.ID, resp.Items[0].ID, 1); err != nil {
		t.Fatalf("Repick: %v", err)
	}

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.ShortPickRatePct == nil || *summary.ShortPickRatePct != 0 {
		t.Fatalf("ShortPickRatePct = %v, want 0 (the SHORT line was cleared by Repick, no longer SHORT)", summary.ShortPickRatePct)
	}
	if summary.RepickRatePct == nil || *summary.RepickRatePct != 50 {
		t.Fatalf("RepickRatePct = %v, want 50 (1 of 2 lines was ever repicked)", summary.RepickRatePct)
	}
	if summary.TotalRepicks != 1 {
		t.Errorf("TotalRepicks = %d, want 1", summary.TotalRepicks)
	}
}

// --- Picking Mode Comparison ---

func TestRepository_Summary_ByPickingMode_DistinguishesBatchFromPickByOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "mode-a")
	svc := newPickingService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-SOLO", 10)
	soloTaskID, _ := seedCompletedTask(t, tx, svc, a, "LOT-SOLO", 10, now.Add(-time.Hour), now, nil)
	_ = soloTaskID

	seedBalance(t, tx, a, "LOT-BATCH", 10)
	batchTaskID, _ := seedCompletedTask(t, tx, svc, a, "LOT-BATCH", 10, now.Add(-time.Hour), now, nil)
	if err := tx.Exec(
		"INSERT INTO picking_batches (tenant_id, batch_no, warehouse_id, status, picking_task_id, sortation_status) VALUES (?, 'BATCH-1', ?, 'CONFIRMED', ?, 'NOT_STARTED')",
		a.tenantID, a.warehouseID, batchTaskID,
	).Error; err != nil {
		t.Fatalf("seed picking_batches: %v", err)
	}

	repo := NewRepository(tx)
	rows, err := repo.pickingModeComparison(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("pickingModeComparison: %v", err)
	}
	byMode := map[string]int64{}
	for _, r := range rows {
		byMode[r.PickingMode] = r.TaskCount
	}
	if byMode["PICK_BY_ORDER"] != 1 || byMode["BATCH"] != 1 {
		t.Fatalf("byMode = %+v, want exactly 1 PICK_BY_ORDER and 1 BATCH", byMode)
	}
}
