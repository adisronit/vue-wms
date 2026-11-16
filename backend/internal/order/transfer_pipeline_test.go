// Phase 33E.1 — Inter-Warehouse Transfer, step 1 of 2. This file is the
// actual point of this batch: a real integration test, against the
// disposable SQL Server, driving a COMPLETE INTERNAL_TRANSFER order
// through the EXISTING pipeline (Release -> CreatePickingTask ->
// Picking -> Packing -> Shipping -> Loading -> Dispatch), completely
// UNMODIFIED — proving the already-made design decision holds: an
// inter-warehouse transfer's stock is a pure status-window over the
// transfer order's own status plus its linked shipment/loading
// records, never a materialized "in-transit" inventory balance. Stock
// simply leaves the SOURCE warehouse's ledger at Picking.Confirm
// (PICKED) time — the exact same inventory.ConsumeReservationTx call,
// writing the exact same single PICK ledger row, that an equivalent
// sales order would produce — and nothing in this phase, nor in
// internal/picking/internal/packing/internal/shipping/internal/loading
// themselves, writes any corresponding credit anywhere. (33E.2, not
// this batch, is what will later add Destination Receiving as its own
// new action, crediting the destination warehouse via inventory.
// AddStockTx once goods physically arrive.)
//
// Everything below reuses this package's own orderFixture/
// newOrderFixture/seedOrderBalance (tenant_isolation_test.go) and wires
// the exact same production dependency graph cmd/server/main.go wires,
// extended with internal/packing, internal/shipping and internal/
// loading — the same "full real stack, one shared tx" shape internal/
// loading's own tenant_isolation_test.go (newLoadingStack) and
// internal/packing's own batch_integration_external_test.go
// (newBatchStack) already establish for their own cross-module
// pipeline tests.
package order

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
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

// newDestinationWarehouse seeds a second warehouse row in the SAME
// tenant as f — a transfer's own destination never needs a seeded
// zone/location in this phase: Destination Receiving (33E.2, which
// would need one) is explicitly out of scope here.
func newDestinationWarehouse(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	// warehouses.code is VARCHAR(20) — the "D-" prefix plus an 18-char
	// label comfortably fits every caller below.
	code := "D-" + label
	if len(code) > 20 {
		code = code[:20]
	}
	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, code, "Destination Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newDestinationWarehouse(%s): %v", label, err)
	}
	return warehouseID
}

// newTransferPipelineStack wires the full real stack an INTERNAL_
// TRANSFER order is proven against below — order.Service plus every
// downstream workflow package's own Service (picking, packing,
// shipping, loading), all bound to the same test transaction, exactly
// the dependency graph cmd/server/main.go wires in production. No
// SetUsers call on pickingSvc/packingSvc — AssignPicker/AssignPacker
// both nil-check their own optional userChecker dependency, the same
// "users is only ever wired in production" precedent internal/loading's
// own newPickingServiceForLoading/newPackingServiceForLoading already
// establish.
func newTransferPipelineStack(tx *gorm.DB) (orderSvc *Service, pickingSvc *picking.Service, packingSvc *packing.Service, shippingSvc *shipping.Service, loadingSvc *loading.Service) {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingSvc = picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	packingSvc = packing.NewService(packing.NewRepository(tx), pickingSvc)
	shippingSvc = shipping.NewService(shipping.NewRepository(tx), packingSvc)
	loadingSvc = loading.NewService(loading.NewRepository(tx), shippingSvc, packingSvc)

	orderRepo := NewRepository(tx)
	customerRepo := customer.NewRepository(tx)
	shipToRepo := customerdeliverylocation.NewRepository(tx)
	orderSvc = NewService(orderRepo, productRepo, warehouseRepo, customerRepo, shipToRepo, pickingSvc, shippingSvc, invSvc)
	return
}

func strPtrTransfer(s string) *string { return &s }

