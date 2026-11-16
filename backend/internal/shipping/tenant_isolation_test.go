// Phase 30 Batch 4 — Shipping tenant isolation. Mirrors
// internal/picking/tenant_isolation_test.go's own shape (Batch 2: fresh
// tenant per test, real seeded rows, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on shipment lookup/list/write, cross-tenant packing-job ownership
// verification at shipment creation, the Confirm status-machine's
// idempotency-guarded rollback, and concurrent Confirm calls sharing one
// idempotency key.
//
// Phase 33D.1 re-keyed Shipping.Create off a COMPLETED packing job
// rather than a picking task directly — every fixture below now runs a
// picking task all the way through a real Packing Create -> CreateCarton
// -> PackItem -> CheckerConfirm cycle (createCompletedPackingJob) before
// handing the resulting packing_job_id to Shipping, exactly what a real
// caller must do now.
package shipping

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
)

// shippingFixture is one fully-seeded tenant: a product, a warehouse,
// and a location within it — the same real-row shape
// internal/picking/internal/order's own Batch 2/3 fixtures use, so
// ownership verification exercises the actual tenant-scoped
// FindByIDForTenant/resolveLocation/picking.Service.Get queries this
// batch's own Create relies on transitively.
type shippingFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newShippingFixture(t *testing.T, tx *gorm.DB, label string) shippingFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"ship-iso-"+label, "Shipping Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newShippingFixture(%s): seed location: %v", label, err)
	}

	return shippingFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedShippingBalance(t *testing.T, tx *gorm.DB, f shippingFixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedShippingBalance: %v", err)
	}
}

// newPickingServiceForShipping wires the real picking stack this
// package's own fixtures need to get a picking task to COMPLETED in the
// first place, and packing.Service's own pickingChecker dependency —
// same construction internal/picking's own tests use.
func newPickingServiceForShipping(tx *gorm.DB) *picking.Service {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, location.NewRepository(tx))
	return picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
}

// newPackingServiceForShipping wires the real packing stack Shipping
// depends on structurally (packingChecker, Phase 33D.1) — bound to the
// same tx throughout.
func newPackingServiceForShipping(tx *gorm.DB, pickingSvc *picking.Service) *packing.Service {
	return packing.NewService(packing.NewRepository(tx), pickingSvc)
}

// newShippingServiceAndRepo wires the full real stack Shipping
// orchestrates through — packing.Service (Get, Phase 33D.1's own
// pack-validation gate) — bound to the same test transaction throughout,
// exactly the dependency graph cmd/server/main.go wires in production.
func newShippingServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	return NewService(repo, packingSvc), repo
}

// createCompletedPickingTask runs a real picking task through
// Create->Confirm(PICKED) to reach COMPLETED status with at least one
// COMPLETED line (picked_qty > 0) — the precondition
// createCompletedPackingJob needs before it can even open a packing job.
// Returns the picking task's own ID.
func createCompletedPickingTask(t *testing.T, pickingSvc *picking.Service, tenantID int64, f shippingFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := pickingSvc.Create(context.Background(), tenantID, picking.CreateRequest{
		OrderNo: "SO-SHIP-TEST", WarehouseID: f.warehouseID,
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

// createCompletedPackingJob (Phase 33D.1) is the real precondition
// Shipping.Create now requires: a picking task run all the way through
// Create -> Confirm(PICKED) (createCompletedPickingTask), then a real
// packing job run all the way through Create -> CreateCarton -> PackItem
// (packing the line's own full picked_qty into one carton) ->
// CheckerConfirm, reaching COMPLETED. Returns the packing job's own ID
// — the only id Shipping.Create accepts as of this phase.
func createCompletedPackingJob(t *testing.T, pickingSvc *picking.Service, packingSvc *packing.Service, tenantID int64, f shippingFixture, lotNo string, qty float64) int64 {
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

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "find-a")
	b := newShippingFixture(t, tx, "find-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, repo := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)

	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.FindByID(context.Background(), a.tenantID, resp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), b.tenantID, resp.ID); err != nil {
		t.Errorf("FindByID(tenantB, its own shipment) = %v, want success", err)
	}
}

func TestRepository_FindDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "detail-a")
	b := newShippingFixture(t, tx, "detail-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, repo := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.FindDetail(context.Background(), a.tenantID, resp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindDetail(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "get-a")
	b := newShippingFixture(t, tx, "get-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Get(context.Background(), a.tenantID, resp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "list-a")
	b := newShippingFixture(t, tx, "list-b")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, repo := newShippingServiceAndRepo(tx)
	jobA := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 5)
	jobB := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 5)
	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobA}, 1); err != nil {
		t.Fatalf("Create(tenantA): %v", err)
	}
	if _, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobB}, 1); err != nil {
		t.Fatalf("Create(tenantB): %v", err)
	}

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's shipment)", len(rows), total)
	}
}

