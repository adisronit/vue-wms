// Phase 33E.2 — Destination Receiving. This file is the actual
// reconciliation proof this batch exists for: a real integration test,
// against the disposable SQL Server, driving a COMPLETE
// INTERNAL_TRANSFER order through the ENTIRE existing pipeline (Create
// -> Release -> CreatePickingTask -> Picking -> Packing -> Shipping ->
// Loading -> Dispatch, all exactly as 33E.1's own
// internal/order/transfer_pipeline_test.go already proved) and THEN
// creating/receiving/confirming a Transfer Receipt at the destination
// — closing the status-window loop end to end.
//
// Duplicates a trimmed copy of internal/order's own fixture/stack
// helpers rather than importing internal/order's unexported test
// helpers across a package boundary (not possible in Go) — the same
// "each package seeds its own fixture" precedent internal/packing's
// own batch_integration_external_test.go and internal/loading's own
// tenant_isolation_test.go (newLoadingStack) already establish for
// their own cross-module pipeline tests.
package transferreceipt

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/customer"
	"wms-backend/internal/customerdeliverylocation"
	"wms-backend/internal/inventory"
	"wms-backend/internal/loading"
	"wms-backend/internal/location"
	"wms-backend/internal/order"
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

// trFixture is one fully-seeded tenant: a product, a SOURCE warehouse
// with a STORAGE location, and a DESTINATION warehouse with a
// RECEIVING (staging) location — real rows via the real repositories,
// not stubs.
type trFixture struct {
	tenantID          int64
	productID         int64
	sourceWarehouseID int64
	sourceLocationID  int64
	destWarehouseID   int64
	destLocationID    int64
}

func newTRFixture(t *testing.T, tx *gorm.DB, label string) trFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"tr-"+label, "Transfer Receipt Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newTRFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newTRFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newTRFixture(%s): seed product: %v", label, err)
	}

	srcWarehouseID := seedWarehouse(t, tx, tenantID, "SW-"+label, "Source Warehouse "+label)
	srcZoneID := seedZone(t, tx, tenantID, srcWarehouseID, "SZ-"+label, "STORAGE")
	srcLocationID := seedLocation(t, tx, tenantID, srcZoneID, "SLOC-"+label)

	destWarehouseID := seedWarehouse(t, tx, tenantID, "DW-"+label, "Destination Warehouse "+label)
	destZoneID := seedZone(t, tx, tenantID, destWarehouseID, "DZ-"+label, "RECEIVING")
	destLocationID := seedLocation(t, tx, tenantID, destZoneID, "DLOC-"+label)

	return trFixture{
		tenantID: tenantID, productID: p.ID,
		sourceWarehouseID: srcWarehouseID, sourceLocationID: srcLocationID,
		destWarehouseID: destWarehouseID, destLocationID: destLocationID,
	}
}

func seedWarehouse(t *testing.T, tx *gorm.DB, tenantID int64, code, name string) int64 {
	t.Helper()
	if len(code) > 20 {
		code = code[:20]
	}
	var id int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, code, name,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seedWarehouse(%s): %v", code, err)
	}
	return id
}

func seedZone(t *testing.T, tx *gorm.DB, tenantID, warehouseID int64, code, zoneType string) int64 {
	t.Helper()
	if len(code) > 20 {
		code = code[:20]
	}
	var id int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, ?, 'ACTIVE')",
		tenantID, warehouseID, code, "Zone "+code, zoneType,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seedZone(%s): %v", code, err)
	}
	return id
}

func seedLocation(t *testing.T, tx *gorm.DB, tenantID, zoneID int64, code string) int64 {
	t.Helper()
	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: code, Name: "Location " + code,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("seedLocation(%s): %v", code, err)
	}
	return l.ID
}

func seedTRBalance(t *testing.T, tx *gorm.DB, tenantID, productID, locationID int64, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: tenantID, ProductID: productID, LocationID: locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedTRBalance: %v", err)
	}
}

// trStack bundles the full real dependency graph a transfer order's
// entire life cycle runs through, plus this module's own Service bound
// to the same transaction — the exact shape cmd/server/main.go wires in
// production, extended with this phase's own wiring.
type trStack struct {
	orderSvc    *order.Service
	pickingSvc  *picking.Service
	packingSvc  *packing.Service
	shippingSvc *shipping.Service
	loadingSvc  *loading.Service
	trSvc       *Service
}

