// Phase 33D.1 — the headline reconciliation scenario this batch exists
// to prove: SO-001(20)/SO-002(30)/SO-003(50) consolidated into one
// batch pick of 100 (internal/pickingbatch, Phase 33C.1), Sortation
// splits the pick 20/30/50 (Phase 33C.2), three independent packing
// jobs are opened and independently checker-confirmed (one per order,
// Phase 33C.2), and THREE independent shipments are created — one per
// packing job/order — each shipment's own item quantities matching
// exactly that order's own packed_qty, with no shipment ever able to
// see another order's quantity. Mirrors internal/packing's own
// TestService_Create_BatchSourced_SO001SO002SO003_ThreeIndependentJobs
// (Phase 33C.2), extended one layer further into Shipping.
//
// Built as an EXTERNAL test package ("shipping_test", not "shipping")
// deliberately: this scenario genuinely needs a real internal/order
// Service alongside internal/shipping's own Service, but internal/order
// itself imports internal/shipping (its own shippingReader dependency,
// see order/service.go) — importing internal/order from one of
// internal/shipping's own INTERNAL test files (package shipping) would
// be a real import cycle (shipping (test) -> order -> shipping). An
// external test package has no such problem: nothing imports
// shipping_test, so shipping_test -> {order, shipping, packing,
// pickingbatch, picking} is a one-directional, acyclic edge regardless
// of what order/shipping/packing import from each other. The trade-off
// is that this file cannot reach internal/shipping's own unexported
// helpers (shippingFixture, newShippingFixture, ...) from
// tenant_isolation_test.go — see its own local fixture below instead,
// the same per-package-own-fixture convention internal/packing's and
// internal/pickingbatch's own batch_integration_test.go/service_test.go
// each already establish.
package shipping_test

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
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

// fixture is one fully-seeded tenant: a product, a warehouse, and a
// location within it — mirrors internal/shipping's own (unexported,
// unreachable from here) shippingFixture.
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
		"ship-batch-"+label, "Shipping Batch Integration Test Tenant "+label,
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

func seedBalance(t *testing.T, tx *gorm.DB, f fixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
}

// batchStack wires order + pickingbatch + picking + packing + shipping
// against one shared tx — the exact graph cmd/server/main.go wires in
// production, including packingSvc.SetBatches(batchSvc) and shipping's
// own packingChecker dependency on packingSvc (Phase 33D.1).
type batchStack struct {
	orderSvc   *order.Service
	batchSvc   *pickingbatch.Service
	pickingSvc *picking.Service
	packingSvc *packing.Service
	shipSvc    *shipping.Service
}

func newBatchStack(tx *gorm.DB) batchStack {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingSvc := picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	pickingSvc.SetUsers(user.NewRepository(tx))

	packingSvc := packing.NewService(packing.NewRepository(tx), pickingSvc)
	packingSvc.SetUsers(user.NewRepository(tx))

	batchSvc := pickingbatch.NewService(pickingbatch.NewRepository(tx), warehouseRepo, pickingSvc)
	packingSvc.SetBatches(batchSvc)

	shipSvc := shipping.NewService(shipping.NewRepository(tx), packingSvc)

	orderRepo := order.NewRepository(tx)
	customerRepo := customer.NewRepository(tx)
	shipToRepo := customerdeliverylocation.NewRepository(tx)
	orderSvc := order.NewService(orderRepo, productRepo, warehouseRepo, customerRepo, shipToRepo, pickingSvc, shipSvc, invSvc)

	return batchStack{orderSvc: orderSvc, batchSvc: batchSvc, pickingSvc: pickingSvc, packingSvc: packingSvc, shipSvc: shipSvc}
}