// --- Create: foreign packing-job ownership rejection ---

func TestService_Create_RejectsForeignTenantPackingJob(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "cpt-a")
	b := newShippingFixture(t, tx, "cpt-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)

	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if !errors.Is(err, ErrPackingJobNotFound) {
		t.Fatalf("Create(tenantA, tenantB's packing_job_id) = %v, want ErrPackingJobNotFound", err)
	}
}

func TestService_Create_RejectsNotYetCompletedJob(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "not-packed")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	taskID := createCompletedPickingTask(t, pickingSvc, a.tenantID, a, "LOT-A", 10)

	// Deliberately stop short of CheckerConfirm — the job exists
	// (DRAFT), real packing activity may even have happened, but it has
	// not been checker-confirmed into COMPLETED yet.
	job, err := packingSvc.Create(context.Background(), a.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("packing Create: %v", err)
	}

	_, err = svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: job.ID}, 1)
	if !errors.Is(err, ErrNotPacked) {
		t.Fatalf("Create(not-yet-COMPLETED packing job) = %v, want ErrNotPacked", err)
	}
}

func TestService_Create_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "create-ok")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)

	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create(own tenant): %v", err)
	}
	if resp.Status != StatusReady || len(resp.Items) != 1 {
		t.Fatalf("Create response = %+v, want 1 READY item", resp)
	}
	if resp.Items[0].Qty != 10 {
		t.Errorf("Create response item qty = %v, want 10 (the packed_qty, not picked_qty)", resp.Items[0].Qty)
	}

	// A second shipment against the same packing job must be rejected —
	// one shipment per packing job, regardless of tenant.
	_, err = svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if !errors.Is(err, ErrAlreadyShipped) {
		t.Errorf("second Create(same packing job) = %v, want ErrAlreadyShipped", err)
	}
}

// --- Update: IDOR on the write path ---

func TestService_Update_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "upd-a")
	b := newShippingFixture(t, tx, "upd-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	carrier := "DHL"
	_, err = svc.Update(context.Background(), a.tenantID, resp.ID, UpdateRequest{Carrier: &carrier})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}

	reloaded, err := svc.Get(context.Background(), b.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Update: %v", err)
	}
	if reloaded.Carrier != nil {
		t.Errorf("Carrier after rejected cross-tenant Update = %v, want still nil", reloaded.Carrier)
	}
}

// --- Confirm: IDOR + status-machine rollback ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "confirm-cross-a")
	b := newShippingFixture(t, tx, "confirm-cross-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-cross-1", Action: "DISPATCH",
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}

	// Confirming as the real owner must still work after the rejected
	// cross-tenant attempt — proves it left no residue (no idempotency
	// row inserted under the wrong tenant pairing that could block the
	// legitimate caller).
	reloaded, err := svc.Get(context.Background(), b.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if reloaded.Status != StatusReady {
		t.Errorf("shipment status after rejected cross-tenant Confirm = %v, want still READY", reloaded.Status)
	}
}