func newTRStack(tx *gorm.DB) trStack {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingSvc := picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	packingSvc := packing.NewService(packing.NewRepository(tx), pickingSvc)
	shippingSvc := shipping.NewService(shipping.NewRepository(tx), packingSvc)
	loadingSvc := loading.NewService(loading.NewRepository(tx), shippingSvc, packingSvc)

	orderRepo := order.NewRepository(tx)
	customerRepo := customer.NewRepository(tx)
	shipToRepo := customerdeliverylocation.NewRepository(tx)
	orderSvc := order.NewService(orderRepo, productRepo, warehouseRepo, customerRepo, shipToRepo, pickingSvc, shippingSvc, invSvc)

	trRepo := NewRepository(tx)
	trSvc := NewService(trRepo, shippingSvc, orderRepo, invSvc)

	return trStack{orderSvc: orderSvc, pickingSvc: pickingSvc, packingSvc: packingSvc, shippingSvc: shippingSvc, loadingSvc: loadingSvc, trSvc: trSvc}
}

func strPtr(s string) *string { return &s }

// sumInventoryQty sums inventory.quantity for (tenantID, productID,
// warehouseID) — used to assert per-warehouse balances before/after
// receipt.
func sumInventoryQtyAtWarehouse(t *testing.T, tx *gorm.DB, tenantID, productID, warehouseID int64) float64 {
	t.Helper()
	var total float64
	if err := tx.Raw(
		`SELECT ISNULL(SUM(i.quantity), 0) FROM inventory i
		 JOIN locations l ON l.id = i.location_id JOIN zones z ON z.id = l.zone_id
		 WHERE i.tenant_id = ? AND i.product_id = ? AND z.warehouse_id = ?`,
		tenantID, productID, warehouseID,
	).Scan(&total).Error; err != nil {
		t.Fatalf("sumInventoryQtyAtWarehouse: %v", err)
	}
	return total
}

func countLedgerByType(t *testing.T, tx *gorm.DB, tenantID, productID int64) map[string]int64 {
	t.Helper()
	byType := map[string]int64{}
	var rows []struct {
		TransactionType string
		N               int64
	}
	if err := tx.Raw(
		"SELECT transaction_type AS transaction_type, COUNT(*) AS n FROM inventory_transactions WHERE tenant_id = ? AND product_id = ? GROUP BY transaction_type",
		tenantID, productID,
	).Scan(&rows).Error; err != nil {
		t.Fatalf("countLedgerByType: %v", err)
	}
	for _, r := range rows {
		byType[r.TransactionType] = r.N
	}
	return byType
}