func releasedPickByProductOrder(t *testing.T, svc *order.Service, tenantID int64, f fixture, customerName string, qty float64) *order.Response {
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

func seedBatchPicker(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	var roleID int64
	if err := tx.Raw(
		"INSERT INTO roles (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "PICKER-"+label, "Picker Role "+label,
	).Scan(&roleID).Error; err != nil {
		t.Fatalf("seedBatchPicker(%s): insert role: %v", label, err)
	}
	var userID int64
	if err := tx.Raw(
		"INSERT INTO users (username, password_hash, full_name, email, role_id) OUTPUT INSERTED.id VALUES (?, 'x', ?, ?, ?)",
		"shippicker-"+label, "Picker "+label, "shippicker-"+label+"@example.com", roleID,
	).Scan(&userID).Error; err != nil {
		t.Fatalf("seedBatchPicker(%s): insert user: %v", label, err)
	}
	return userID
}

// TestService_Create_BatchSourced_SO001SO002SO003_ThreeIndependentShipments
// is this batch's own headline scenario — see package-level doc.
func TestService_Create_BatchSourced_SO001SO002SO003_ThreeIndependentShipments(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "batch3ship")
	seedBalance(t, tx, f, "LOT-X", 100, 0)
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

	pickerID := seedBatchPicker(t, tx, f.tenantID, "batch3ship")
	if _, err := st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, pickerID, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}
	if _, err := st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, itemID, picking.ScanRequest{
		LocationCode: line.LocationCode, SKU: line.SKU, LotNo: line.LotNo,
	}, 1); err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	pickedResp, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "batch3ship-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 100}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}
	if pickedResp.Status != picking.StatusCompleted {
		t.Fatalf("task status = %v, want COMPLETED", pickedResp.Status)
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

	// One independent packing job per order, each packed in full and
	// independently checker-confirmed into COMPLETED.
	wantQty := map[int64]float64{so1.ID: 20, so2.ID: 30, so3.ID: 50}
	wantOrderNo := map[int64]string{so1.ID: so1.OrderNo, so2.ID: so2.OrderNo, so3.ID: so3.OrderNo}
	jobIDs := make(map[int64]int64, 3)
	for _, so := range []*order.Response{so1, so2, so3} {
		job, err := st.packingSvc.Create(context.Background(), f.tenantID, packing.CreateRequest{PickingTaskID: taskID, OrderID: &so.ID}, 1)
		if err != nil {
			t.Fatalf("packing Create(order=%d): %v", so.ID, err)
		}
		carton, err := st.packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
		if err != nil {
			t.Fatalf("CreateCarton(order=%d): %v", so.ID, err)
		}
		packed, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, packing.PackItemRequest{
			IdempotencyKey: "batch3ship-pack-" + job.OrderNo,
			ItemID:         job.Items[0].ID, CartonID: carton.ID,
			ScannedSKU: sku, ScannedLotNo: line.LotNo, Quantity: wantQty[so.ID],
		}, 1)
		if err != nil {
			t.Fatalf("PackItem(order=%d): %v", so.ID, err)
		}
		if packed.Items[0].PackedQty != wantQty[so.ID] {
			t.Fatalf("job for order %d: PackedQty = %v, want %v", so.ID, packed.Items[0].PackedQty, wantQty[so.ID])
		}
		confirmedJob, err := st.packingSvc.CheckerConfirm(context.Background(), f.tenantID, job.ID, packing.CheckerConfirmRequest{}, 1)
		if err != nil {
			t.Fatalf("CheckerConfirm(order=%d): %v", so.ID, err)
		}
		if confirmedJob.Status != packing.StatusCompleted {
			t.Fatalf("job for order %d: status = %v, want COMPLETED", so.ID, confirmedJob.Status)
		}
		jobIDs[so.ID] = job.ID
	}

	// Now the actual point of this batch: three independent shipments,
	// one per packing job/order.
	shipmentIDs := make(map[int64]int64, 3)
	for _, so := range []*order.Response{so1, so2, so3} {
		shipResp, err := st.shipSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{PackingJobID: jobIDs[so.ID]}, 1)
		if err != nil {
			t.Fatalf("shipping Create(order=%d): %v", so.ID, err)
		}
		if shipResp.OrderNo != wantOrderNo[so.ID] {
			t.Errorf("shipment for order %d: OrderNo = %q, want %q (its OWN order, via the packing job's own snapshot, never the batch's shared task)", so.ID, shipResp.OrderNo, wantOrderNo[so.ID])
		}
		if len(shipResp.Items) != 1 {
			t.Fatalf("shipment for order %d: len(Items) = %d, want 1", so.ID, len(shipResp.Items))
		}
		if shipResp.Items[0].Qty != wantQty[so.ID] {
			t.Errorf("shipment for order %d: item qty = %v, want %v — must match ONLY that order's own packed_qty", so.ID, shipResp.Items[0].Qty, wantQty[so.ID])
		}
		shipmentIDs[so.ID] = shipResp.ID
	}

	if shipmentIDs[so1.ID] == shipmentIDs[so2.ID] || shipmentIDs[so2.ID] == shipmentIDs[so3.ID] || shipmentIDs[so1.ID] == shipmentIDs[so3.ID] {
		t.Fatal("expected three DISTINCT shipment ids, one per order")
	}

	// No shipment can ever see another order's quantity: reload each
	// one independently and re-verify.
	for _, so := range []*order.Response{so1, so2, so3} {
		reloaded, err := st.shipSvc.Get(context.Background(), f.tenantID, shipmentIDs[so.ID])
		if err != nil {
			t.Fatalf("Get(shipment for order=%d): %v", so.ID, err)
		}
		if len(reloaded.Items) != 1 || reloaded.Items[0].Qty != wantQty[so.ID] {
			t.Fatalf("reloaded shipment for order %d: items = %+v, want exactly one item of qty %v", so.ID, reloaded.Items, wantQty[so.ID])
		}
		if reloaded.OrderNo != wantOrderNo[so.ID] {
			t.Fatalf("reloaded shipment for order %d: OrderNo = %q, want %q", so.ID, reloaded.OrderNo, wantOrderNo[so.ID])
		}
	}

	// A second shipment against any one of those SAME packing jobs is
	// rejected — migrations/0106's own UQ_shipments_packing_job_id.
	if _, err := st.shipSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{PackingJobID: jobIDs[so1.ID]}, 1); !errors.Is(err, shipping.ErrAlreadyShipped) {
		t.Fatalf("second Create for same packing job: err = %v, want ErrAlreadyShipped", err)
	}
}

