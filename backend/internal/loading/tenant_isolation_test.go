// Phase 33D.2 — Loading tenant isolation and workflow coverage. Mirrors
// internal/shipping's own tenant_isolation_test.go shape (fresh tenant
// per test, real seeded rows via the real picking/packing/shipping
// stack, testsupport.ConnectDB + BeginRollbackTx) — every fixture below
// runs a picking task all the way through Create -> Confirm(PICKED),
// then a packing job through Create -> CreateCarton -> PackItem ->
// CheckerConfirm, then a shipping.Service.Create, to reach a real READY
// shipment with real cartons behind it before handing it to Loading,
// exactly what a real caller must do.
package loading

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

type loadingFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newLoadingFixture(t *testing.T, tx *gorm.DB, label string) loadingFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"load-iso-"+label, "Loading Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newLoadingFixture(%s): seed location: %v", label, err)
	}

	return loadingFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedLoadingBalance(t *testing.T, tx *gorm.DB, f loadingFixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedLoadingBalance: %v", err)
	}
}

func newPickingServiceForLoading(tx *gorm.DB) *picking.Service {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, location.NewRepository(tx))
	return picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
}

func newPackingServiceForLoading(tx *gorm.DB, pickingSvc *picking.Service) *packing.Service {
	return packing.NewService(packing.NewRepository(tx), pickingSvc)
}

func newShippingServiceForLoading(tx *gorm.DB, packingSvc *packing.Service) *shipping.Service {
	return shipping.NewService(shipping.NewRepository(tx), packingSvc)
}

// newLoadingStack wires the full real stack Loading orchestrates
// through (shipping.Service, packing.Service) — all bound to the same
// test transaction, exactly the dependency graph cmd/server/main.go
// wires in production.
func newLoadingStack(tx *gorm.DB) (loadingSvc *Service, shippingSvc *shipping.Service, packingSvc *packing.Service, pickingSvc *picking.Service, repo *Repository) {
	pickingSvc = newPickingServiceForLoading(tx)
	packingSvc = newPackingServiceForLoading(tx, pickingSvc)
	shippingSvc = newShippingServiceForLoading(tx, packingSvc)
	repo = NewRepository(tx)
	loadingSvc = NewService(repo, shippingSvc, packingSvc)
	return
}

func createCompletedPickingTask(t *testing.T, pickingSvc *picking.Service, tenantID int64, f loadingFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := pickingSvc.Create(context.Background(), tenantID, picking.CreateRequest{
		OrderNo: "SO-LOAD-TEST", WarehouseID: f.warehouseID,
		Items: []picking.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, OrderedQty: qty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createCompletedPickingTask: picking Create: %v", err)
	}
	confirmed, err := pickingSvc.Confirm(context.Background(), tenantID, resp.ID, picking.ConfirmRequest{
		IdempotencyKey: "pick-confirm-" + resp.PickNo,
		Items:          []picking.ConfirmItemRequest{{ItemID: resp.Items[0].ID, Action: "PICKED", Quantity: qty}},
	}, 1)
	if err != nil {
		t.Fatalf("createCompletedPickingTask: picking Confirm: %v", err)
	}
	if confirmed.Status != picking.StatusCompleted {
		t.Fatalf("createCompletedPickingTask: picking task status = %v, want COMPLETED", confirmed.Status)
	}
	return resp.ID
}

// createCompletedPackingJob runs a picking task through to COMPLETED,
// then a packing job through Create -> CreateCarton -> PackItem ->
// CheckerConfirm (one single carton holding the full picked_qty).
// Returns the packing job's own ID.
func createCompletedPackingJob(t *testing.T, pickingSvc *picking.Service, packingSvc *packing.Service, tenantID int64, f loadingFixture, lotNo string, qty float64) int64 {
	t.Helper()
	taskID := createCompletedPickingTask(t, pickingSvc, tenantID, f, lotNo, qty)

	job, err := packingSvc.Create(context.Background(), tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("createCompletedPackingJob: packing Create: %v", err)
	}
	carton, err := packingSvc.CreateCarton(context.Background(), tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("createCompletedPackingJob: packing CreateCarton: %v", err)
	}
	packed, err := packingSvc.PackItem(context.Background(), tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: "pack-confirm-" + job.JobNo, ItemID: job.Items[0].ID, CartonID: carton.ID,
		ScannedSKU: job.Items[0].SKU, Quantity: job.Items[0].PickedQty,
	}, 1)
	if err != nil {
		t.Fatalf("createCompletedPackingJob: packing PackItem: %v", err)
	}
	confirmed, err := packingSvc.CheckerConfirm(context.Background(), tenantID, packed.ID, packing.CheckerConfirmRequest{}, 1)
	if err != nil {
		t.Fatalf("createCompletedPackingJob: packing CheckerConfirm: %v", err)
	}
	if confirmed.Status != packing.StatusCompleted {
		t.Fatalf("createCompletedPackingJob: packing job status = %v, want COMPLETED", confirmed.Status)
	}
	return job.ID
}