// runTransferToShipment drives Create -> Release -> CreatePickingTask
// -> Picking -> Packing -> Shipping(Create) -> Loading -> Dispatch for
// one INTERNAL_TRANSFER order, exactly mirroring internal/order's own
// TestInternalTransfer_FullPipeline_UnmodifiedExistingWorkflow, and
// returns the dispatched shipment's own ID — the real starting point
// for this phase's own work.
func runTransferToShipment(t *testing.T, tx *gorm.DB, st trStack, f trFixture, lotNo string, startingQty, transferQty float64, idemPrefix string) int64 {
	t.Helper()

	seedTRBalance(t, tx, f.tenantID, f.productID, f.sourceLocationID, lotNo, startingQty, 0)

	created, err := st.orderSvc.Create(context.Background(), f.tenantID, order.CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.sourceWarehouseID,
		DestinationWarehouseID: &f.destWarehouseID, TransferReference: strPtr("TRF-" + idemPrefix),
		Items: []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: transferQty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("order Create: %v", err)
	}

	if _, err := st.orderSvc.Release(context.Background(), f.tenantID, created.ID, order.ReleaseRequest{}, 1); err != nil {
		t.Fatalf("order Release: %v", err)
	}

	itemID := created.Items[0].ID
	afterCreateTask, err := st.orderSvc.CreatePickingTask(context.Background(), f.tenantID, created.ID, order.CreatePickingTaskRequest{
		Items: []order.LocationAssignment{{OrderItemID: itemID, LocationID: f.sourceLocationID, LotNo: lotNo}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask: %v", err)
	}
	taskID := *afterCreateTask.PickingTaskID

	pickTask, err := st.pickingSvc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("picking Get: %v", err)
	}
	pickItem := pickTask.Items[0]
	if _, err := st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, 1, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}
	if _, err := st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, pickItem.ID, picking.ScanRequest{
		LocationCode: pickItem.LocationCode, SKU: pickItem.SKU, LotNo: pickItem.LotNo,
	}, 1); err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	if _, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: idemPrefix + "-picked",
		Items:          []picking.ConfirmItemRequest{{ItemID: pickItem.ID, Action: "PICKED", Quantity: transferQty}},
	}, 1); err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}

	job, err := st.packingSvc.Create(context.Background(), f.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("packing Create: %v", err)
	}
	carton, err := st.packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	if _, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: idemPrefix + "-pack", ItemID: job.Items[0].ID, CartonID: carton.ID,
		ScannedSKU: job.Items[0].SKU, Quantity: job.Items[0].PickedQty,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}
	if _, err := st.packingSvc.CheckerConfirm(context.Background(), f.tenantID, job.ID, packing.CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}

	carrier, tracking := "INTERNAL", "XFER-"+created.OrderNo
	shipResp, err := st.shippingSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{
		PackingJobID: job.ID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}

	trip, err := st.loadingSvc.CreateTrip(context.Background(), f.tenantID, loading.CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	assigned, err := st.loadingSvc.AssignShipment(context.Background(), f.tenantID, trip.ID, loading.AssignShipmentRequest{ShipmentID: shipResp.ID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	for _, c := range assigned.Cartons {
		if _, err := st.loadingSvc.ScanCarton(context.Background(), f.tenantID, trip.ID, loading.ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}
	if _, err := st.loadingSvc.ConfirmTrip(context.Background(), f.tenantID, trip.ID, loading.ConfirmRequest{}, 1); err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}

	dispatched, err := st.shippingSvc.Confirm(context.Background(), f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: idemPrefix + "-dispatch", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DISPATCH): %v", err)
	}
	if dispatched.Status != shipping.StatusDispatched {
		t.Fatalf("shipment status = %v, want DISPATCHED", dispatched.Status)
	}
	return shipResp.ID
}

// TestTransferReceipt_FullPipeline_ReconciliationProof is this batch's
// own headline proof: a complete INTERNAL_TRANSFER order driven all
// the way through Dispatch (33E.1's own pipeline, unmodified), then a
// Transfer Receipt created/received/confirmed at the destination,
// asserting the full reconciliation: before receipt, destination has
// zero balance; after receipt, destination's staging location holds
// exactly the received quantity, a TRANSFER_RECEIVE ledger row exists,
// and the SOURCE warehouse is completely unaffected by the receipt
// (its own balance already dropped at pick time).
func TestTransferReceipt_FullPipeline_ReconciliationProof(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "recon")
	st := newTRStack(tx)
	const startingQty = 100.0
	const transferQty = 40.0

	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-RECON", startingQty, transferQty, "recon")

	// --- Before receipt: destination warehouse has zero balance for the
	// transferred product; source has already dropped by transferQty
	// (33E.1's own proof, re-asserted here as the starting point). ---
	if got := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.destWarehouseID); got != 0 {
		t.Fatalf("before receipt: destination warehouse quantity = %v, want 0", got)
	}
	sourceBefore := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.sourceWarehouseID)
	if sourceBefore != startingQty-transferQty {
		t.Fatalf("before receipt: source warehouse quantity = %v, want %v", sourceBefore, startingQty-transferQty)
	}

	// --- Create the transfer receipt from the dispatched shipment. ---
	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("transferreceipt Create: %v", err)
	}
	if receipt.Status != StatusDraft {
		t.Fatalf("receipt status = %v, want DRAFT", receipt.Status)
	}
	if len(receipt.Items) != 1 {
		t.Fatalf("receipt items = %d, want 1", len(receipt.Items))
	}
	lineID := receipt.Items[0].ID
	if receipt.Items[0].DispatchedQty != transferQty {
		t.Fatalf("receipt line dispatched_qty = %v, want %v", receipt.Items[0].DispatchedQty, transferQty)
	}

	// --- Receive the full quantity into the destination staging
	// location. ---
	received, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, lineID, ReceiveItemRequest{
		IdempotencyKey: "recon-receive-1", LocationID: f.destLocationID, Quantity: transferQty,
	}, 1)
	if err != nil {
		t.Fatalf("ReceiveItem: %v", err)
	}
	if received.Items[0].Status != ItemStatusCompleted {
		t.Fatalf("line status = %v, want COMPLETED", received.Items[0].Status)
	}

	// --- Confirm the receipt. ---
	confirmed, err := st.trSvc.Confirm(context.Background(), f.tenantID, receipt.ID, ConfirmRequest{IdempotencyKey: "recon-confirm-1"}, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status != StatusCompleted {
		t.Fatalf("receipt status = %v, want COMPLETED", confirmed.Status)
	}
	if confirmed.ReceivedBy == nil || *confirmed.ReceivedBy != 1 {
		t.Fatalf("receipt ReceivedBy = %v, want 1", confirmed.ReceivedBy)
	}

	// --- After receipt: destination's staging location increased by
	// exactly the received quantity. ---
	var destBalance float64
	if err := tx.Raw(
		"SELECT ISNULL(SUM(quantity), 0) FROM inventory WHERE tenant_id = ? AND product_id = ? AND location_id = ?",
		f.tenantID, f.productID, f.destLocationID,
	).Scan(&destBalance).Error; err != nil {
		t.Fatalf("destBalance query: %v", err)
	}
	if destBalance != transferQty {
		t.Fatalf("destination staging location balance = %v, want %v", destBalance, transferQty)
	}

	// --- A TRANSFER_RECEIVE ledger row exists with the correct
	// quantity. ---
	var ledgerQty float64
	var ledgerCount int64
	if err := tx.Raw(
		"SELECT COUNT(*), ISNULL(SUM(quantity), 0) FROM inventory_transactions WHERE tenant_id = ? AND product_id = ? AND transaction_type = 'TRANSFER_RECEIVE'",
		f.tenantID, f.productID,
	).Row().Scan(&ledgerCount, &ledgerQty); err != nil {
		t.Fatalf("ledger query: %v", err)
	}
	if ledgerCount != 1 || ledgerQty != transferQty {
		t.Fatalf("TRANSFER_RECEIVE ledger: count=%d qty=%v, want count=1 qty=%v", ledgerCount, ledgerQty, transferQty)
	}

	// --- The SOURCE warehouse's balance is completely unaffected by the
	// receipt — still exactly what it was right after PICKED, and the
	// only ledger rows for this product are PICK (source debit) and
	// TRANSFER_RECEIVE (destination credit): no third type appeared. ---
	sourceAfter := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.sourceWarehouseID)
	if sourceAfter != sourceBefore {
		t.Fatalf("source warehouse quantity after receipt = %v, want unchanged %v", sourceAfter, sourceBefore)
	}
	byType := countLedgerByType(t, tx, f.tenantID, f.productID)
	if len(byType) != 2 || byType["PICK"] != 1 || byType["TRANSFER_RECEIVE"] != 1 {
		t.Fatalf("ledger rows by type = %v, want exactly {PICK: 1, TRANSFER_RECEIVE: 1}", byType)
	}
}

