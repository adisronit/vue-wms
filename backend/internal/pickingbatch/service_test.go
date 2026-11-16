// Phase 33C.1 — Batch Picking / Wave Management tests. Mirrors internal/
// order's and internal/relocation's own tenant_isolation_test.go shape
// (real seeded rows, testsupport.ConnectDB + BeginRollbackTx for
// sequential scenarios, a real unwrapped db.Raw-seeded fixture + manual
// cleanup for the one genuine concurrency scenario) rather than mocking
// out GORM or internal/picking.
//
// Phase 33D.1: built as an EXTERNAL test package ("pickingbatch_test",
// not "pickingbatch") together with sortation_test.go/pdf_test.go
// (which share this file's own fixture/stack helpers and so must move
// together) — internal/shipping now imports internal/packing (its own
// packingChecker), and internal/packing already imports internal/
// pickingbatch (its own batchOrchestrator, Phase 33C.2), so importing
// internal/order (which itself imports internal/shipping, order/dto.go)
// from one of THIS package's own INTERNAL test files would be a real
// import cycle: pickingbatch (test) -> order -> shipping -> packing ->
// pickingbatch. An external test package has no such problem: nothing
// imports pickingbatch_test, so pickingbatch_test -> {order,
// pickingbatch, picking} is a one-directional, acyclic edge regardless
// of what order/shipping/packing/pickingbatch import from each other.
package pickingbatch_test

import (
	"context"
	"errors"
	"sync"
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
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

// fixture is one fully-seeded tenant: a product, a warehouse, and a
// location within it — real rows via the real product/location/
// warehouse repositories, matching internal/order's own orderFixture
// shape exactly (this package orchestrates through both internal/order
// and internal/picking, so it needs the same dependency graph seeded).
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
		"pb-"+label, "Picking Batch Test Tenant "+label,
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

// stack wires the full real dependency graph this package orchestrates
// through — internal/order (to create/release orders) and internal/
// picking (SuggestLots/CreateTx/Get) — exactly the same graph
// cmd/server/main.go wires in production, bound to one shared *gorm.DB
// throughout.
type stack struct {
	orderSvc   *order.Service
	batchSvc   *pickingbatch.Service
	pickingSvc *picking.Service
}

func newStack(tx *gorm.DB) stack {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingRepo := picking.NewRepository(tx)
	pickingSvc := picking.NewService(pickingRepo, productRepo, warehouseRepo, invSvc)
	pickingSvc.SetUsers(user.NewRepository(tx))

	orderRepo := order.NewRepository(tx)
	customerRepo := customer.NewRepository(tx)
	shipToRepo := customerdeliverylocation.NewRepository(tx)
	// Phase 33D.1: order.NewService's own shipping dependency is passed
	// as nil here — internal/shipping now imports internal/packing (for
	// its own packingChecker), and internal/packing itself imports
	// internal/pickingbatch (Phase 33C.2's batchOrchestrator), so merely
	// importing internal/shipping from one of THIS package's own
	// internal (non-"_test"-suffixed-package) test files would be a real
	// import cycle: pickingbatch (test) -> shipping -> packing ->
	// pickingbatch. nil is safe: this package's own tests only ever
	// reach order.Service's shipping dependency through the read-only
	// FindByPickingTaskID path inside Service.Get, which every order
	// reaching this stack skips entirely — batch-picked orders never set
	// picking_task_id at all (see migrations/0097's own doc; they carry
	// picking_batch_id instead), so that branch is never taken here.
	orderSvc := order.NewService(orderRepo, productRepo, warehouseRepo, customerRepo, shipToRepo, pickingSvc, nil, invSvc)

	batchRepo := pickingbatch.NewRepository(tx)
	batchSvc := pickingbatch.NewService(batchRepo, warehouseRepo, pickingSvc)

	return stack{orderSvc: orderSvc, batchSvc: batchSvc, pickingSvc: pickingSvc}
}

// createReleasedOrder runs order.Service.Create then Release end-to-end
// (PENDING -> RELEASED) for a single-line PICK_BY_PRODUCT order, and
// returns the full response (including the line's own order_item_id).
func createReleasedOrder(t *testing.T, svc *order.Service, tenantID int64, f fixture, customerName string, qty float64) *order.Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, order.CreateRequest{
		WarehouseID: f.warehouseID, Customer: &customerName, PickingMode: "PICK_BY_PRODUCT",
		Items: []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("createReleasedOrder: Create: %v", err)
	}
	resp, err = svc.Release(context.Background(), tenantID, resp.ID, order.ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("createReleasedOrder: Release: %v", err)
	}
	return resp
}

