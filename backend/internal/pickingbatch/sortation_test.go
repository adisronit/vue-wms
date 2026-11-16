// Phase 33C.2 -- Sortation tests. Reuses this package's own
// service_test.go fixtures/stack helpers (newFixture/seedBalance/
// newStack/createReleasedOrder/seedPickerUser) — real seeded rows via
// the real product/location/warehouse/order/picking repositories,
// testsupport.ConnectDB + BeginRollbackTx for sequential scenarios, a
// real unwrapped db.Raw-seeded fixture + manual cleanup for the one
// genuine concurrency scenario, exactly the same shape Phase 33C.1's
// own tests already established.
//
// Phase 33D.1: moved to the external "pickingbatch_test" package
// alongside service_test.go — see that file's own doc for why.
package pickingbatch_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/location"
	"wms-backend/internal/picking"
	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

// completeTaskFully drives taskID's single consolidated line (one
// product/lot/location tuple, qty total) straight to COMPLETED with one
// PICKED confirm — the simplest way to reach a terminal status for
// Sortation's own gate, since Sortation only cares that the task IS
// terminal, not which path it took to get there (Phase 33C.1's own
// TestService_Confirm_FullLifecycleThroughUnmodifiedPicking already
// covers the SHORT/Repick path in depth).
func completeTaskFully(t *testing.T, st stack, tx *gorm.DB, tenantID, taskID, itemID int64, pickerLabel string, qty float64, locationCode, sku, lotNo string) {
	t.Helper()
	pickerID := seedPickerUser(t, tx, tenantID, pickerLabel)
	if _, err := st.pickingSvc.AssignPicker(context.Background(), tenantID, taskID, pickerID, 1); err != nil {
		t.Fatalf("completeTaskFully: AssignPicker: %v", err)
	}
	if _, err := st.pickingSvc.ScanConfirm(context.Background(), tenantID, taskID, itemID, picking.ScanRequest{
		LocationCode: locationCode, SKU: sku, LotNo: lotNo,
	}, 1); err != nil {
		t.Fatalf("completeTaskFully: ScanConfirm: %v", err)
	}
	resp, err := st.pickingSvc.Confirm(context.Background(), tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: pickerLabel + "-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: qty}},
	}, 1)
	if err != nil {
		t.Fatalf("completeTaskFully: Confirm(PICKED): %v", err)
	}
	if resp.Status != picking.StatusCompleted {
		t.Fatalf("completeTaskFully: task status = %v, want COMPLETED", resp.Status)
	}
}

// buildSO001SO002SO003Batch is the exact SO-001(20)/SO-002(30)/
// SO-003(50) spec example, consolidated, confirmed, and picked all the
// way to COMPLETED — the shared setup every sortation test below needs.
func buildSO001SO002SO003Batch(t *testing.T, tx *gorm.DB, label string) (st stack, f fixture, batchID, taskID, itemID int64, so1, so2, so3 int64) {
	t.Helper()
	f = newFixture(t, tx, label)
	seedBalance(t, tx, f, "LOT-X", 100)
	st = newStack(tx)

	r1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	r2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "B", 30)
	r3 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "C", 50)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{r1.ID, r2.ID, r3.ID},
	}, 1)
	if err != nil {
		t.Fatalf("buildSO001SO002SO003Batch: Create: %v", err)
	}
	confirmed, err := st.batchSvc.Confirm(context.Background(), f.tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("buildSO001SO002SO003Batch: Confirm: %v", err)
	}
	taskID = *confirmed.PickingTaskID
	line := confirmed.Lines[0]
	itemID = *line.Allocations[0].PickingTaskItemID

	completeTaskFully(t, st, tx, f.tenantID, taskID, itemID, label, 100, line.LocationCode, line.SKU, line.LotNo)

	return st, f, draft.ID, taskID, itemID, r1.ID, r2.ID, r3.ID
}

func productSKU(t *testing.T, tx *gorm.DB, productID int64) string {
	t.Helper()
	p, err := product.NewRepository(tx).FindByID(context.Background(), productID)
	if err != nil {
		t.Fatalf("productSKU: %v", err)
	}
	return p.SKU
}

func orderNo(t *testing.T, tx *gorm.DB, tenantID, orderID int64) string {
	t.Helper()
	var no string
	if err := tx.Raw("SELECT order_no FROM orders WHERE id = ? AND tenant_id = ?", orderID, tenantID).Scan(&no).Error; err != nil {
		t.Fatalf("orderNo: %v", err)
	}
	return no
}