// createReadyShipment runs createCompletedPackingJob, then
// shipping.Service.Create, returning the new, READY shipment's own ID.
func createReadyShipment(t *testing.T, pickingSvc *picking.Service, packingSvc *packing.Service, shippingSvc *shipping.Service, tenantID int64, f loadingFixture, lotNo string, qty float64) int64 {
	t.Helper()
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, tenantID, f, lotNo, qty)
	carrier, tracking := "DHL", "DHL-"+lotNo
	resp, err := shippingSvc.Create(context.Background(), tenantID, shipping.CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("createReadyShipment: shipping Create: %v", err)
	}
	if resp.Status != shipping.StatusReady {
		t.Fatalf("createReadyShipment: shipment status = %v, want READY", resp.Status)
	}
	return resp.ID
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindTripByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "find-a")
	b := newLoadingFixture(t, tx, "find-b")
	loadingSvc, _, _, _, repo := newLoadingStack(tx)

	trip, err := loadingSvc.CreateTrip(context.Background(), b.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}

	if _, err := repo.FindTripByID(context.Background(), a.tenantID, trip.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindTripByID(tenantA, tenantB's trip) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindTripByID(context.Background(), b.tenantID, trip.ID); err != nil {
		t.Errorf("FindTripByID(tenantB, its own trip) = %v, want success", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "get-a")
	b := newLoadingFixture(t, tx, "get-b")
	loadingSvc, _, _, _, _ := newLoadingStack(tx)

	trip, err := loadingSvc.CreateTrip(context.Background(), b.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}

	if _, err := loadingSvc.Get(context.Background(), a.tenantID, trip.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's trip) = %v, want ErrNotFound", err)
	}
}

// --- AssignShipment ---

func TestService_AssignShipment_RejectsForeignTenantShipment(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "assign-cross-a")
	b := newLoadingFixture(t, tx, "assign-cross-b")
	seedLoadingBalance(t, tx, b, "LOT-B", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, b.tenantID, b, "LOT-B", 10)

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}

	_, err = loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if !errors.Is(err, ErrShipmentNotFound) {
		t.Fatalf("AssignShipment(tenantA trip, tenantB's shipment) = %v, want ErrShipmentNotFound", err)
	}
}

func TestService_AssignShipment_RejectsNotReadyShipment(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "assign-notready")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	carrier, tracking := "DHL", "DHL-NOTREADY"
	shipment, err := shippingSvc.Create(context.Background(), a.tenantID, shipping.CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}
	// Cancel it — no longer READY.
	if _, err := shippingSvc.Cancel(context.Background(), a.tenantID, shipment.ID, shipping.CancelRequest{}, 1); err != nil {
		t.Fatalf("shipping Cancel: %v", err)
	}

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	_, err = loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipment.ID}, 1)
	if !errors.Is(err, ErrShipmentNotReady) {
		t.Fatalf("AssignShipment(cancelled shipment) = %v, want ErrShipmentNotReady", err)
	}
}

func TestService_AssignShipment_OwnTenant_PopulatesCartons(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "assign-ok")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}

	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	if len(resp.Shipments) != 1 {
		t.Fatalf("AssignShipment response shipments = %d, want 1", len(resp.Shipments))
	}
	if resp.PlannedCount != 1 || len(resp.Cartons) != 1 {
		t.Fatalf("AssignShipment response = %+v, want exactly 1 planned carton (one carton created by createReadyShipment)", resp)
	}
	if resp.Cartons[0].Status != CartonStatusPlanned {
		t.Errorf("new trip carton status = %v, want PLANNED", resp.Cartons[0].Status)
	}

	// Assigning the SAME shipment to a SECOND trip must be rejected —
	// still actively assigned to the first.
	trip2, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip(2): %v", err)
	}
	_, err = loadingSvc.AssignShipment(context.Background(), a.tenantID, trip2.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if !errors.Is(err, ErrShipmentAlreadyAssigned) {
		t.Errorf("AssignShipment(same shipment, second trip) = %v, want ErrShipmentAlreadyAssigned", err)
	}
}