// createReleasedOrderApprovingShortage is createReleasedOrder for a
// fixture that deliberately has insufficient on-hand stock to cover
// qty — Release's own availability-check gate would otherwise reject
// it before this package's own Create ever runs its aggregate
// insufficient-stock check.
func createReleasedOrderApprovingShortage(t *testing.T, svc *order.Service, tenantID int64, f fixture, customerName string, qty float64) *order.Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, order.CreateRequest{
		WarehouseID: f.warehouseID, Customer: &customerName, PickingMode: "PICK_BY_PRODUCT",
		Items: []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("createReleasedOrderApprovingShortage: Create: %v", err)
	}
	resp, err = svc.Release(context.Background(), tenantID, resp.ID, order.ReleaseRequest{ApproveShortage: true}, 1)
	if err != nil {
		t.Fatalf("createReleasedOrderApprovingShortage: Release: %v", err)
	}
	return resp
}

// seedPickerUser inserts a real role + user row — AssignPicker's own
// userChecker.FindByID call requires a real users row to reference (the
// FK_picking_tasks_assigned_picker constraint), matching internal/
// picking's own seedPickerUser precedent exactly.
func seedPickerUser(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	var roleID int64
	if err := tx.Raw(
		"INSERT INTO roles (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "PICKER-"+label, "Picker Role "+label,
	).Scan(&roleID).Error; err != nil {
		t.Fatalf("seedPickerUser(%s): insert role: %v", label, err)
	}
	var userID int64
	if err := tx.Raw(
		"INSERT INTO users (username, password_hash, full_name, email, role_id) OUTPUT INSERTED.id VALUES (?, 'x', ?, ?, ?)",
		"picker-"+label, "Picker "+label, "picker-"+label+"@example.com", roleID,
	).Scan(&userID).Error; err != nil {
		t.Fatalf("seedPickerUser(%s): insert user: %v", label, err)
	}
	return userID
}

func allocatedQtyForOrder(line pickingbatch.ConsolidatedLineResponse, orderID int64) float64 {
	var total float64
	for _, a := range line.Allocations {
		if a.OrderID == orderID {
			total += a.AllocatedQty
		}
	}
	return total
}

// --- Create (DRAFT consolidation) ---

// TestService_Create_ConsolidatesSO001SO002SO003 is the exact example
// from the Phase 33C.1 spec: SO-001 20 + SO-002 30 + SO-003 50 of the
// same SKU, in one lot, consolidate into one batch pick qty 100, with
// the allocation table showing the 20/30/50 split back to each order.
func TestService_Create_ConsolidatesSO001SO002SO003(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "so123")
	seedBalance(t, tx, f, "LOT-X", 100)

	st := newStack(tx)
	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer B", 30)
	so3 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer C", 50)

	resp, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID, so3.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.Status != pickingbatch.StatusDraft {
		t.Errorf("Status = %v, want DRAFT", resp.Status)
	}
	if len(resp.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (single lot covers all demand)", len(resp.Lines))
	}
	line := resp.Lines[0]
	if line.Qty != 100 {
		t.Errorf("line.Qty = %v, want 100", line.Qty)
	}
	if got := allocatedQtyForOrder(line, so1.ID); got != 20 {
		t.Errorf("SO-001 allocated qty = %v, want 20", got)
	}
	if got := allocatedQtyForOrder(line, so2.ID); got != 30 {
		t.Errorf("SO-002 allocated qty = %v, want 30", got)
	}
	if got := allocatedQtyForOrder(line, so3.ID); got != 50 {
		t.Errorf("SO-003 allocated qty = %v, want 50", got)
	}
	if len(resp.Orders) != 3 {
		t.Errorf("len(Orders) = %d, want 3", len(resp.Orders))
	}
}