// countInventoryTransactions is a small assertion helper: total rows
// in inventory_transactions for (tenantID, productID), and how many of
// those are NOT the given expected type — used below to assert "only
// ever one PICK row, never anything else", i.e. no credit anywhere.
func countInventoryTransactions(t *testing.T, tx *gorm.DB, tenantID, productID int64) (total int64, byType map[string]int64) {
	t.Helper()
	byType = map[string]int64{}
	var rows []struct {
		TransactionType string
		N               int64
	}
	if err := tx.Raw(
		"SELECT transaction_type AS transaction_type, COUNT(*) AS n FROM inventory_transactions WHERE tenant_id = ? AND product_id = ? GROUP BY transaction_type",
		tenantID, productID,
	).Scan(&rows).Error; err != nil {
		t.Fatalf("countInventoryTransactions: %v", err)
	}
	for _, r := range rows {
		byType[r.TransactionType] = r.N
		total += r.N
	}
	return total, byType
}

// sumInventoryQuantity sums inventory.quantity for (tenantID,
// productID) across every location — used below to assert "warehouse-
// wide total dropped by exactly the transferred quantity, nowhere
// else, no offsetting credit at any other location/warehouse".
func sumInventoryQuantity(t *testing.T, tx *gorm.DB, tenantID, productID int64) float64 {
	t.Helper()
	var total float64
	if err := tx.Raw(
		"SELECT ISNULL(SUM(quantity), 0) FROM inventory WHERE tenant_id = ? AND product_id = ?",
		tenantID, productID,
	).Scan(&total).Error; err != nil {
		t.Fatalf("sumInventoryQuantity: %v", err)
	}
	return total
}

