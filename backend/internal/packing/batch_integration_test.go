// Phase 33C.2 -- Sortation + Packing Integration tests. Wires the full
// real dependency graph this scenario spans: internal/order ->
// internal/pickingbatch -> internal/picking (unmodified) -> internal/
// packing's own new batch-sourced Create branch, all bound to one
// shared *gorm.DB, mirroring internal/pickingbatch's own service_test.go
// stack shape.
package packing

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/customer"
	"wms-backend/internal/customerdeliverylocation"
	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/order"
	"wms-backend/internal/picking"
	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

// batchStack wires order + pickingbatch + picking + packing against one
// shared tx — the exact graph cmd/server/main.go wires in production,
// including packingService.SetBatches(pickingBatchService).
type batchStack struct {
	orderSvc   *order.Service
	batchSvc   *pickingbatch.Service
	pickingSvc *picking.Service
	packingSvc *Service
}

func newBatchStack(tx *gorm.DB) batchStack {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingSvc := picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	pickingSvc.SetUsers(user.NewRepository(tx))

	orderRepo := order.NewRepository(tx)
	customerRepo := customer.NewRepository(tx)
	shipToRepo := customerdeliverylocation.NewRepository(tx)
	shippingSvc := shipping.NewService(shipping.NewRepository(tx), pickingSvc)
	orderSvc := order.NewService(orderRepo, productRepo, warehouseRepo, customerRepo, shipToRepo, pickingSvc, shippingSvc, invSvc)

	batchSvc := pickingbatch.NewService(pickingbatch.NewRepository(tx), warehouseRepo, pickingSvc)

	packingSvc := NewService(NewRepository(tx), pickingSvc)
	packingSvc.SetUsers(user.NewRepository(tx))
	packingSvc.SetBatches(batchSvc)

	return batchStack{orderSvc: orderSvc, batchSvc: batchSvc, pickingSvc: pickingSvc, packingSvc: packingSvc}
}

func batchFixture(t *testing.T, tx *gorm.DB, label string) packingFixture {
	t.Helper()
	return newPackingFixture(t, tx, label)
}

func releasedPickByProductOrder(t *testing.T, svc *order.Service, tenantID int64, f packingFixture, customerName string, qty float64) *order.Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, order.CreateRequest{
		WarehouseID: f.warehouseID, Customer: &customerName, PickingMode: "PICK_BY_PRODUCT",
		Items: []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("releasedPickByProductOrder: Create: %v", err)
	}
	resp, err = svc.Release(context.Background(), tenantID, resp.ID, order.ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("releasedPickByProductOrder: Release: %v", err)
	}
	return resp
}

func seedPicker(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	var roleID int64
	if err := tx.Raw(
		"INSERT INTO roles (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "PICKER-"+label, "Picker Role "+label,
	).Scan(&roleID).Error; err != nil {
		t.Fatalf("seedPicker(%s): insert role: %v", label, err)
	}
	var userID int64
	if err := tx.Raw(
		"INSERT INTO users (username, password_hash, full_name, email, role_id) OUTPUT INSERTED.id VALUES (?, 'x', ?, ?, ?)",
		"packpicker-"+label, "Picker "+label, "packpicker-"+label+"@example.com", roleID,
	).Scan(&userID).Error; err != nil {
		t.Fatalf("seedPicker(%s): insert user: %v", label, err)
	}
	return userID
}