// TestService_Create_DistinctCustomerShipToPreserved verifies this
// package never merges or rewrites each order's own customer snapshot
// when consolidating them into one batch — purely a "did not lose or
// conflate data" check, not a new feature.
func TestService_Create_DistinctCustomerShipToPreserved(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "distinct")
	seedBalance(t, tx, f, "LOT-X", 100)

	st := newStack(tx)
	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Acme Co", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Widgets Inc", 30)

	resp, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	byOrder := make(map[int64]*string, len(resp.Orders))
	for _, o := range resp.Orders {
		byOrder[o.OrderID] = o.Customer
	}
	if byOrder[so1.ID] == nil || *byOrder[so1.ID] != "Acme Co" {
		t.Errorf("SO-001 customer = %v, want Acme Co", byOrder[so1.ID])
	}
	if byOrder[so2.ID] == nil || *byOrder[so2.ID] != "Widgets Inc" {
		t.Errorf("SO-002 customer = %v, want Widgets Inc", byOrder[so2.ID])
	}
}

// TestService_Create_SplitsAcrossMultipleLots proves a single product's
// consolidated demand can be split across more than one FEFO lot/
// location, and that each order's own share still sums correctly across
// both resulting lines.
func TestService_Create_SplitsAcrossMultipleLots(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "splitlot")
	seedBalance(t, tx, f, "LOT-EARLY", 60)
	seedBalance(t, tx, f, "LOT-LATE", 40)

	st := newStack(tx)
	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer B", 30)
	so3 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "Customer C", 50)

	resp, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID, so3.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2 (100 demand split across 60+40 lots)", len(resp.Lines))
	}
	var totalQty float64
	perOrder := map[int64]float64{so1.ID: 0, so2.ID: 0, so3.ID: 0}
	for _, line := range resp.Lines {
		var lineSum float64
		for _, a := range line.Allocations {
			lineSum += a.AllocatedQty
			perOrder[a.OrderID] += a.AllocatedQty
		}
		if lineSum != line.Qty {
			t.Errorf("line %s/%s allocations sum = %v, want %v (line.Qty)", line.LotNo, line.LocationCode, lineSum, line.Qty)
		}
		totalQty += line.Qty
	}
	if totalQty != 100 {
		t.Errorf("total consolidated qty = %v, want 100", totalQty)
	}
	if perOrder[so1.ID] != 20 || perOrder[so2.ID] != 30 || perOrder[so3.ID] != 50 {
		t.Errorf("per-order totals = %v, want {20,30,50}", perOrder)
	}
}

func createPendingOrder(t *testing.T, svc *order.Service, tenantID int64, f fixture, pickingMode string, qty float64) *order.Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, order.CreateRequest{
		WarehouseID: f.warehouseID, PickingMode: pickingMode,
		Items: []order.CreateItemRequest{{ProductID: f.productID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("createPendingOrder: Create: %v", err)
	}
	return resp
}

func TestService_Create_RejectsOrderNotReleased(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "notreleased")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := createPendingOrder(t, st.orderSvc, f.tenantID, f, "PICK_BY_PRODUCT", 30) // still PENDING

	_, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("Create = %v, want pickingbatch.ErrValidation", err)
	}
	t.Logf("got expected validation error: %s", verr.Message)
}

func TestService_Create_RejectsPickByOrderMode(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "pickbyorder")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2Resp := createPendingOrder(t, st.orderSvc, f.tenantID, f, "PICK_BY_ORDER", 30)
	so2, err := st.orderSvc.Release(context.Background(), f.tenantID, so2Resp.ID, order.ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	_, err = st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("Create = %v, want pickingbatch.ErrValidation", err)
	}
}