func TestService_Confirm_DispatchThenDeliver_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "confirm-ok")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	carrier, tracking := "DHL", "DHL123"
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Phase 33D.2's own dispatch gate — a shipment must have gone
	// through internal/loading's own Confirm Trip action (which alone
	// calls this) before DISPATCH is allowed. This test is about the
	// DISPATCH -> DELIVER status machine itself, not the gate, so it is
	// satisfied directly here rather than running a full Loading cycle
	// — see internal/loading's own tests for coverage of the gate
	// itself.
	if err := svc.MarkLoaded(context.Background(), a.tenantID, resp.ID, 1); err != nil {
		t.Fatalf("MarkLoaded (simulating a completed Loading cycle): %v", err)
	}

	dispatched, err := svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-dispatch-1", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DISPATCH, own tenant): %v", err)
	}
	if dispatched.Status != StatusDispatched || dispatched.DispatchedAt == nil {
		t.Fatalf("after DISPATCH = %+v, want DISPATCHED with DispatchedAt set", dispatched)
	}

	delivered, err := svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-deliver-1", Action: "DELIVER",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(DELIVER, own tenant): %v", err)
	}
	if delivered.Status != StatusDelivered || delivered.DeliveredAt == nil {
		t.Fatalf("after DELIVER = %+v, want DELIVERED with DeliveredAt set", delivered)
	}
}

// TestService_Confirm_InvalidTransition_RollsBackEverything proves
// Confirm's whole transaction — including the idempotency-guard INSERT
// — rolls back together when the requested action is rejected (here:
// DISPATCH without a carrier/tracking number already set). A rolled-
// back idempotency row means the SAME key can be retried later with a
// corrected request, rather than being permanently burned by a rejected
// attempt.
func TestService_Confirm_InvalidTransition_RollsBackEverything(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "confirm-rollback")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	// Deliberately no carrier/tracking_no set.
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Phase 33D.2's own dispatch gate is satisfied directly (see
	// TestService_Confirm_DispatchThenDeliver_OwnTenant_Succeeds' own
	// doc) — this test is specifically about the carrier/tracking
	// validation rejection, not the loading gate.
	if err := svc.MarkLoaded(context.Background(), a.tenantID, resp.ID, 1); err != nil {
		t.Fatalf("MarkLoaded (simulating a completed Loading cycle): %v", err)
	}

	_, err = svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1", Action: "DISPATCH",
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(DISPATCH, no carrier/tracking) = %v, want ErrValidation", err)
	}

	reloaded, err := svc.Get(context.Background(), a.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rolled-back Confirm: %v", err)
	}
	if reloaded.Status != StatusReady {
		t.Errorf("shipment status after rolled-back Confirm = %v, want still READY", reloaded.Status)
	}

	// The same idempotency key must still be usable — proves the guard
	// row itself rolled back, not just the status.
	carrier, tracking := "DHL", "DHL123"
	if _, err := svc.Update(context.Background(), a.tenantID, resp.ID, UpdateRequest{Carrier: &carrier, TrackingNo: &tracking}); err != nil {
		t.Fatalf("Update to add carrier/tracking before retry: %v", err)
	}
	retried, err := svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("retry Confirm with same idempotency key after rollback: %v", err)
	}
	if retried.Status != StatusDispatched {
		t.Errorf("retried Confirm status = %v, want DISPATCHED", retried.Status)
	}
}

// --- Cancel: IDOR ---

func TestService_Cancel_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "cancel-cross-a")
	b := newShippingFixture(t, tx, "cancel-cross-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	resp, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Cancel(context.Background(), a.tenantID, resp.ID, CancelRequest{}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}

	reloaded, err := svc.Get(context.Background(), b.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Cancel: %v", err)
	}
	if reloaded.Status != StatusReady {
		t.Errorf("shipment status after rejected cross-tenant Cancel = %v, want still READY", reloaded.Status)
	}
}