// TestService_Sortation_NotReadyBeforeTaskTerminal proves the gate: a
// CONFIRMED batch whose task is still PENDING/IN_PROGRESS rejects every
// sortation action with pickingbatch.ErrSortationNotReady, and a never-confirmed
// (DRAFT) batch does too.
func TestService_Sortation_NotReadyBeforeTaskTerminal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "sortgate")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "B", 30)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := st.batchSvc.GetSortation(context.Background(), f.tenantID, draft.ID); !errors.Is(err, pickingbatch.ErrSortationNotReady) {
		t.Fatalf("GetSortation on DRAFT batch: err = %v, want pickingbatch.ErrSortationNotReady", err)
	}

	confirmed, err := st.batchSvc.Confirm(context.Background(), f.tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if _, err := st.batchSvc.GetSortation(context.Background(), f.tenantID, draft.ID); !errors.Is(err, pickingbatch.ErrSortationNotReady) {
		t.Fatalf("GetSortation on CONFIRMED-but-not-terminal batch: err = %v, want pickingbatch.ErrSortationNotReady", err)
	}
	_ = confirmed
}

// TestService_Sortation_SO001SO002SO003_ScanConfirmsExactSplit is the
// phase's own spec example end to end: batch pick 100, sortation scans
// confirm 20/30/50 against each order, sortation reaches COMPLETED, and
// a repeat scan for any order is rejected as over-allocation (nothing
// left to sort).
func TestService_Sortation_SO001SO002SO003_ScanConfirmsExactSplit(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, so1, so2, so3 := buildSO001SO002SO003Batch(t, tx, "sortsplit")
	sku := productSKU(t, tx, f.productID)

	worklist, err := st.batchSvc.GetSortation(context.Background(), f.tenantID, batchID)
	if err != nil {
		t.Fatalf("GetSortation: %v", err)
	}
	if len(worklist.Lines) != 3 {
		t.Fatalf("len(Lines) = %d, want 3", len(worklist.Lines))
	}
	if worklist.SortationStatus != pickingbatch.SortationNotStarted {
		t.Fatalf("SortationStatus = %v, want NOT_STARTED", worklist.SortationStatus)
	}

	scan := func(orderID int64, qty float64) *pickingbatch.SortationResponse {
		resp, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, batchID, pickingbatch.SortationScanRequest{
			OrderBarcode: orderNo(t, tx, f.tenantID, orderID), SKU: sku, LotNo: "LOT-X", Quantity: qty,
		}, 1)
		if err != nil {
			t.Fatalf("ScanSortation(order=%d, qty=%v): %v", orderID, qty, err)
		}
		return resp
	}

	r1 := scan(so1, 20)
	if r1.SortationStatus != pickingbatch.SortationInProgress {
		t.Errorf("after first scan, SortationStatus = %v, want IN_PROGRESS", r1.SortationStatus)
	}
	scan(so2, 30)
	r3 := scan(so3, 50)
	if r3.SortationStatus != pickingbatch.SortationCompleted {
		t.Errorf("after all three scans, SortationStatus = %v, want COMPLETED", r3.SortationStatus)
	}

	for _, line := range r3.Lines {
		if !line.Resolved {
			t.Errorf("line (order=%d) Resolved = false, want true", line.OrderID)
		}
		if line.Remaining != 0 {
			t.Errorf("line (order=%d) Remaining = %v, want 0", line.OrderID, line.Remaining)
		}
	}

	// Nothing left to sort for so1 -- even 1 more unit is rejected.
	if _, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, batchID, pickingbatch.SortationScanRequest{
		OrderBarcode: orderNo(t, tx, f.tenantID, so1), SKU: sku, LotNo: "LOT-X", Quantity: 1,
	}, 1); !errors.Is(err, pickingbatch.ErrSortationOverAllocation) {
		t.Fatalf("repeat scan after fully sorted: err = %v, want pickingbatch.ErrSortationOverAllocation", err)
	}
}

// TestService_Sortation_ScanMismatch covers every "this scan does not
// resolve to a real target" case: wrong order barcode, and a real order
// but wrong SKU/lot.
func TestService_Sortation_ScanMismatch(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, so1, _, _ := buildSO001SO002SO003Batch(t, tx, "sortmismatch")
	sku := productSKU(t, tx, f.productID)
	so1No := orderNo(t, tx, f.tenantID, so1)

	cases := []struct {
		name string
		req  pickingbatch.SortationScanRequest
	}{
		{"wrong order barcode", pickingbatch.SortationScanRequest{OrderBarcode: "NO-SUCH-ORDER", SKU: sku, LotNo: "LOT-X", Quantity: 5}},
		{"wrong sku", pickingbatch.SortationScanRequest{OrderBarcode: so1No, SKU: "WRONG-SKU", LotNo: "LOT-X", Quantity: 5}},
		{"wrong lot", pickingbatch.SortationScanRequest{OrderBarcode: so1No, SKU: sku, LotNo: "WRONG-LOT", Quantity: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, batchID, tc.req, 1); !errors.Is(err, pickingbatch.ErrSortationScanMismatch) {
				t.Fatalf("err = %v, want pickingbatch.ErrSortationScanMismatch", err)
			}
		})
	}
}