func TestService_AssignShipment_FreedAfterCancel(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "assign-freed")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)

	trip1, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip(1): %v", err)
	}
	if _, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip1.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1); err != nil {
		t.Fatalf("AssignShipment(1): %v", err)
	}
	if _, err := loadingSvc.CancelTrip(context.Background(), a.tenantID, trip1.ID, CancelRequest{}, 1); err != nil {
		t.Fatalf("CancelTrip(1): %v", err)
	}

	// Mirrors picking_batch_orders' own "cancelled batches don't lock
	// the order forever" precedent: the shipment is free to join a
	// brand new trip now that trip1 is CANCELLED.
	trip2, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip(2): %v", err)
	}
	if _, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip2.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1); err != nil {
		t.Errorf("AssignShipment(same shipment, after trip1 cancelled) = %v, want success", err)
	}
}

// --- ScanCarton / ShortCarton ---

func TestService_ScanCarton_WrongTrip_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "scan-wrongtrip")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)
	seedLoadingBalance(t, tx, a, "LOT-A2", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipment1 := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)
	shipment2 := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A2", 10)

	trip1, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	trip2, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	resp1, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip1.ID, AssignShipmentRequest{ShipmentID: shipment1}, 1)
	if err != nil {
		t.Fatalf("AssignShipment(trip1): %v", err)
	}
	if _, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip2.ID, AssignShipmentRequest{ShipmentID: shipment2}, 1); err != nil {
		t.Fatalf("AssignShipment(trip2): %v", err)
	}

	// shipment1's own carton's LPN, scanned against trip2 (which only
	// has shipment2's own carton planned) — must be rejected.
	lpn := resp1.Cartons[0].LPN
	_, err = loadingSvc.ScanCarton(context.Background(), a.tenantID, trip2.ID, ScanCartonRequest{LPN: lpn}, 1)
	if !errors.Is(err, ErrCartonNotPlannedForTrip) {
		t.Fatalf("ScanCarton(trip1's carton against trip2) = %v, want ErrCartonNotPlannedForTrip", err)
	}

	// The correct trip still accepts it.
	resp, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip1.ID, ScanCartonRequest{LPN: lpn}, 1)
	if err != nil {
		t.Fatalf("ScanCarton(trip1's own carton against trip1) = %v, want success", err)
	}
	if resp.Status != StatusLoading {
		t.Errorf("trip status after first scan = %v, want LOADING", resp.Status)
	}
}

func TestService_ScanCarton_UnplannedLPN_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "scan-unplanned")
	loadingSvc, _, _, _, _ := newLoadingStack(tx)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)

	_, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: "CTN-DOES-NOT-EXIST"}, 1)
	if !errors.Is(err, ErrCartonNotFound) {
		t.Fatalf("ScanCarton(nonexistent LPN) = %v, want ErrCartonNotFound", err)
	}
}

func TestService_ScanCarton_DuplicateScan_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "scan-dup")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	lpn := resp.Cartons[0].LPN

	if _, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: lpn}, 1); err != nil {
		t.Fatalf("first ScanCarton: %v", err)
	}
	_, err = loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: lpn}, 1)
	if !errors.Is(err, ErrCartonAlreadyLoaded) {
		t.Fatalf("duplicate ScanCarton = %v, want ErrCartonAlreadyLoaded", err)
	}
}

func TestService_ShortCarton_ThenRescanRejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "short-ok")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	lpn := resp.Cartons[0].LPN

	remark := "Carton crushed while staging"
	shortResp, err := loadingSvc.ShortCarton(context.Background(), a.tenantID, trip.ID, ShortCartonRequest{LPN: lpn, Reason: "DAMAGED", Remark: &remark}, 1)
	if err != nil {
		t.Fatalf("ShortCarton: %v", err)
	}
	if shortResp.ShortCount != 1 || shortResp.Cartons[0].Status != CartonStatusShort {
		t.Fatalf("after ShortCarton = %+v, want 1 SHORT carton", shortResp)
	}

	_, err = loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: lpn}, 1)
	if !errors.Is(err, ErrCartonAlreadyResolved) {
		t.Fatalf("ScanCarton after ShortCarton = %v, want ErrCartonAlreadyResolved", err)
	}
}