// TestService_Create_BatchSourced_SO001SO002SO003_ThreeIndependentJobs
// is Phase 33C.2's own headline scenario: SO-001(20)/SO-002(30)/
// SO-003(50) consolidated into one batch pick of 100, the pick
// completed, Sortation scan-confirms each order's own 20/30/50 share,
// and THREE independent packing jobs are created (one per order) —
// never one job for the whole consolidated batch — each one bounded to
// exactly that order's own sorted_qty, never any other order's share.
func TestService_Create_BatchSourced_SO001SO002SO003_ThreeIndependentJobs(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := batchFixture(t, tx, "batch3jobs")
	seedPackingBalance(t, tx, f, "LOT-X", 100, 0)
	st := newBatchStack(tx)

	so1 := releasedPickByProductOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := releasedPickByProductOrder(t, st.orderSvc, f.tenantID, f, "B", 30)
	so3 := releasedPickByProductOrder(t, st.orderSvc, f.tenantID, f, "C", 50)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID, so3.ID},
	}, 1)
	if err != nil {
		t.Fatalf("batchSvc.Create: %v", err)
	}
	confirmed, err := st.batchSvc.Confirm(context.Background(), f.tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("batchSvc.Confirm: %v", err)
	}
	taskID := *confirmed.PickingTaskID
	line := confirmed.Lines[0]
	itemID := *line.Allocations[0].PickingTaskItemID

	pickerID := seedPicker(t, tx, f.tenantID, "batch3jobs")
	if _, err := st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, pickerID, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}
	if _, err := st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, itemID, picking.ScanRequest{
		LocationCode: line.LocationCode, SKU: line.SKU, LotNo: line.LotNo,
	}, 1); err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	pickedResp, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "batch3jobs-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 100}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}
	if pickedResp.Status != picking.StatusCompleted {
		t.Fatalf("task status = %v, want COMPLETED", pickedResp.Status)
	}

	// Before any sortation at all, every order's job creation attempt is
	// rejected (nothing sorted yet).
	if _, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{
		PickingTaskID: taskID, OrderID: &so1.ID,
	}, 1); !errors.Is(err, ErrSortationIncomplete) {
		t.Fatalf("Create before sortation: err = %v, want ErrSortationIncomplete", err)
	}

	sku := line.SKU
	scanSort := func(orderID int64, orderNo string, qty float64) {
		if _, err := st.batchSvc.ScanSortation(context.Background(), f.tenantID, draft.ID, pickingbatch.SortationScanRequest{
			OrderBarcode: orderNo, SKU: sku, LotNo: line.LotNo, Quantity: qty,
		}, 1); err != nil {
			t.Fatalf("ScanSortation(order=%d): %v", orderID, err)
		}
	}
	scanSort(so1.ID, so1.OrderNo, 20)
	scanSort(so2.ID, so2.OrderNo, 30)
	scanSort(so3.ID, so3.OrderNo, 50)

	// Now each order's own job can be created independently.
	job1, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID, OrderID: &so1.ID}, 1)
	if err != nil {
		t.Fatalf("Create(order=so1): %v", err)
	}
	job2, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID, OrderID: &so2.ID}, 1)
	if err != nil {
		t.Fatalf("Create(order=so2): %v", err)
	}
	job3, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID, OrderID: &so3.ID}, 1)
	if err != nil {
		t.Fatalf("Create(order=so3): %v", err)
	}

	if job1.ID == job2.ID || job2.ID == job3.ID || job1.ID == job3.ID {
		t.Fatal("expected three DISTINCT packing job ids, one per order")
	}

	jobs := map[int64]*Response{so1.ID: job1, so2.ID: job2, so3.ID: job3}
	wantQty := map[int64]float64{so1.ID: 20, so2.ID: 30, so3.ID: 50}
	wantOrderNo := map[int64]string{so1.ID: so1.OrderNo, so2.ID: so2.OrderNo, so3.ID: so3.OrderNo}
	for orderID, job := range jobs {
		if job.OrderID == nil || *job.OrderID != orderID {
			t.Errorf("job for order %d: OrderID = %v, want %d", orderID, job.OrderID, orderID)
		}
		if job.PickingTaskID != taskID {
			t.Errorf("job for order %d: PickingTaskID = %d, want %d (shared task)", orderID, job.PickingTaskID, taskID)
		}
		if job.OrderNo != wantOrderNo[orderID] {
			t.Errorf("job for order %d: OrderNo = %q, want %q (copied from the ORDER itself, not the shared task)", orderID, job.OrderNo, wantOrderNo[orderID])
		}
		if len(job.Items) != 1 {
			t.Fatalf("job for order %d: len(Items) = %d, want 1", orderID, len(job.Items))
		}
		if job.Items[0].PickedQty != wantQty[orderID] {
			t.Errorf("job for order %d: PickedQty ceiling = %v, want %v (that order's own sorted_qty, never another order's share)", orderID, job.Items[0].PickedQty, wantQty[orderID])
		}
		if job.Items[0].OrderedQty != wantQty[orderID] {
			t.Errorf("job for order %d: OrderedQty = %v, want %v", orderID, job.Items[0].OrderedQty, wantQty[orderID])
		}
	}

	// A second job for the SAME order against the SAME task is rejected
	// — migrations/0102's own UQ_packing_jobs_picking_task_order.
	if _, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID, OrderID: &so1.ID}, 1); !errors.Is(err, ErrAlreadyPacked) {
		t.Fatalf("second Create for same order: err = %v, want ErrAlreadyPacked", err)
	}

	// Each job can now be packed independently, each bounded to its own
	// sorted_qty ceiling — PackItem's own packed_qty <= picked_qty
	// constraint, unchanged, now automatically protects cross-order
	// leakage since picked_qty was frozen from sorted_qty at Create.
	for orderID, job := range jobs {
		carton, err := st.packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, CreateCartonRequest{}, 1)
		if err != nil {
			t.Fatalf("CreateCarton(order=%d): %v", orderID, err)
		}
		packed, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, PackItemRequest{
			IdempotencyKey: "batch3jobs-pack-" + job.OrderNo,
			ItemID:         job.Items[0].ID, CartonID: carton.ID,
			ScannedSKU: sku, ScannedLotNo: line.LotNo, Quantity: wantQty[orderID],
		}, 1)
		if err != nil {
			t.Fatalf("PackItem(order=%d): %v", orderID, err)
		}
		if packed.Items[0].PackedQty != wantQty[orderID] {
			t.Errorf("job for order %d: final PackedQty = %v, want %v", orderID, packed.Items[0].PackedQty, wantQty[orderID])
		}
		// Overpacking beyond that order's own share is still rejected —
		// attempting even 1 more unit must fail.
		if _, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, PackItemRequest{
			IdempotencyKey: "batch3jobs-pack-overflow-" + job.OrderNo,
			ItemID:         job.Items[0].ID, CartonID: carton.ID,
			ScannedSKU: sku, ScannedLotNo: line.LotNo, Quantity: 1,
		}, 1); !errors.Is(err, ErrOverpack) {
			t.Errorf("overpack attempt for order %d: err = %v, want ErrOverpack", orderID, err)
		}
	}
}