// TestTransferReceipt_ShortReceipt_DiscrepancyThenConfirm covers the
// short-receipt path: received < dispatched, closed with a discrepancy
// reason, and Confirm still succeeds once every line is settled.
func TestTransferReceipt_ShortReceipt_DiscrepancyThenConfirm(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "short")
	st := newTRStack(tx)
	const startingQty = 100.0
	const transferQty = 50.0
	const actuallyReceived = 30.0

	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-SHORT", startingQty, transferQty, "short")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lineID := receipt.Items[0].ID

	// Confirm is blocked until the line is settled.
	if _, err := st.trSvc.Confirm(context.Background(), f.tenantID, receipt.ID, ConfirmRequest{IdempotencyKey: "short-confirm-early"}, 1); !errors.Is(err, ErrNotReadyToConfirm) {
		t.Fatalf("Confirm before any line settled = %v, want ErrNotReadyToConfirm", err)
	}

	// Receive a partial quantity and close the line short in the same call.
	received, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, lineID, ReceiveItemRequest{
		IdempotencyKey: "short-receive-1", LocationID: f.destLocationID, Quantity: actuallyReceived,
		CloseShort: true, DiscrepancyReason: "Two cartons damaged in transit",
	}, 1)
	if err != nil {
		t.Fatalf("ReceiveItem(close short): %v", err)
	}
	line := received.Items[0]
	if line.Status != ItemStatusDiscrepancy {
		t.Fatalf("line status = %v, want DISCREPANCY", line.Status)
	}
	if line.DiscrepancyQty == nil || *line.DiscrepancyQty != transferQty-actuallyReceived {
		t.Fatalf("discrepancy_qty = %v, want %v", line.DiscrepancyQty, transferQty-actuallyReceived)
	}
	if line.DiscrepancyReason == nil || *line.DiscrepancyReason == "" {
		t.Fatalf("discrepancy_reason = %v, want a non-empty reason", line.DiscrepancyReason)
	}
	if line.ReceivedQty != actuallyReceived {
		t.Fatalf("received_qty = %v, want %v", line.ReceivedQty, actuallyReceived)
	}

	// Confirm now succeeds — every line is settled (DISCREPANCY counts).
	confirmed, err := st.trSvc.Confirm(context.Background(), f.tenantID, receipt.ID, ConfirmRequest{IdempotencyKey: "short-confirm-1"}, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status != StatusCompleted {
		t.Fatalf("receipt status = %v, want COMPLETED", confirmed.Status)
	}

	// The destination only ever received the actual (short) quantity.
	destBalance := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.destWarehouseID)
	if destBalance != actuallyReceived {
		t.Fatalf("destination warehouse quantity = %v, want %v", destBalance, actuallyReceived)
	}
}