func TestService_ShortCarton_InvalidReason_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "short-badreason")
	loadingSvc, _, _, _, _ := newLoadingStack(tx)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)

	_, err := loadingSvc.ShortCarton(context.Background(), a.tenantID, trip.ID, ShortCartonRequest{LPN: "CTN-X", Reason: "NOT_A_REAL_REASON"}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("ShortCarton(invalid reason) = %v, want ErrValidation", err)
	}
}

// --- ConfirmTrip readiness + the real dispatch-gate payoff ---

func TestService_ConfirmTrip_RejectsWhileCartonsStillPlanned(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "confirm-notready")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if _, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1); err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}

	// Still PLANNED — a still-PLANNED trip (CanConfirm requires LOADING)
	// must be rejected as ErrInvalidTransition before readiness is even
	// checked.
	_, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ConfirmTrip(still PLANNED, zero scans) = %v, want ErrInvalidTransition", err)
	}
}

func TestService_ConfirmTrip_EmptyTrip_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "confirm-empty")
	loadingSvc, _, _, _, _ := newLoadingStack(tx)
	trip, _ := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)

	_, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ConfirmTrip(empty trip, never left PLANNED) = %v, want ErrInvalidTransition", err)
	}
}

// TestFullHappyPath_TwoShipmentsTwoPackingJobs_BothDispatchableAfterConfirm
// is the end-to-end scenario this phase's own instructions ask for:
// create a trip, assign 2 shipments sourced from 2 different packing
// jobs, scan every carton loaded, confirm the trip, and prove BOTH
// shipments are now dispatchable (the real payoff: shipping.Service.
// MarkLoaded was called for each one).
func TestFullHappyPath_TwoShipmentsTwoPackingJobs_BothDispatchableAfterConfirm(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "happy")
	seedLoadingBalance(t, tx, a, "LOT-1", 100, 0)
	seedLoadingBalance(t, tx, a, "LOT-2", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipment1 := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-1", 10)
	shipment2 := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-2", 15)

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{VehicleNo: strPtr("1กก-1234")}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	if _, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipment1}, 1); err != nil {
		t.Fatalf("AssignShipment(1): %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipment2}, 1)
	if err != nil {
		t.Fatalf("AssignShipment(2): %v", err)
	}
	if len(resp.Shipments) != 2 || resp.PlannedCount != 2 {
		t.Fatalf("after assigning both shipments = %+v, want 2 shipments / 2 planned cartons", resp)
	}

	for _, c := range resp.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}

	confirmed, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1)
	if err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}
	if confirmed.Status != StatusCompleted || confirmed.LoadedCount != 2 {
		t.Fatalf("after ConfirmTrip = %+v, want COMPLETED with 2 loaded cartons", confirmed)
	}

	for _, shipmentID := range []int64{shipment1, shipment2} {
		dispatched, err := shippingSvc.Confirm(context.Background(), a.tenantID, shipmentID, shipping.ConfirmRequest{
			IdempotencyKey: "dispatch-after-load-" + strconv.FormatInt(shipmentID, 10), Action: "DISPATCH",
		}, 1)
		if err != nil {
			t.Errorf("Confirm(DISPATCH) for shipment %d after full Loading cycle = %v, want success", shipmentID, err)
			continue
		}
		if dispatched.Status != shipping.StatusDispatched {
			t.Errorf("shipment %d status after DISPATCH = %v, want DISPATCHED", shipmentID, dispatched.Status)
		}
	}
}

func strPtr(s string) *string { return &s }

// --- The dispatch-gate regression test this phase exists for ---