func TestService_Create_RejectsCrossWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "xwh-a")
	b := newFixture(t, tx, "xwh-b")
	seedBalance(t, tx, a, "LOT-X", 100)
	seedBalance(t, tx, b, "LOT-X", 100)

	// b must be in the same tenant as a for this to exercise the
	// warehouse check specifically (cross-tenant would be rejected as
	// "not found" first) — reuse a's tenant for b's own warehouse/
	// location/product by seeding a second warehouse directly under
	// tenant a.
	var warehouseID2 int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-XWH2', 'Warehouse XWH2', 'Distribution Center', 'ACTIVE')",
		a.tenantID,
	).Scan(&warehouseID2).Error; err != nil {
		t.Fatalf("seed second warehouse: %v", err)
	}

	st := newStack(tx)
	so1 := createReleasedOrder(t, st.orderSvc, a.tenantID, a, "A", 20)
	// so2 belongs to a's own tenant but a's own warehouse (not warehouseID2)
	so2 := createReleasedOrder(t, st.orderSvc, a.tenantID, a, "B", 30)

	_, err := st.batchSvc.Create(context.Background(), a.tenantID, pickingbatch.CreateRequest{
		WarehouseID: warehouseID2, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("Create = %v, want pickingbatch.ErrValidation (cross warehouse)", err)
	}
}

func TestService_Create_RejectsCrossTenantOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "xten-a")
	b := newFixture(t, tx, "xten-b")
	seedBalance(t, tx, a, "LOT-X", 100)
	seedBalance(t, tx, b, "LOT-X", 100)

	st := newStack(tx)
	so1 := createReleasedOrder(t, st.orderSvc, a.tenantID, a, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, b.tenantID, b, "B", 30) // different tenant

	_, err := st.batchSvc.Create(context.Background(), a.tenantID, pickingbatch.CreateRequest{
		WarehouseID: a.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("Create = %v, want pickingbatch.ErrValidation (cross tenant order not found)", err)
	}
}

func TestService_Create_RejectsOrderAlreadyInActiveBatch(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "alreadybatched")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "B", 30)
	so3 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "C", 50)

	if _, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// so1 is now a member of an active (DRAFT) batch — a second batch
	// reusing it must be rejected, not silently drop it.
	_, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so3.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("second Create = %v, want pickingbatch.ErrValidation (already in active batch)", err)
	}
}

func TestService_Create_RejectsInsufficientStock(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "insufficient")
	seedBalance(t, tx, f, "LOT-X", 10) // far less than the 50 demanded below
	st := newStack(tx)

	so1 := createReleasedOrderApprovingShortage(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := createReleasedOrderApprovingShortage(t, st.orderSvc, f.tenantID, f, "B", 30)

	_, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	var verr pickingbatch.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("Create = %v, want pickingbatch.ErrValidation (insufficient stock)", err)
	}
}

// --- Confirm: proves the consolidated task runs through 100% unmodified
// internal/picking code (AssignPicker/ScanConfirm/Confirm(SHORT)/Repick/
// Confirm(PICKED)), and that the reservation/consumption numbers are
// exactly additive (100 = 20+30+50). ---