// TestInternalTransfer_FullPipeline_UnmodifiedExistingWorkflow is
// Phase 33E.1's own headline proof: Create (INTERNAL_TRANSFER, with
// destination_warehouse_id) -> Release -> CreatePickingTask -> Picking
// (assign, scan, confirm PICKED) -> Packing (create job, create
// carton, pack item, checker-confirm) -> Shipping (create from the
// completed packing job) -> Loading (create trip, assign shipment,
// scan carton, confirm trip) -> Shipping.Confirm(DISPATCHED), gated by
// Loading per 33D.2 — all the way through, calling every one of
// internal/picking's/internal/packing's/internal/shipping's/internal/
// loading's own methods completely unmodified, exactly as an
// equivalent SALES_ORDER would.
func TestInternalTransfer_FullPipeline_UnmodifiedExistingWorkflow(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-happy")
	destWarehouseID := newDestinationWarehouse(t, tx, f.tenantID, "xfer-happy")
	const startingQty = 100.0
	const transferQty = 40.0
	seedOrderBalance(t, tx, f, "LOT-XFER", startingQty, 0)

	orderSvc, pickingSvc, packingSvc, shippingSvc, loadingSvc := newTransferPipelineStack(tx)

	// --- Create: INTERNAL_TRANSFER with a destination warehouse, no
	// customer at all (a transfer order simply omits it). ---
	created, err := orderSvc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID,
		DestinationWarehouseID: &destWarehouseID, TransferReference: strPtrTransfer("TRF-0001"),
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: transferQty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.OrderType != "INTERNAL_TRANSFER" {
		t.Fatalf("Create: OrderType = %q, want INTERNAL_TRANSFER", created.OrderType)
	}
	if created.DestinationWarehouseID == nil || *created.DestinationWarehouseID != destWarehouseID {
		t.Fatalf("Create: DestinationWarehouseID = %v, want %d", created.DestinationWarehouseID, destWarehouseID)
	}
	if created.TransferReference == nil || *created.TransferReference != "TRF-0001" {
		t.Fatalf("Create: TransferReference = %v, want TRF-0001", created.TransferReference)
	}
	if created.Customer != nil || created.CustomerID != nil {
		t.Fatalf("Create: Customer/CustomerID = %v/%v, want both nil for a transfer order", created.Customer, created.CustomerID)
	}

	// --- Release: availability check against the SOURCE warehouse. ---
	released, err := orderSvc.Release(context.Background(), f.tenantID, created.ID, ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if released.Status != StatusReleased {
		t.Fatalf("Release: Status = %v, want RELEASED", released.Status)
	}

	// --- CreatePickingTask: the orchestration call, unmodified. ---
	itemID := created.Items[0].ID
	afterCreateTask, err := orderSvc.CreatePickingTask(context.Background(), f.tenantID, created.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: itemID, LocationID: f.locationID, LotNo: "LOT-XFER"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask: %v", err)
	}
	if afterCreateTask.PickingTaskID == nil {
		t.Fatalf("CreatePickingTask: PickingTaskID is nil")
	}
	taskID := *afterCreateTask.PickingTaskID

	// At this point the reservation step (picking.Service.CreateTx's own
	// ReserveStockTx) has run; the warehouse-wide total must be
	// unchanged — only reserved_quantity moves, never quantity itself.
	if got := sumInventoryQuantity(t, tx, f.tenantID, f.productID); got != startingQty {
		t.Fatalf("after CreatePickingTask (reservation only): total quantity = %v, want unchanged %v", got, startingQty)
	}

	// --- Picking: assign, scan, confirm PICKED — exactly as for a
	// sales order; internal/picking has no order_type awareness at
	// all. ---
	pickTask, err := pickingSvc.Get(context.Background(), f.tenantID, taskID)
	if err != nil {
		t.Fatalf("picking Get: %v", err)
	}
	pickItem := pickTask.Items[0]
	if _, err := pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, 1, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}
	if _, err := pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, pickItem.ID, picking.ScanRequest{
		LocationCode: pickItem.LocationCode, SKU: pickItem.SKU, LotNo: pickItem.LotNo,
	}, 1); err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	pickedResp, err := pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "xfer-happy-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: pickItem.ID, Action: "PICKED", Quantity: transferQty}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}
	if pickedResp.Status != picking.StatusCompleted {
		t.Fatalf("picking task status = %v, want COMPLETED", pickedResp.Status)
	}

	// --- The status-window assertion, right where it matters: stock
	// has already left the SOURCE warehouse's ledger, exactly as it
	// would for a sales order, via the exact same ConsumeReservationTx
	// call internal/picking's own Confirm(PICKED) always makes — no
	// code path in this phase, nor in internal/picking itself, was
	// touched to make this happen. ---
	sourceBalance, err := inventorySvcBalance(tx, f.tenantID, f.productID, f.locationID, "LOT-XFER")
	if err != nil {
		t.Fatalf("sourceBalance: %v", err)
	}
	if sourceBalance.Quantity != startingQty-transferQty {
		t.Fatalf("source location quantity after PICKED = %v, want %v", sourceBalance.Quantity, startingQty-transferQty)
	}
	if sourceBalance.ReservedQuantity != 0 {
		t.Fatalf("source location reserved_quantity after PICKED = %v, want 0 (fully consumed)", sourceBalance.ReservedQuantity)
	}
	if got := sumInventoryQuantity(t, tx, f.tenantID, f.productID); got != startingQty-transferQty {
		t.Fatalf("warehouse-wide total quantity after PICKED = %v, want %v (no credit anywhere else)", got, startingQty-transferQty)
	}
	total, byType := countInventoryTransactions(t, tx, f.tenantID, f.productID)
	if total != 1 {
		t.Fatalf("inventory_transactions row count = %d, want exactly 1 (the PICK row) — %v", total, byType)
	}
	if byType["PICK"] != 1 {
		t.Fatalf("inventory_transactions by type = %v, want exactly {PICK: 1} — no RECEIVE/ADJUST/other row, and critically no credit-shaped row at all", byType)
	}

	// --- Packing: create job, create carton, pack item, checker-
	// confirm — unmodified, no order_type/customer awareness anywhere
	// in internal/packing. ---
	job, err := packingSvc.Create(context.Background(), f.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("packing Create: %v", err)
	}
	carton, err := packingSvc.CreateCarton(context.Background(), f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	packed, err := packingSvc.PackItem(context.Background(), f.tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: "xfer-happy-pack-1", ItemID: job.Items[0].ID, CartonID: carton.ID,
		ScannedSKU: job.Items[0].SKU, Quantity: job.Items[0].PickedQty,
	}, 1)
	if err != nil {
		t.Fatalf("PackItem: %v", err)
	}
	confirmedPack, err := packingSvc.CheckerConfirm(context.Background(), f.tenantID, packed.ID, packing.CheckerConfirmRequest{}, 1)
	if err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}
	if confirmedPack.Status != packing.StatusCompleted {
		t.Fatalf("packing job status = %v, want COMPLETED", confirmedPack.Status)
	}

	// --- Shipping: create from the completed packing job — no
	// customer_id/ship_to_id required (both nil on this order, and
	// internal/shipping never required them in the first place).
	// Carrier/TrackingNo are set here only because Service.Confirm's
	// own DISPATCH branch requires them already set (unrelated to this
	// phase — the identical requirement a sales order's shipment would
	// also hit). ---
	carrier, tracking := "INTERNAL", "XFER-"+created.OrderNo
	shipResp, err := shippingSvc.Create(context.Background(), f.tenantID, shipping.CreateRequest{
		PackingJobID: job.ID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}
	if shipResp.Status != shipping.StatusReady {
		t.Fatalf("shipment status = %v, want READY", shipResp.Status)
	}

	// --- Dispatch is blocked until Loading — the same 33D.2 gate a
	// sales order's own shipment would hit. ---
	if _, err := shippingSvc.Confirm(context.Background(), f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: "xfer-happy-early-dispatch", Action: "DISPATCH",
	}, 1); !errors.Is(err, shipping.ErrNotLoaded) {
		t.Fatalf("Confirm(DISPATCH) before Loading = %v, want shipping.ErrNotLoaded", err)
	}

	// --- Loading: create trip, assign shipment, scan every planned
	// carton, confirm trip — unmodified. ---
	trip, err := loadingSvc.CreateTrip(context.Background(), f.tenantID, loading.CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	assigned, err := loadingSvc.AssignShipment(context.Background(), f.tenantID, trip.ID, loading.AssignShipmentRequest{ShipmentID: shipResp.ID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	for _, c := range assigned.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), f.tenantID, trip.ID, loading.ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}
	confirmedTrip, err := loadingSvc.ConfirmTrip(context.Background(), f.tenantID, trip.ID, loading.ConfirmRequest{}, 1)
	if err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}
	if confirmedTrip.Status != loading.StatusCompleted {
		t.Fatalf("trip status = %v, want COMPLETED", confirmedTrip.Status)
	}

	// --- Dispatch: now allowed, since Loading's own ConfirmTrip just
	// called shipping.Service.MarkLoaded for this shipment. ---
	dispatched, err := shippingSvc.Confirm(context.Background(), f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: "xfer-happy-dispatch-1", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DISPATCH) after Loading cycle = %v, want success", err)
	}
	if dispatched.Status != shipping.StatusDispatched {
		t.Fatalf("shipment status after DISPATCH = %v, want DISPATCHED", dispatched.Status)
	}

	// --- Final ledger assertions: the pipeline's only inventory effect,
	// start to finish, is the single PICK row already asserted above —
	// Packing/Shipping/Loading/Dispatch write ZERO additional inventory
	// rows and change ZERO inventory balances (they operate purely on
	// their own tables). Re-asserted here, after the full pipeline, to
	// prove the status-window decision holds all the way through: "in
	// transit" was represented ENTIRELY by this order's own status plus
	// shipment/trip status, never by any inventory row anywhere. ---
	if got := sumInventoryQuantity(t, tx, f.tenantID, f.productID); got != startingQty-transferQty {
		t.Fatalf("final warehouse-wide total quantity = %v, want %v (still no credit anywhere, even after full dispatch)", got, startingQty-transferQty)
	}
	finalTotal, finalByType := countInventoryTransactions(t, tx, f.tenantID, f.productID)
	if finalTotal != 1 || finalByType["PICK"] != 1 {
		t.Fatalf("final inventory_transactions = %v (total %d), want exactly {PICK: 1} — Packing/Shipping/Loading/Dispatch must write zero additional ledger rows", finalByType, finalTotal)
	}
	// And no balance row exists anywhere for this product at any
	// location belonging to the DESTINATION warehouse — the real
	// status-window proof: crediting the destination is 33E.2's own
	// job, not something this pipeline run creates as a side effect.
	var destRowCount int64
	if err := tx.Raw(
		`SELECT COUNT(*) FROM inventory i JOIN locations l ON l.id = i.location_id JOIN zones z ON z.id = l.zone_id
		 WHERE i.tenant_id = ? AND i.product_id = ? AND z.warehouse_id = ?`,
		f.tenantID, f.productID, destWarehouseID,
	).Scan(&destRowCount).Error; err != nil {
		t.Fatalf("destRowCount query: %v", err)
	}
	if destRowCount != 0 {
		t.Fatalf("destination warehouse has %d inventory row(s) for this product, want 0 (no credit anywhere — 33E.2's own job)", destRowCount)
	}
}