func TestService_Cancel_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "cancel-ok")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cancelled, err := svc.Cancel(context.Background(), a.tenantID, resp.ID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(own tenant): %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Errorf("shipment status = %v, want CANCELLED", cancelled.Status)
	}
}

// --- Concurrency: the idempotency-key guard under real concurrent Confirm calls ---

// TestService_Confirm_ConcurrentSameIdempotencyKey_OnlyOneApplies proves
// shipment_confirmations' own PRIMARY KEY (migrations/0022) still
// prevents a double-apply when two goroutines race Confirm with the
// exact SAME idempotency key — exactly one actually transitions the
// shipment; the other safely observes the already-applied state via
// ErrDuplicateIdempotencyKey's own idempotent-return path. Deliberately
// does NOT use testsupport.BeginRollbackTx — a single *sql.Tx is not
// safe for concurrent use by multiple goroutines — so this uses the
// real pooled *gorm.DB with explicit, FK-ordered cleanup instead of
// rollback, the same pattern internal/picking's own
// TestRepository_AdjustPickedQty_Concurrent_NoLostUpdates (Batch 2) and
// internal/order's own TestService_CreatePickingTask_ConcurrentClaims_OnlyOneWins
// (Batch 3) established.
func TestService_Confirm_ConcurrentSameIdempotencyKey_OnlyOneApplies(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('ship-concurrent', 'Shipping Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		// inventory_transactions must be deleted before inventory/
		// locations/products (FK_inventory_transactions_location/
		// _product) — this Confirm(PICKED) call writes a ledger row via
		// ConsumeReservationTx, so omitting this line (as this cleanup
		// originally did) leaves every one of the DELETEs below it
		// failing on an FK conflict and the whole tenant orphaned,
		// colliding with this test's own hard-coded tenant slug on the
		// next run — the same class of bug internal/order's own
		// equivalent concurrency test cleanup was fixed for. Phase 33D.1
		// adds packing's own tables to this same FK-ordered cleanup,
		// since every shipment now has a real packing job behind it.
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
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-SCONC', 'Category SConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-SCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-SCONC', 'Warehouse SConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-SCONC', 'Zone SConc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-SCONC", Name: "Location SConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	f := shippingFixture{tenantID: tenantID, productID: productID, warehouseID: warehouseID, locationID: locationID}
	inv := inventory.Inventory{TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-SCONC", Quantity: 100, ReservedQuantity: 0}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("seed balance: %v", err)
	}

	pickingSvc := newPickingServiceForShipping(db)
	packingSvc := newPackingServiceForShipping(db, pickingSvc)
	svc, _ := newShippingServiceAndRepo(db)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, tenantID, f, "LOT-SCONC", 10)
	carrier, tracking := "DHL", "DHL-SCONC"
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Phase 33D.2's own dispatch gate, satisfied directly — this test
	// is about the idempotency-guard race, not the loading gate.
	if err := svc.MarkLoaded(context.Background(), tenantID, resp.ID, 1); err != nil {
		t.Fatalf("MarkLoaded (simulating a completed Loading cycle): %v", err)
	}

	const attempts = 10
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.Confirm(context.Background(), tenantID, resp.ID, ConfirmRequest{
				IdempotencyKey: "key-sconc-shared", Action: "DISPATCH",
			}, 1)
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err != nil {
			t.Errorf("concurrent Confirm with shared idempotency key returned unexpected error: %v", err)
			continue
		}
		successes++
	}
	if successes != attempts {
		t.Errorf("successes = %d, want all %d calls to return successfully (idempotent no-op after the first)", successes, attempts)
	}

	final, err := svc.Get(context.Background(), tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after concurrent Confirm calls: %v", err)
	}
	if final.Status != StatusDispatched {
		t.Errorf("final shipment status = %v, want DISPATCHED exactly once, never corrupted by the race", final.Status)
	}
}