func TestService_Confirm_FullLifecycleThroughUnmodifiedPicking(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "lifecycle")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	so1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "B", 30)
	so3 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "C", 50)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{so1.ID, so2.ID, so3.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	confirmed, err := st.batchSvc.Confirm(context.Background(), f.tenantID, draft.ID, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status != pickingbatch.StatusConfirmed {
		t.Fatalf("Status = %v, want CONFIRMED", confirmed.Status)
	}
	if confirmed.PickingTaskID == nil {
		t.Fatal("PickingTaskID is nil after Confirm")
	}
	taskID := *confirmed.PickingTaskID

	// Every order is claimed into PICKING with picking_batch_id set,
	// never picking_task_id (see migrations/0097's own doc).
	for _, so := range []*order.Response{so1, so2, so3} {
		reloaded, err := st.orderSvc.Get(context.Background(), f.tenantID, so.ID)
		if err != nil {
			t.Fatalf("order.Get(%d): %v", so.ID, err)
		}
		if reloaded.Status != order.StatusPicking {
			t.Errorf("order %d status = %v, want PICKING", so.ID, reloaded.Status)
		}
		if reloaded.PickingBatchID == nil || *reloaded.PickingBatchID != draft.ID {
			t.Errorf("order %d picking_batch_id = %v, want %d", so.ID, reloaded.PickingBatchID, draft.ID)
		}
		if reloaded.PickingTaskID != nil {
			t.Errorf("order %d picking_task_id = %v, want nil (batch-picked orders never set it)", so.ID, *reloaded.PickingTaskID)
		}
	}

	// Allocations are backfilled with the real task item id.
	if len(confirmed.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(confirmed.Lines))
	}
	for _, a := range confirmed.Lines[0].Allocations {
		if a.PickingTaskItemID == nil {
			t.Errorf("allocation for order %d has nil PickingTaskItemID after Confirm", a.OrderID)
		}
	}
	itemID := *confirmed.Lines[0].Allocations[0].PickingTaskItemID

	// Reservation is exactly additive: 100 reserved, matching 20+30+50.
	var reserved float64
	if err := tx.Raw("SELECT reserved_quantity FROM inventory WHERE tenant_id = ? AND product_id = ? AND lot_no = ?", f.tenantID, f.productID, "LOT-X").Scan(&reserved).Error; err != nil {
		t.Fatalf("query reserved_quantity: %v", err)
	}
	if reserved != 100 {
		t.Errorf("reserved_quantity = %v, want 100", reserved)
	}

	// From this point on, every action runs through internal/picking's
	// own completely unmodified public API — AssignPicker, ScanConfirm,
	// a SHORT confirm, Repick, and the final completing PICKED confirm.
	pickerID := seedPickerUser(t, tx, f.tenantID, "lifecycle")
	if _, err := st.pickingSvc.AssignPicker(context.Background(), f.tenantID, taskID, pickerID, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}

	line := confirmed.Lines[0]
	scanResp, err := st.pickingSvc.ScanConfirm(context.Background(), f.tenantID, taskID, itemID, picking.ScanRequest{
		LocationCode: line.LocationCode, SKU: line.SKU, LotNo: line.LotNo,
	}, 1)
	if err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	if !scanResp.Matched {
		t.Errorf("ScanConfirm.Matched = false, want true (%s)", scanResp.Detail)
	}

	shortResp, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "lifecycle-short-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "SHORT", ShortReason: "OUT_OF_STOCK"}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(SHORT): %v", err)
	}
	if shortResp.Status != picking.StatusShortPick {
		t.Fatalf("after SHORT, task status = %v, want SHORT_PICK", shortResp.Status)
	}

	if _, err := st.pickingSvc.Repick(context.Background(), f.tenantID, taskID, itemID, 1); err != nil {
		t.Fatalf("Repick: %v", err)
	}

	completedResp, err := st.pickingSvc.Confirm(context.Background(), f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "lifecycle-picked-1",
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 100}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(PICKED): %v", err)
	}
	if completedResp.Status != picking.StatusCompleted {
		t.Fatalf("after full PICKED confirm, task status = %v, want COMPLETED", completedResp.Status)
	}

	// The batch's own status lazily mirrors the task's terminal status
	// on next read — never a cross-package DB trigger.
	finalBatch, err := st.batchSvc.Get(context.Background(), f.tenantID, draft.ID)
	if err != nil {
		t.Fatalf("Get after completion: %v", err)
	}
	if finalBatch.Status != pickingbatch.StatusCompleted {
		t.Errorf("batch Status = %v, want COMPLETED (mirrored from the task)", finalBatch.Status)
	}

	// Final stock: 100 consumed (RemoveStockTx), nothing left reserved —
	// exactly what three separate 20/30/50 picks would have produced.
	var row struct {
		Quantity         float64
		ReservedQuantity float64
	}
	if err := tx.Raw("SELECT quantity, reserved_quantity FROM inventory WHERE tenant_id = ? AND product_id = ? AND lot_no = ?", f.tenantID, f.productID, "LOT-X").Scan(&row).Error; err != nil {
		t.Fatalf("query final balance: %v", err)
	}
	if row.Quantity != 0 {
		t.Errorf("final quantity = %v, want 0 (100 on hand - 100 consumed)", row.Quantity)
	}
	if row.ReservedQuantity != 0 {
		t.Errorf("final reserved_quantity = %v, want 0", row.ReservedQuantity)
	}
}