// TestService_Create_NonBatch_RegressionAgainstBatchStack is the direct
// regression bar this batch's own instructions call for: plain
// (non-batch) Shipping — one order -> one task -> one packing job -> one
// shipment — must continue to work unchanged, exercised against this
// same full batchStack wiring (order -> picking -> packing -> shipping)
// rather than the narrower (internal, package shipping) test stack, to
// prove nothing about the batch-aware wiring above regresses the
// ordinary path.
func TestService_Create_NonBatch_RegressionAgainstBatchStack(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "nonbatch-regress")
	seedBalance(t, tx, f, "LOT-Y", 50, 0)
	st := newBatchStack(tx)

	so, err := st.orderSvc.Create(context.Background(), f.tenantID, order.CreateRequest{
		WarehouseID: f.warehouseID,
		Items:       []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: 15, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("order Create: %v", err)
	}
	so, err = st.orderSvc.Release(context.Background(), f.tenantID, so.ID, order.ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("order Release: %v", err)
	}
	released, err := st.orderSvc.CreatePickingTask(context.Background(), f.tenantID, so.ID, order.CreatePickingTaskRequest{
		Items: []order.LocationAssignment{{OrderItemID: so.Items[0].ID, LocationID: f.locationID, LotNo: "LOT-Y"}},
	}, 1)
	if err != nil {
		t.Fatalf("order CreatePickingTask: %v", err)
	}
	taskID := *released.PickingTaskID

	taskResp, err := st.pickingSvc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("picking Get: %v", err)
	}
	if _, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "nonbatch-regress-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: taskResp.Items[0].ID, Action: "PICKED", Quantity: 15}},
	}, 1); err != nil {
		t.Fatalf("picking Confirm: %v", err)
	}

	job, err := st.packingSvc.Create(context.Background(), f.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("packing Create: %v", err)
	}
	if job.OrderID != nil {
		t.Fatalf("non-batch job OrderID = %v, want nil", job.OrderID)
	}
	carton, err := st.packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("packing CreateCarton: %v", err)
	}
	if _, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: "nonbatch-regress-pack-1", ItemID: job.Items[0].ID, CartonID: carton.ID,
		ScannedSKU: job.Items[0].SKU, Quantity: 15,
	}, 1); err != nil {
		t.Fatalf("packing PackItem: %v", err)
	}
	if _, err := st.packingSvc.CheckerConfirm(context.Background(), f.tenantID, job.ID, packing.CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("packing CheckerConfirm: %v", err)
	}

	shipResp, err := st.shipSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{PackingJobID: job.ID}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}
	if shipResp.OrderNo != so.OrderNo {
		t.Errorf("shipment OrderNo = %q, want %q", shipResp.OrderNo, so.OrderNo)
	}
	if len(shipResp.Items) != 1 || shipResp.Items[0].Qty != 15 {
		t.Fatalf("shipment items = %+v, want exactly one item of qty 15", shipResp.Items)
	}
	if shipResp.Status != shipping.StatusReady {
		t.Errorf("shipment status = %v, want READY", shipResp.Status)
	}
}