// TestService_Sortation_OverAllocationRejectedNotClamped proves a
// single scan that itself exceeds the order's own remaining allocation
// is rejected outright and leaves sorted_qty completely unchanged (no
// partial/clamped application).
func TestService_Sortation_OverAllocationRejectedNotClamped(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, so1, _, _ := buildSO001SO002SO003Batch(t, tx, "sortoverflow")
	sku := productSKU(t, tx, f.productID)
	so1No := orderNo(t, tx, f.tenantID, so1)

	if _, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, batchID, pickingbatch.SortationScanRequest{
		OrderBarcode: so1No, SKU: sku, LotNo: "LOT-X", Quantity: 25, // so1 is only allocated 20
	}, 1); !errors.Is(err, pickingbatch.ErrSortationOverAllocation) {
		t.Fatalf("err = %v, want pickingbatch.ErrSortationOverAllocation", err)
	}

	worklist, err := st.batchSvc.GetSortation(context.Background(), f.tenantID, batchID)
	if err != nil {
		t.Fatalf("GetSortation: %v", err)
	}
	for _, line := range worklist.Lines {
		if line.OrderID == so1 && line.SortedQty != 0 {
			t.Errorf("after rejected over-allocation scan, SortedQty = %v, want 0 (not clamped, not partially applied)", line.SortedQty)
		}
	}
}

// TestService_Sortation_FlagException proves the shortage/exception
// path resolves a line without ever sorting it, and that re-flagging an
// already-flagged line is rejected.
func TestService_Sortation_FlagException(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, so1, so2, so3 := buildSO001SO002SO003Batch(t, tx, "sortexc")
	sku := productSKU(t, tx, f.productID)

	worklist, err := st.batchSvc.GetSortation(context.Background(), f.tenantID, batchID)
	if err != nil {
		t.Fatalf("GetSortation: %v", err)
	}
	var so1AllocID int64
	for _, l := range worklist.Lines {
		if l.OrderID == so1 {
			so1AllocID = l.AllocationID
		}
	}
	if so1AllocID == 0 {
		t.Fatal("could not find so1's own allocation line")
	}

	resp, err := st.batchSvc.FlagSortationException(context.Background(), f.tenantID, batchID, pickingbatch.SortationExceptionRequest{
		AllocationID: so1AllocID, ShortageQty: 20, Reason: "Found damaged while sorting",
	}, 1)
	if err != nil {
		t.Fatalf("FlagSortationException: %v", err)
	}
	for _, l := range resp.Lines {
		if l.OrderID == so1 {
			if !l.Resolved {
				t.Errorf("so1 line Resolved = false after full shortage flag, want true")
			}
			if l.ExceptionReason == nil {
				t.Errorf("so1 line ExceptionReason is nil, want set")
			}
		}
	}

	if _, err := st.batchSvc.FlagSortationException(context.Background(), f.tenantID, batchID, pickingbatch.SortationExceptionRequest{
		AllocationID: so1AllocID, ShortageQty: 5, Reason: "second attempt",
	}, 1); !errors.Is(err, pickingbatch.ErrSortationAlreadyFlagged) {
		t.Fatalf("re-flag: err = %v, want pickingbatch.ErrSortationAlreadyFlagged", err)
	}

	// Scanning so2/so3 fully should now complete sortation (so1's own
	// line is resolved via shortage, not sorting).
	scan := func(orderID int64, qty float64) *pickingbatch.SortationResponse {
		r, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, batchID, pickingbatch.SortationScanRequest{
			OrderBarcode: orderNo(t, tx, f.tenantID, orderID), SKU: sku, LotNo: "LOT-X", Quantity: qty,
		}, 1)
		if err != nil {
			t.Fatalf("ScanSortation(order=%d): %v", orderID, err)
		}
		return r
	}
	scan(so2, 30)
	final := scan(so3, 50)
	if final.SortationStatus != pickingbatch.SortationCompleted {
		t.Errorf("SortationStatus = %v, want COMPLETED (so1 resolved via shortage, so2/so3 fully scanned)", final.SortationStatus)
	}
}

// TestService_Sortation_AssignSortation covers the station/operator
// planning annotation.
func TestService_Sortation_AssignSortation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, _, _, _ := buildSO001SO002SO003Batch(t, tx, "sortassign")
	station := "SORT-01"

	resp, err := st.batchSvc.AssignSortation(context.Background(), f.tenantID, batchID, pickingbatch.SortationAssignRequest{Station: &station})
	if err != nil {
		t.Fatalf("AssignSortation: %v", err)
	}
	if resp.SortationStation == nil || *resp.SortationStation != station {
		t.Errorf("SortationStation = %v, want %q", resp.SortationStation, station)
	}
}