// TestService_Confirm_ConcurrentConfirms_OnlyOneWins is this package's
// own concurrency proof, the same shape as internal/order's own
// TestService_CreatePickingTask_ConcurrentClaims_OnlyOneWins and Phase
// 32's relocation concurrency test — a real unwrapped *gorm.DB (not a
// shared transaction: concurrent goroutines against one BeginRollbackTx
// transaction would not exercise real row-locking) with manual cleanup.
//
// By design (see migrations/0095's own doc and LoadOrdersForConsolidation's
// own active_batch_count check), two DISTINCT DRAFT batches can never
// contain the same order_id in the first place — Create itself rejects
// that before a second DRAFT could ever exist. The real concurrency
// surface this package introduces is therefore two concurrent Confirm
// calls racing the SAME batch (a double-click/retry), which is exactly
// what this test fires: of N concurrent Confirm(batchID) calls, at most
// one may succeed (create a picking task and claim both orders); every
// other call must fail without creating a second task or double-
// claiming either order.
func TestService_Confirm_ConcurrentConfirms_OnlyOneWins(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('pb-concurrent', 'Picking Batch Concurrency Test Tenant', 'ACTIVE')",
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
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-PBCONC', 'Category PBConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-PBCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-PBCONC', 'Warehouse PBConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-PBCONC', 'Zone PBConc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-PBCONC", Name: "Location PBConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	f := fixture{tenantID: tenantID, productID: productID, warehouseID: warehouseID, locationID: locationID}
	seedBalance(t, db, f, "LOT-PBCONC", 100)

	st := newStack(db)
	so1 := createReleasedOrder(t, st.orderSvc, tenantID, f, "A", 20)
	so2 := createReleasedOrder(t, st.orderSvc, tenantID, f, "B", 30)

	draft, err := st.batchSvc.Create(context.Background(), tenantID, pickingbatch.CreateRequest{
		WarehouseID: warehouseID, OrderIDs: []int64{so1.ID, so2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const attempts = 10
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := st.batchSvc.Confirm(context.Background(), tenantID, draft.ID, 1)
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, pickingbatch.ErrConfirmConflict), errors.Is(err, pickingbatch.ErrNotConfirmable):
			conflicts++
		default:
			t.Errorf("concurrent Confirm returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d across %d concurrent Confirm calls, want exactly 1", successes, attempts)
	}
	if conflicts != attempts-1 {
		t.Errorf("conflict count = %d, want %d", conflicts, attempts-1)
	}

	// Exactly one picking task was created, and both orders were claimed
	// into it exactly once (never double-claimed).
	var taskCount int64
	if err := db.Raw("SELECT COUNT(*) FROM picking_tasks WHERE tenant_id = ?", tenantID).Scan(&taskCount).Error; err != nil {
		t.Fatalf("count picking_tasks: %v", err)
	}
	if taskCount != 1 {
		t.Errorf("picking_tasks count = %d, want 1", taskCount)
	}

	var pickingOrders int64
	if err := db.Raw("SELECT COUNT(*) FROM orders WHERE tenant_id = ? AND status = 'PICKING' AND picking_batch_id = ?", tenantID, draft.ID).Scan(&pickingOrders).Error; err != nil {
		t.Fatalf("count claimed orders: %v", err)
	}
	if pickingOrders != 2 {
		t.Errorf("orders claimed into PICKING for this batch = %d, want 2", pickingOrders)
	}
}