// TestService_Create_BatchSourced_RejectsOrderNotInBatch and
// TestService_Create_BatchSourced_RequiresOrderID cover the two
// client-input validation gaps specific to the batch-sourced branch.
func TestService_Create_BatchSourced_RequiresOrderID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := batchFixture(t, tx, "batchreqorder")
	seedPackingBalance(t, tx, f, "LOT-X", 100, 0)
	st := newBatchStack(tx)

	so1 := releasedPickByProductOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := releasedPickByProductOrder(t, st.orderSvc, f.tenantID, f, "B", 30)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("batchSvc.Create: %v", err)
	}
	confirmed, err := st.batchSvc.Confirm(context.Background(), f.tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("batchSvc.Confirm: %v", err)
	}
	taskID := *confirmed.PickingTaskID
	line := confirmed.Lines[0]
	itemID := *line.Allocations[0].PickingTaskItemID

	pickerID := seedPicker(t, tx, f.tenantID, "batchreqorder")
	st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, pickerID, 1)
	st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, itemID, picking.ScanRequest{
		LocationCode: line.LocationCode, SKU: line.SKU, LotNo: line.LotNo,
	}, 1)
	if _, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "batchreqorder-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 50}},
	}, 1); err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}

	if _, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID}, 1); !errors.Is(err, ErrOrderIDRequired) {
		t.Fatalf("Create with no order_id against a batch task: err = %v, want ErrOrderIDRequired", err)
	}

	otherOrderID := so1.ID + 999999
	if _, err := st.packingSvc.Create(context.Background(), f.tenantID, CreateRequest{PickingTaskID: taskID, OrderID: &otherOrderID}, 1); !errors.Is(err, ErrOrderNotInBatch) {
		t.Fatalf("Create with order_id not in batch: err = %v, want ErrOrderNotInBatch", err)
	}
}

// TestService_Create_NonBatch_RejectsOrderID and
// TestService_Create_NonBatch_Unchanged are the hard regression bar:
// the ordinary (non-batch) flow must behave byte-for-byte as before
// Phase 33C.2 — order_id stays nil on the resulting job, and supplying
// one against a plain PICK_BY_ORDER task is rejected outright.
func TestService_Create_NonBatch_Unchanged(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "nb-unchg")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	// Exercise the exact same helper shape this package's own pre-
	// existing tenant_isolation_test.go already uses for the ordinary
	// flow (newPackingServiceAndRepo/createCompletedPickingTask), but
	// with batches wired up too — proving SetBatches being present
	// changes NOTHING about this path.
	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	batchRepo := pickingbatch.NewRepository(tx)
	batchSvc := pickingbatch.NewService(batchRepo, warehouse.NewRepository(tx), pickingSvc)
	svc.SetBatches(batchSvc)

	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)

	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: pickTaskID}, 1)
	if err != nil {
		t.Fatalf("Create (non-batch): %v", err)
	}
	if resp.OrderID != nil {
		t.Errorf("OrderID = %v, want nil for a non-batch job", resp.OrderID)
	}

	// order_id supplied against a non-batch task is rejected.
	someOrderID := int64(1)
	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: pickTaskID, OrderID: &someOrderID}, 1); !errors.Is(err, ErrOrderIDNotAllowed) {
		// This second call also hits ErrAlreadyPacked territory for the
		// same task, but the order_id guard fires BEFORE any duplicate
		// check runs, so ErrOrderIDNotAllowed is the one and only
		// expected error here.
		t.Fatalf("Create with order_id against non-batch task: err = %v, want ErrOrderIDNotAllowed", err)
	}
}