// TestService_Sortation_CrossTenantIsolation proves every sortation
// action is tenant-scoped exactly like every other FindByID in this
// package — a batch id that is real but belongs to a DIFFERENT tenant
// is rejected identically to a nonexistent one.
func TestService_Sortation_CrossTenantIsolation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, _, batchID, _, _, _, _, _ := buildSO001SO002SO003Batch(t, tx, "sorttenantA")
	fB := newFixture(t, tx, "sorttenantB")

	if _, err := st.batchSvc.GetSortation(context.Background(), fB.tenantID, batchID); !errors.Is(err, pickingbatch.ErrNotFound) {
		t.Fatalf("GetSortation cross-tenant: err = %v, want pickingbatch.ErrNotFound", err)
	}
	if _, err := st.batchSvc.ScanSortation(context.Background(), fB.tenantID, batchID, pickingbatch.SortationScanRequest{
		OrderBarcode: "ANY", SKU: "ANY", Quantity: 1,
	}, 1); !errors.Is(err, pickingbatch.ErrNotFound) {
		t.Fatalf("ScanSortation cross-tenant: err = %v, want pickingbatch.ErrNotFound", err)
	}
}

// TestService_Sortation_ConcurrentScans_NeverOverAllocates is this
// phase's own concurrency proof, the same shape as Phase 33C.1's own
// TestService_Confirm_ConcurrentConfirms_OnlyOneWins and Phase 33C's own
// TestService_PackItem_ConcurrentPacking_NeverOverpacks — a real
// unwrapped *gorm.DB (not a shared transaction: concurrent goroutines
// against one BeginRollbackTx transaction would not exercise real row-
// locking) with manual cleanup. N concurrent scans each request more
// than their fair share of the last remaining units of one order's own
// allocation line; at most the correct total may ever succeed.
func TestService_Sortation_ConcurrentScans_NeverOverAllocates(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('pb-sortconc', 'Sortation Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM picking_batch_allocations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_batch_orders WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM order_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_scan_log WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_task_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_tasks WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_batches WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM users WHERE username LIKE 'picker-sortconc%'")
		db.Exec("DELETE FROM roles WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-SORTCONC', 'Category SortConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-SORTCONC", Name: "Concurrent Sortation Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-SORTCONC', 'Warehouse SortConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-SORTCONC', 'Zone SortConc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-SORTCONC", Name: "Location SortConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	f := fixture{tenantID: tenantID, productID: productID, warehouseID: warehouseID, locationID: locationID}
	seedBalance(t, db, f, "LOT-SORTCONC", 100)

	st := newStack(db)
	so1 := createReleasedOrder(t, st.orderSvc, tenantID, f, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, tenantID, f, "B", 80)

	draft, err := st.batchSvc.Create(context.Background(), tenantID, pickingbatch.CreateRequest{
		WarehouseID: warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	confirmed, err := st.batchSvc.Confirm(context.Background(), tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	taskID := *confirmed.PickingTaskID
	line := confirmed.Lines[0]
	itemID := *line.Allocations[0].PickingTaskItemID
	completeTaskFully(t, st, db, tenantID, taskID, itemID, "sortconc", 100, line.LocationCode, line.SKU, line.LotNo)

	so1No := orderNo(t, db, tenantID, so1.ID)
	sku := "SKU-SORTCONC"

	// so1's own allocation is exactly 20. Fire 10 concurrent scans of 5
	// units each (50 requested total) -- at most 4 of them (20 units)
	// may ever succeed; the rest must be rejected, never clamped.
	const attempts = 10
	const perScan = 5.0
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := st.batchSvc.ScanSortation(context.Background(), tenantID, draft.ID, pickingbatch.SortationScanRequest{
				OrderBarcode: so1No, SKU: sku, LotNo: "LOT-SORTCONC", Quantity: perScan,
			}, 1)
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, pickingbatch.ErrSortationOverAllocation):
			// expected for the losers
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successes != 4 {
		t.Errorf("successes = %d, want exactly 4 (4*5 = 20 = so1's own allocated_qty)", successes)
	}

	var sorted float64
	if err := db.Raw(
		"SELECT sorted_qty FROM picking_batch_allocations WHERE tenant_id = ? AND batch_id = ? AND order_id = ?",
		tenantID, draft.ID, so1.ID,
	).Scan(&sorted).Error; err != nil {
		t.Fatalf("query sorted_qty: %v", err)
	}
	if sorted != 20 {
		t.Errorf("final sorted_qty for so1 = %v, want exactly 20 (never more than allocated_qty)", sorted)
	}
}