// inventorySvcBalance is a tiny raw-SQL helper (no inventory.Repository
// dependency needed) returning the live balance row this test asserts
// against.
func inventorySvcBalance(tx *gorm.DB, tenantID, productID, locationID int64, lotNo string) (*inventory.Inventory, error) {
	var inv inventory.Inventory
	err := tx.Where("tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?", tenantID, productID, locationID, lotNo).First(&inv).Error
	return &inv, err
}

// --- Validation coverage ---

// TestService_Create_InternalTransfer_RequiresDestinationWarehouse
// covers Service.validateTransferFields' own "required" branch.
func TestService_Create_InternalTransfer_RequiresDestinationWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-missing-dest")
	seedOrderBalance(t, tx, f, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	_, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(INTERNAL_TRANSFER, no destination_warehouse_id) = %v, want ErrValidation", err)
	}
}

// TestService_Create_InternalTransfer_RejectsSameSourceAndDestination
// covers the "destination_warehouse_id must differ from warehouse_id"
// rule.
func TestService_Create_InternalTransfer_RejectsSameSourceAndDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-same-wh")
	seedOrderBalance(t, tx, f, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	sameID := f.warehouseID
	_, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID, DestinationWarehouseID: &sameID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(destination == source) = %v, want ErrValidation", err)
	}
}