func TestDispatchGate_BlockedBeforeLoading_AllowedAfter_NeverAssignedStaysBlocked(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "gate")
	seedLoadingBalance(t, tx, a, "LOT-GATE-1", 100, 0)
	seedLoadingBalance(t, tx, a, "LOT-GATE-2", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)

	// Shipment A: never goes through Loading at all — a brand-new,
	// post-33D.1 shipment. Must be permanently blocked.
	neverLoaded := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-GATE-1", 10)
	_, err := shippingSvc.Confirm(context.Background(), a.tenantID, neverLoaded, shipping.ConfirmRequest{
		IdempotencyKey: "gate-key-1", Action: "DISPATCH",
	}, 1)
	if !errors.Is(err, shipping.ErrNotLoaded) {
		t.Fatalf("Confirm(DISPATCH) on a shipment never sent through Loading = %v, want shipping.ErrNotLoaded", err)
	}
	// Still blocked on a second attempt — no backdoor opens over time.
	_, err = shippingSvc.Confirm(context.Background(), a.tenantID, neverLoaded, shipping.ConfirmRequest{
		IdempotencyKey: "gate-key-1b", Action: "DISPATCH",
	}, 1)
	if !errors.Is(err, shipping.ErrNotLoaded) {
		t.Fatalf("second Confirm(DISPATCH) attempt = %v, want shipping.ErrNotLoaded (no backdoor)", err)
	}

	// Shipment B: goes through a FULL real Loading cycle — trip
	// created, shipment assigned, every carton scanned, trip confirmed
	// — and must become dispatchable as a direct result.
	willBeLoaded := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-GATE-2", 10)
	_, err = shippingSvc.Confirm(context.Background(), a.tenantID, willBeLoaded, shipping.ConfirmRequest{
		IdempotencyKey: "gate-key-2-early", Action: "DISPATCH",
	}, 1)
	if !errors.Is(err, shipping.ErrNotLoaded) {
		t.Fatalf("Confirm(DISPATCH) before Loading = %v, want shipping.ErrNotLoaded", err)
	}

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: willBeLoaded}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	for _, c := range resp.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}
	if _, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1); err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}

	dispatched, err := shippingSvc.Confirm(context.Background(), a.tenantID, willBeLoaded, shipping.ConfirmRequest{
		IdempotencyKey: "gate-key-2-after", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DISPATCH) after full Loading cycle = %v, want success", err)
	}
	if dispatched.Status != shipping.StatusDispatched {
		t.Errorf("after Loading + DISPATCH, status = %v, want DISPATCHED", dispatched.Status)
	}

	// The never-loaded shipment remains blocked even now — proves the
	// gate is per-shipment, not some global switch flipped by the
	// Loading cycle above.
	_, err = shippingSvc.Confirm(context.Background(), a.tenantID, neverLoaded, shipping.ConfirmRequest{
		IdempotencyKey: "gate-key-1c", Action: "DISPATCH",
	}, 1)
	if !errors.Is(err, shipping.ErrNotLoaded) {
		t.Fatalf("never-loaded shipment after sibling's Loading cycle = %v, want still shipping.ErrNotLoaded", err)
	}
}

// --- Concurrency: two scans racing for the same carton ---

func TestService_ScanCarton_ConcurrentScans_OnlyOneWins(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('load-concurrent', 'Loading Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM loading_trip_cartons WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM loading_trip_shipments WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM loading_trips WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM shipment_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM shipment_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM shipments WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM packing_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM carton_contents WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM cartons WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM packing_job_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM packing_jobs WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_confirmations WHERE tenant_id = ?", tenantID)
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
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-LCONC', 'Category LConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-LCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-LCONC', 'Warehouse LConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-LCONC', 'Zone LConc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-LCONC", Name: "Location LConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	f := loadingFixture{tenantID: tenantID, productID: productID, warehouseID: warehouseID, locationID: locationID}
	inv := inventory.Inventory{TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-LCONC", Quantity: 100, ReservedQuantity: 0}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("seed balance: %v", err)
	}

	pickingSvc := newPickingServiceForLoading(db)
	packingSvc := newPackingServiceForLoading(db, pickingSvc)
	shippingSvc := newShippingServiceForLoading(db, packingSvc)
	loadingSvc := NewService(NewRepository(db), shippingSvc, packingSvc)

	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, tenantID, f, "LOT-LCONC", 10)
	trip, err := loadingSvc.CreateTrip(context.Background(), tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}
	lpn := resp.Cartons[0].LPN

	const goroutines = 8
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := loadingSvc.ScanCarton(context.Background(), tenantID, trip.ID, ScanCartonRequest{LPN: lpn}, 1)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes, alreadyLoaded := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrCartonAlreadyLoaded):
			alreadyLoaded++
		default:
			t.Errorf("unexpected ScanCarton goroutine error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1 (only one concurrent scan may win)", successes)
	}
	if successes+alreadyLoaded != goroutines {
		t.Errorf("successes(%d) + alreadyLoaded(%d) = %d, want %d (every goroutine accounted for)", successes, alreadyLoaded, successes+alreadyLoaded, goroutines)
	}

	finalRepo := NewRepository(db)
	finalCarton, err := finalRepo.FindTripCarton(context.Background(), tenantID, trip.ID, resp.Cartons[0].CartonID)
	if err != nil {
		t.Fatalf("FindTripCarton after race: %v", err)
	}
	if finalCarton.Status != CartonStatusLoaded {
		t.Errorf("final carton status = %v, want LOADED exactly once, never corrupted by the race", finalCarton.Status)
	}
}