// TestTransferReceipt_Create_RejectsSalesOrderShipment covers "only
// INTERNAL_TRANSFER shipments may be receipted this way" — a
// SALES_ORDER-sourced shipment, driven through the identical pipeline,
// is rejected.
func TestTransferReceipt_Create_RejectsSalesOrderShipment(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "sales")
	st := newTRStack(tx)
	seedTRBalance(t, tx, f.tenantID, f.productID, f.sourceLocationID, "LOT-SALES", 100, 0)

	created, err := st.orderSvc.Create(context.Background(), f.tenantID, order.CreateRequest{
		WarehouseID: f.sourceWarehouseID,
		Items:       []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: 20, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("order Create: %v", err)
	}
	if _, err := st.orderSvc.Release(context.Background(), f.tenantID, created.ID, order.ReleaseRequest{}, 1); err != nil {
		t.Fatalf("Release: %v", err)
	}
	itemID := created.Items[0].ID
	afterCreateTask, err := st.orderSvc.CreatePickingTask(context.Background(), f.tenantID, created.ID, order.CreatePickingTaskRequest{
		Items: []order.LocationAssignment{{OrderItemID: itemID, LocationID: f.sourceLocationID, LotNo: "LOT-SALES"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask: %v", err)
	}
	taskID := *afterCreateTask.PickingTaskID
	pickTask, _ := st.pickingSvc.Get(context.Background(), f.tenantID, taskID)
	pickItem := pickTask.Items[0]
	st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, 1, 1)
	st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, pickItem.ID, picking.ScanRequest{
		LocationCode: pickItem.LocationCode, SKU: pickItem.SKU, LotNo: pickItem.LotNo,
	}, 1)
	if _, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "sales-picked", Items: []picking.ConfirmItemRequest{{ItemID: pickItem.ID, Action: "PICKED", Quantity: 20}},
	}, 1); err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}
	job, err := st.packingSvc.Create(context.Background(), f.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("packing Create: %v", err)
	}
	carton, _ := st.packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if _, err := st.packingSvc.PackItem(context.Background(), f.tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: "sales-pack", ItemID: job.Items[0].ID, CartonID: carton.ID, ScannedSKU: job.Items[0].SKU, Quantity: job.Items[0].PickedQty,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}
	if _, err := st.packingSvc.CheckerConfirm(context.Background(), f.tenantID, job.ID, packing.CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}
	carrier, tracking := "INTERNAL", "SALES-"+created.OrderNo
	shipResp, err := st.shippingSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{
		PackingJobID: job.ID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}
	trip, _ := st.loadingSvc.CreateTrip(context.Background(), f.tenantID, loading.CreateRequest{}, 1)
	assigned, _ := st.loadingSvc.AssignShipment(context.Background(), f.tenantID, trip.ID, loading.AssignShipmentRequest{ShipmentID: shipResp.ID}, 1)
	for _, c := range assigned.Cartons {
		st.loadingSvc.ScanCarton(context.Background(), f.tenantID, trip.ID, loading.ScanCartonRequest{LPN: c.LPN}, 1)
	}
	st.loadingSvc.ConfirmTrip(context.Background(), f.tenantID, trip.ID, loading.ConfirmRequest{}, 1)
	dispatched, err := st.shippingSvc.Confirm(context.Background(), f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: "sales-dispatch", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DISPATCH): %v", err)
	}

	_, err = st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: dispatched.ID}, 1)
	if !errors.Is(err, ErrNotTransferOrder) {
		t.Fatalf("Create(sales-order shipment) = %v, want ErrNotTransferOrder", err)
	}
}