// TestService_Create_InternalTransfer_RejectsCrossTenantDestination is
// the real security-boundary test: a destination_warehouse_id
// belonging to a DIFFERENT tenant must be rejected exactly like a
// nonexistent one, never distinguishable from it.
func TestService_Create_InternalTransfer_RejectsCrossTenantDestination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "xfer-cross-a")
	b := newOrderFixture(t, tx, "xfer-cross-b")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	foreignWarehouseID := b.warehouseID
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: a.warehouseID, DestinationWarehouseID: &foreignWarehouseID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrDestinationWarehouseNotFound) {
		t.Fatalf("Create(destination belongs to another tenant) = %v, want ErrDestinationWarehouseNotFound", err)
	}
}

// TestService_Create_SalesOrder_RejectsTransferFields covers the other
// half of the field-separation rule: a SALES_ORDER request that tries
// to set destination_warehouse_id/transfer_reference is rejected
// outright, never silently ignored.
func TestService_Create_SalesOrder_RejectsTransferFields(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-sales-reject")
	destWarehouseID := newDestinationWarehouse(t, tx, f.tenantID, "xfer-sales-reject")
	seedOrderBalance(t, tx, f, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	_, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "SALES_ORDER", WarehouseID: f.warehouseID, DestinationWarehouseID: &destWarehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(SALES_ORDER with destination_warehouse_id set) = %v, want ErrValidation", err)
	}

	ref := "TRF-SHOULD-NOT-BE-ALLOWED"
	_, err = svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "SALES_ORDER", WarehouseID: f.warehouseID, TransferReference: &ref,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(SALES_ORDER with transfer_reference set) = %v, want ErrValidation", err)
	}
}

// TestService_Create_InternalTransfer_Succeeds_DestinationCodeOnGet
// is a small, focused check (no full pipeline) that a valid transfer
// order Create succeeds and Get resolves the destination warehouse's
// own code for display.
func TestService_Create_InternalTransfer_Succeeds_DestinationCodeOnGet(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-get-code")
	destWarehouseID := newDestinationWarehouse(t, tx, f.tenantID, "xfer-get-code")
	seedOrderBalance(t, tx, f, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	created, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID, DestinationWarehouseID: &destWarehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(context.Background(), f.tenantID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DestinationWarehouseCode == nil || *got.DestinationWarehouseCode == "" {
		t.Fatalf("Get: DestinationWarehouseCode = %v, want a resolved warehouse code", got.DestinationWarehouseCode)
	}
}

// TestService_Update_InternalTransfer_SameValidationAsCreate proves
// Update runs the identical validateTransferFields call Create does —
// a PENDING SALES_ORDER updated into INTERNAL_TRANSFER still requires
// destination_warehouse_id, and updating it back to SALES_ORDER while
// still carrying destination_warehouse_id is still rejected.
func TestService_Update_InternalTransfer_SameValidationAsCreate(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newOrderFixture(t, tx, "xfer-update")
	destWarehouseID := newDestinationWarehouse(t, tx, f.tenantID, "xfer-update")
	seedOrderBalance(t, tx, f, "LOT-A", 100, 0)
	svc, _ := newOrderServiceAndRepo(tx)

	created, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		OrderType: "SALES_ORDER", WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// PENDING SALES_ORDER -> INTERNAL_TRANSFER with no destination: rejected.
	var validationErr ErrValidation
	_, err = svc.Update(context.Background(), f.tenantID, created.ID, UpdateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	})
	if !errors.As(err, &validationErr) {
		t.Fatalf("Update(SALES_ORDER -> INTERNAL_TRANSFER, no destination) = %v, want ErrValidation", err)
	}

	// Now with a valid destination: succeeds.
	updated, err := svc.Update(context.Background(), f.tenantID, created.ID, UpdateRequest{
		OrderType: "INTERNAL_TRANSFER", WarehouseID: f.warehouseID, DestinationWarehouseID: &destWarehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	})
	if err != nil {
		t.Fatalf("Update(SALES_ORDER -> INTERNAL_TRANSFER, with destination): %v", err)
	}
	if updated.DestinationWarehouseID == nil || *updated.DestinationWarehouseID != destWarehouseID {
		t.Fatalf("Update: DestinationWarehouseID = %v, want %d", updated.DestinationWarehouseID, destWarehouseID)
	}

	// Back to SALES_ORDER while still passing destination_warehouse_id: rejected.
	_, err = svc.Update(context.Background(), f.tenantID, created.ID, UpdateRequest{
		OrderType: "SALES_ORDER", WarehouseID: f.warehouseID, DestinationWarehouseID: &destWarehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	})
	if !errors.As(err, &validationErr) {
		t.Fatalf("Update(INTERNAL_TRANSFER -> SALES_ORDER, still carrying destination) = %v, want ErrValidation", err)
	}

	// Back to SALES_ORDER with destination cleared: succeeds, and the
	// column is cleared back to NULL (full-replace semantics).
	cleared, err := svc.Update(context.Background(), f.tenantID, created.ID, UpdateRequest{
		OrderType: "SALES_ORDER", WarehouseID: f.warehouseID,
		Items: []CreateItemRequest{{ProductID: f.productID, OrderedQty: 10, UOM: "PCS"}},
	})
	if err != nil {
		t.Fatalf("Update(INTERNAL_TRANSFER -> SALES_ORDER, destination cleared): %v", err)
	}
	if cleared.DestinationWarehouseID != nil {
		t.Fatalf("Update: DestinationWarehouseID = %v, want nil after reverting to SALES_ORDER", cleared.DestinationWarehouseID)
	}
}

// TestNormalizeOrderTypeAndPriority_InternalTransfer_StillValid is a
// tiny regression guard: INTERNAL_TRANSFER must remain an accepted
// order_type after this phase's own changes (unchanged from Phase 33A).
func TestNormalizeOrderTypeAndPriority_InternalTransfer_StillValid(t *testing.T) {
	orderType, _, err := normalizeOrderTypeAndPriority("INTERNAL_TRANSFER", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderType != "INTERNAL_TRANSFER" {
		t.Errorf("orderType = %q, want INTERNAL_TRANSFER", orderType)
	}
}