// TestTransferReceipt_Create_RejectsDuplicateReceipt covers the
// one-receipt-per-shipment rule.
func TestTransferReceipt_Create_RejectsDuplicateReceipt(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "dup")
	st := newTRStack(tx)
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-DUP", 100, 25, "dup")

	if _, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if !errors.Is(err, ErrReceiptAlreadyExists) {
		t.Fatalf("second Create = %v, want ErrReceiptAlreadyExists", err)
	}
}

// TestTransferReceipt_Create_RejectsCrossTenantShipment is the real
// security-boundary test: a shipment_id belonging to a DIFFERENT
// tenant must be rejected exactly like a nonexistent one.
func TestTransferReceipt_Create_RejectsCrossTenantShipment(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTRFixture(t, tx, "cross-a")
	b := newTRFixture(t, tx, "cross-b")
	stA := newTRStack(tx)
	stB := newTRStack(tx)

	shipmentIDOnB := runTransferToShipment(t, tx, stB, b, "LOT-CROSS-B", 100, 25, "cross-b")

	_, err := stA.trSvc.Create(context.Background(), a.tenantID, CreateRequest{ShipmentID: shipmentIDOnB}, 1)
	if !errors.Is(err, ErrShipmentNotFound) {
		t.Fatalf("Create(shipment from another tenant) = %v, want ErrShipmentNotFound", err)
	}
}

// TestTransferReceipt_ReceiveItem_RejectsOverReceipt covers "over-
// receipt beyond dispatched_qty is rejected, not clamped."
func TestTransferReceipt_ReceiveItem_RejectsOverReceipt(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "over")
	st := newTRStack(tx)
	const transferQty = 30.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-OVER", 100, transferQty, "over")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lineID := receipt.Items[0].ID

	_, err = st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, lineID, ReceiveItemRequest{
		IdempotencyKey: "over-receive-1", LocationID: f.destLocationID, Quantity: transferQty + 1,
	}, 1)
	if !errors.Is(err, ErrOverReceipt) {
		t.Fatalf("ReceiveItem(over dispatched_qty) = %v, want ErrOverReceipt", err)
	}

	// The line itself is untouched — no partial credit for a rejected
	// over-receipt attempt.
	got, err := st.trSvc.Get(context.Background(), f.tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Items[0].ReceivedQty != 0 {
		t.Fatalf("received_qty after rejected over-receipt = %v, want 0", got.Items[0].ReceivedQty)
	}
}

// TestTransferReceipt_ReceiveItem_IdempotentRetry covers resending the
// same idempotency_key: returns the current state rather than adding
// stock a second time.
func TestTransferReceipt_ReceiveItem_IdempotentRetry(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "idem")
	st := newTRStack(tx)
	const transferQty = 20.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-IDEM", 100, transferQty, "idem")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lineID := receipt.Items[0].ID

	req := ReceiveItemRequest{IdempotencyKey: "idem-key-1", LocationID: f.destLocationID, Quantity: transferQty}
	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, lineID, req, 1); err != nil {
		t.Fatalf("first ReceiveItem: %v", err)
	}
	// Resend the exact same key.
	got, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, lineID, req, 1)
	if err != nil {
		t.Fatalf("retried ReceiveItem: %v", err)
	}
	if got.Items[0].ReceivedQty != transferQty {
		t.Fatalf("received_qty after retried idempotency key = %v, want unchanged %v", got.Items[0].ReceivedQty, transferQty)
	}
	destBalance := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.destWarehouseID)
	if destBalance != transferQty {
		t.Fatalf("destination balance after retried key = %v, want %v (no double-add)", destBalance, transferQty)
	}
}
